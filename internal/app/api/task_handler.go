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
	CancelTask(ctx context.Context, input taskapi.CancelTaskInput) (taskapi.TaskStatusResult, error)
	PauseTask(ctx context.Context, input taskapi.PauseTaskInput) (taskapi.TaskStatusResult, error)
	ResumeTask(ctx context.Context, input taskapi.ResumeTaskInput) (taskapi.ResumeTaskResult, error)
	ResumeFromCheckpoint(ctx context.Context, input taskapi.ResumeFromCheckpointInput) (taskapi.ResumeTaskResult, error)
	GetTaskState(ctx context.Context, input taskapi.GetTaskStateInput) (taskapi.TaskStateResult, error)
	DecideToolCall(ctx context.Context, input taskapi.DecideToolCallInput) (taskapi.ToolCallDecisionResult, error)
	ListToolCalls(ctx context.Context, input taskapi.ListTaskChildrenInput) (taskapi.ToolCallPage, error)
	ListCheckpoints(ctx context.Context, input taskapi.ListTaskChildrenInput) (taskapi.CheckpointPage, error)
	ListArtifacts(ctx context.Context, input taskapi.ListTaskChildrenInput) (taskapi.ArtifactPage, error)
}

type taskHandler struct {
	service taskService
	logger  *slog.Logger
}

type createTaskRequest struct {
	Goal            string            `json:"goal"`
	ClientRequestID string            `json:"client_request_id"`
	Budget          domaintask.Budget `json:"budget"`
	Constraints     json.RawMessage   `json:"constraints"`
}

type decideToolCallRequest struct {
	Comment string `json:"comment"`
	Note    string `json:"note"`
	Reason  string `json:"reason"`
	// TerminateTask 对齐审批拒绝扩展 body,用于决定是否直接终止任务。
	TerminateTask bool `json:"terminate_task"`
}

