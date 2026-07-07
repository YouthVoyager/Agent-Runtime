package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	httpserver "agent-runtime/internal/transport/http"
	"agent-runtime/internal/usecase/taskapi"
	apperrors "agent-runtime/pkg/errors"
)

func TestCreateTaskRoute(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	fake := &fakeTaskService{
		createTask: func(_ context.Context, input taskapi.CreateTaskInput) (taskapi.CreatedTask, error) {
			if input.TenantID != "tenant_001" || input.UserID != "user_001" {
				t.Fatalf("鉴权上下文未正确传入: %+v", input)
			}
			if input.Goal != "生成技术方案" || input.Budget.MaxSteps != 50 {
				t.Fatalf("请求体未正确解析: %+v", input)
			}
			return taskapi.CreatedTask{TaskID: "task_001", Status: "QUEUED", CreatedAt: now}, nil
		},
	}
	router := newTestTaskRouter(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{
		"goal":"生成技术方案",
		"budget":{"max_steps":50,"max_tokens":200000,"max_tool_calls":40,"max_cost_usd":10},
		"constraints":{"language":"zh-CN"}
	}`))
	req.Header.Set("Authorization", "Bearer "+signAPITestJWT(t, "test-secret", map[string]any{
		"tenant_id": "tenant_001",
		"user_id":   "user_001",
		"exp":       now.Add(time.Hour).Unix(),
	}))
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Data taskapi.CreatedTask `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if body.Data.TaskID != "task_001" || body.Data.Status != "QUEUED" {
		t.Fatalf("响应数据不正确: %+v", body.Data)
	}
}

func TestTaskRoutesRequireJWT(t *testing.T) {
	router := newTestTaskRouter(&fakeTaskService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	var body httpserver.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是统一错误 JSON: %v", err)
	}
	if body.Error.Code != string(apperrors.CodeUnauthorized) || body.Error.RequestID == "" {
		t.Fatalf("错误响应不正确: %+v", body.Error)
	}
}

func TestListTasksRoutePassesFiltersAndPagination(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	fake := &fakeTaskService{
		listTasks: func(_ context.Context, input taskapi.ListTasksInput) (taskapi.TaskPage, error) {
			if input.TenantID != "tenant_001" || input.Status != "RUNNING" || input.Page != 2 || input.PageSize != 5 || input.Sort != "created_at_desc" {
				t.Fatalf("列表参数不正确: %+v", input)
			}
			return taskapi.TaskPage{
				Items: []taskapi.TaskSummary{{
					TaskID:    "task_001",
					Goal:      "生成技术方案",
					Status:    "RUNNING",
					CreatedAt: now,
					UpdatedAt: now,
				}},
				Page:     2,
				PageSize: 5,
				Sort:     "created_at_desc",
				Status:   "RUNNING",
			}, nil
		},
	}
	router := newTestTaskRouter(fake)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?status=RUNNING&page=2&page_size=5&sort=created_at_desc", nil)
	req.Header.Set("Authorization", "Bearer "+signAPITestJWT(t, "test-secret", map[string]any{
		"tenant_id": "tenant_001",
		"user_id":   "user_001",
		"role":      "admin",
		"exp":       now.Add(time.Hour).Unix(),
	}))
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func newTestTaskRouter(service taskService) http.Handler {
	router := chi.NewRouter()
	router.Use(httpserver.WithRequestID("X-Request-ID"))
	registerTaskRoutes(router, service, "test-secret", slog.Default())
	return router
}

type fakeTaskService struct {
	createTask func(context.Context, taskapi.CreateTaskInput) (taskapi.CreatedTask, error)
	getTask    func(context.Context, taskapi.GetTaskInput) (taskapi.TaskDetail, error)
	listTasks  func(context.Context, taskapi.ListTasksInput) (taskapi.TaskPage, error)
}

func (s *fakeTaskService) CreateTask(ctx context.Context, input taskapi.CreateTaskInput) (taskapi.CreatedTask, error) {
	if s.createTask == nil {
		return taskapi.CreatedTask{}, apperrors.New(apperrors.CodeNotImplemented, "fake 未实现 CreateTask")
	}
	return s.createTask(ctx, input)
}

func (s *fakeTaskService) GetTask(ctx context.Context, input taskapi.GetTaskInput) (taskapi.TaskDetail, error) {
	if s.getTask == nil {
		return taskapi.TaskDetail{}, apperrors.New(apperrors.CodeNotImplemented, "fake 未实现 GetTask")
	}
	return s.getTask(ctx, input)
}

func (s *fakeTaskService) ListTasks(ctx context.Context, input taskapi.ListTasksInput) (taskapi.TaskPage, error) {
	if s.listTasks == nil {
		return taskapi.TaskPage{}, apperrors.New(apperrors.CodeNotImplemented, "fake 未实现 ListTasks")
	}
	return s.listTasks(ctx, input)
}

func signAPITestJWT(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]any{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("序列化 header 失败: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("序列化 claims 失败: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + payload + "." + signature
}
