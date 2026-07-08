package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	domaintask "stableagent/internal/domain/task"
	"stableagent/internal/security/authn"
	securitytenant "stableagent/internal/security/tenant"
	httpserver "stableagent/internal/transport/http"
	"stableagent/internal/usecase/taskapi"
	apperrors "stableagent/pkg/errors"
)

const maxTaskRequestBodyBytes = 1 << 20

type taskService interface {
	CreateTask(ctx context.Context, input taskapi.CreateTaskInput) (taskapi.CreatedTask, error)
	GetTask(ctx context.Context, input taskapi.GetTaskInput) (taskapi.TaskDetail, error)
	ListTasks(ctx context.Context, input taskapi.ListTasksInput) (taskapi.TaskPage, error)
}

type taskHandler struct {
	service taskService
	logger  *slog.Logger
}

type createTaskRequest struct {
	Goal        string            `json:"goal"`
	Budget      domaintask.Budget `json:"budget"`
	Constraints json.RawMessage   `json:"constraints"`
}

// newTaskHandler 创建任务 HTTP handler。
func newTaskHandler(service taskService, logger *slog.Logger) *taskHandler {
	return &taskHandler{service: service, logger: logger}
}

// createTask 处理任务创建请求，完成请求体解析并委托用例层创建任务。
func (h *taskHandler) createTask(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}

	var req createTaskRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	task, err := h.service.CreateTask(r.Context(), taskapi.CreateTaskInput{
		TenantID:    tenantID,
		UserID:      user.UserID,
		UserRole:    user.Role,
		RequestID:   httpserver.RequestIDFromContext(r.Context()),
		Goal:        req.Goal,
		Budget:      req.Budget,
		Constraints: req.Constraints,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	httpserver.WriteData(w, http.StatusCreated, task)
}

// getTask 处理任务详情查询请求。
func (h *taskHandler) getTask(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}

	taskID := strings.TrimSpace(chi.URLParam(r, "task_id"))
	if taskID == "" {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeInvalidArg, "task_id 不能为空"))
		return
	}

	task, err := h.service.GetTask(r.Context(), taskapi.GetTaskInput{
		TenantID: tenantID,
		UserID:   user.UserID,
		UserRole: user.Role,
		TaskID:   taskID,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	httpserver.WriteData(w, http.StatusOK, task)
}

// listTasks 处理任务列表查询请求，并解析过滤和分页参数。
func (h *taskHandler) listTasks(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}

	query := r.URL.Query()
	status := strings.TrimSpace(query.Get("status"))
	page, err := parsePositiveIntQuery(query.Get("page"), 1, "page")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	pageSize, err := parsePositiveIntQuery(query.Get("page_size"), 20, "page_size")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	sort := strings.TrimSpace(query.Get("sort"))
	if sort == "" {
		// 当前 SQL 只按创建时间倒序索引优化，暂不开放其他排序避免慢查询。
		sort = "created_at_desc"
	}

	tasks, err := h.service.ListTasks(r.Context(), taskapi.ListTasksInput{
		TenantID: tenantID,
		UserID:   user.UserID,
		UserRole: user.Role,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
		Sort:     sort,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	httpserver.WriteData(w, http.StatusOK, tasks)
}

// principalFromRequest 从上下文提取用户和租户，并校验两者一致。
func principalFromRequest(w http.ResponseWriter, r *http.Request) (authn.User, string, bool) {
	user, ok := authn.UserFromContext(r.Context())
	if !ok {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeUnauthorized, "缺少用户上下文"))
		return authn.User{}, "", false
	}
	tenantID, ok := securitytenant.TenantIDFromContext(r.Context())
	// 租户上下文必须与用户声明一致，避免调用方伪造跨租户请求。
	if !ok || tenantID != user.TenantID {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeUnauthorized, "缺少租户上下文"))
		return authn.User{}, "", false
	}
	return user, tenantID, true
}

// decodeJSONBody 安全解码 JSON 请求体，限制大小、拒绝未知字段和多余 JSON 文档。
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxTaskRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apperrors.New(apperrors.CodeInvalidArg, "请求体不能为空")
		}
		return apperrors.Wrap(apperrors.CodeInvalidArg, "请求 JSON 格式错误", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return apperrors.New(apperrors.CodeInvalidArg, "请求体只能包含一个 JSON 对象")
	}
	return nil
}

// parsePositiveIntQuery 解析正整数查询参数，空值时返回默认值。
func parsePositiveIntQuery(raw string, fallback int, name string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, apperrors.New(apperrors.CodeInvalidArg, name+" 必须是正整数")
	}
	return value, nil
}