type resumeFromCheckpointRequest struct {
	CheckpointID string `json:"checkpoint_id"`
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
		TenantID:        tenantID,
		UserID:          user.UserID,
		UserRole:        user.Role,
		RequestID:       httpserver.RequestIDFromContext(r.Context()),
		ClientRequestID: req.ClientRequestID,
		Goal:            req.Goal,
		Budget:          req.Budget,
		Constraints:     req.Constraints,
		IP:              httpserver.ClientIP(r),
		UserAgent:       r.UserAgent(),
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

	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
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

// cancelTask 处理任务取消请求，幂等返回当前任务状态。
func (h *taskHandler) cancelTask(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	result, err := h.service.CancelTask(r.Context(), taskapi.CancelTaskInput{
		TenantID:  tenantID,
		UserID:    user.UserID,
		UserRole:  user.Role,
		TaskID:    taskID,
		IP:        httpserver.ClientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteData(w, http.StatusOK, result)
}

// pauseTask 处理任务暂停请求，只允许 RUNNING 任务进入 PAUSED。
func (h *taskHandler) pauseTask(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	result, err := h.service.PauseTask(r.Context(), taskapi.PauseTaskInput{
		TenantID:  tenantID,
		UserID:    user.UserID,
		UserRole:  user.Role,
		TaskID:    taskID,
		IP:        httpserver.ClientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteData(w, http.StatusOK, result)
}

// resumeTask 处理任务恢复请求，从最近 checkpoint 重新排队。
func (h *taskHandler) resumeTask(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	result, err := h.service.ResumeTask(r.Context(), taskapi.ResumeTaskInput{
		TenantID:  tenantID,
		UserID:    user.UserID,
		UserRole:  user.Role,
		TaskID:    taskID,
		IP:        httpserver.ClientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteData(w, http.StatusOK, result)
}

// resumeFromCheckpoint 处理从指定 checkpoint 恢复任务的请求。
func (h *taskHandler) resumeFromCheckpoint(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	var req resumeFromCheckpointRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	result, err := h.service.ResumeFromCheckpoint(r.Context(), taskapi.ResumeFromCheckpointInput{
		TenantID:     tenantID,
		UserID:       user.UserID,
		UserRole:     user.Role,
		TaskID:       taskID,
		CheckpointID: req.CheckpointID,
		IP:           httpserver.ClientIP(r),
		UserAgent:    r.UserAgent(),
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteData(w, http.StatusOK, result)
}

// getTaskState 处理任务当前 AgentState 查询,用例层按角色控制完整 state 是否返回。
func (h *taskHandler) getTaskState(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	result, err := h.service.GetTaskState(r.Context(), taskapi.GetTaskStateInput{
		TenantID: tenantID,
		UserID:   user.UserID,
		UserRole: user.Role,
		TaskID:   taskID,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteData(w, http.StatusOK, result)
}

// listToolCalls 处理任务工具调用列表请求。
func (h *taskHandler) listToolCalls(w http.ResponseWriter, r *http.Request) {
	h.listTaskChildren(w, r, func(ctx context.Context, input taskapi.ListTaskChildrenInput) (any, error) {
		return h.service.ListToolCalls(ctx, input)
	})
}

// listCheckpoints 处理任务 checkpoint 列表请求。
func (h *taskHandler) listCheckpoints(w http.ResponseWriter, r *http.Request) {
	h.listTaskChildren(w, r, func(ctx context.Context, input taskapi.ListTaskChildrenInput) (any, error) {
		return h.service.ListCheckpoints(ctx, input)
	})
}

// listArtifacts 处理任务 artifact 列表请求。
func (h *taskHandler) listArtifacts(w http.ResponseWriter, r *http.Request) {
	h.listTaskChildren(w, r, func(ctx context.Context, input taskapi.ListTaskChildrenInput) (any, error) {
		return h.service.ListArtifacts(ctx, input)
	})
}

// decideToolCall 处理工具调用审批通过或拒绝请求。
func (h *taskHandler) decideToolCall(w http.ResponseWriter, r *http.Request, approve bool) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}
	callID := strings.TrimSpace(chi.URLParam(r, "call_id"))
	if callID == "" {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeInvalidArg, "call_id 不能为空"))
		return
	}
	var req decideToolCallRequest
	if err := decodeOptionalJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	result, err := h.service.DecideToolCall(r.Context(), taskapi.DecideToolCallInput{
		TenantID:      tenantID,
		UserID:        user.UserID,
		UserRole:      user.Role,
		CallID:        callID,
		Approve:       approve,
		Comment:       req.Comment,
		Note:          req.Note,
		Reason:        req.Reason,
		TerminateTask: req.TerminateTask,
		IP:            httpserver.ClientIP(r),
		UserAgent:     r.UserAgent(),
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteData(w, http.StatusOK, result)
}

// listTaskChildren 解析通用子资源分页参数并执行对应查询。
func (h *taskHandler) listTaskChildren(w http.ResponseWriter, r *http.Request, list func(context.Context, taskapi.ListTaskChildrenInput) (any, error)) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	limit, err := parsePositiveIntQuery(query.Get("limit"), 50, "limit")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	offset, err := parseNonNegativeIntQuery(query.Get("offset"), 0, "offset")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	result, err := list(r.Context(), taskapi.ListTaskChildrenInput{
		TenantID: tenantID,
		UserID:   user.UserID,
		UserRole: user.Role,
		TaskID:   taskID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteData(w, http.StatusOK, result)
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
		TenantID:        tenantID,
		UserID:          user.UserID,
		UserRole:        user.Role,
		Status:          status,
		Goal:            query.Get("goal"),
		FilterUserID:    query.Get("user_id"),
		FilterTenantID:  query.Get("tenant_id"),
		CreatedFrom:     query.Get("created_from"),
		CreatedTo:       query.Get("created_to"),
		WaitingApproval: query.Get("waiting_approval") == "true",
		StoppedByLimit:  query.Get("stopped_by_limit") == "true",
		Page:            page,
		PageSize:        pageSize,
		Sort:            sort,
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

// decodeOptionalJSONBody 解码可选 JSON 请求体，空 body 按空对象处理。
func decodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	return decodeJSONBody(w, r, dst)
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

// parseNonNegativeIntQuery 解析非负整数查询参数，空值时返回默认值。
func parseNonNegativeIntQuery(raw string, fallback int, name string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, apperrors.New(apperrors.CodeInvalidArg, name+" 必须是非负整数")
	}
	return value, nil
}

// taskIDFromRequest 从路由参数中读取 task_id。
func taskIDFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	taskID := strings.TrimSpace(chi.URLParam(r, "task_id"))
	if taskID == "" {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeInvalidArg, "task_id 不能为空"))
		return "", false
	}
	return taskID, true
}
