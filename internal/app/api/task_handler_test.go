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

	domainevent "agent-runtime/internal/domain/event"
	httpserver "agent-runtime/internal/transport/http"
	"agent-runtime/internal/usecase/eventapi"
	"agent-runtime/internal/usecase/taskapi"
	apperrors "agent-runtime/pkg/errors"
)

// TestCreateTaskRoute 验证创建任务路由能解析 JWT、请求体并返回统一成功响应。
func TestCreateTaskRoute(t *testing.T) {
	now := time.Now().UTC()
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

// TestTaskRoutesRequireJWT 验证任务路由缺少 JWT 时会返回统一未授权错误。
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

// TestListTasksRoutePassesFiltersAndPagination 验证列表路由会透传过滤、分页和排序参数。
func TestListTasksRoutePassesFiltersAndPagination(t *testing.T) {
	now := time.Now().UTC()
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

// TestListTaskEventsRoutePassesFilters 验证 timeline 路由会透传游标、limit 和事件类型过滤。
func TestListTaskEventsRoutePassesFilters(t *testing.T) {
	now := time.Now().UTC()
	fakeEvents := &fakeEventService{
		listTaskEvents: func(_ context.Context, input eventapi.ListTaskEventsInput) (eventapi.EventPage, error) {
			if input.TenantID != "tenant_001" || input.UserID != "user_001" || input.TaskID != "task_001" {
				t.Fatalf("事件查询鉴权上下文不正确: %+v", input)
			}
			if input.AfterEventID != 9 || input.Limit != 5 || input.Type != "STEP_STARTED" {
				t.Fatalf("事件查询参数不正确: %+v", input)
			}
			return eventapi.EventPage{
				Items: []domainevent.AgentEvent{{
					EventID:   10,
					TaskID:    "task_001",
					Type:      "STEP_STARTED",
					Input:     json.RawMessage(`null`),
					Output:    json.RawMessage(`null`),
					Metadata:  json.RawMessage(`{}`),
					CreatedAt: now,
				}},
				NextAfterEventID: 10,
			}, nil
		},
	}
	router := newTestTaskRouterWithEvents(&fakeTaskService{}, fakeEvents)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task_001/events?after_event_id=9&limit=5&type=STEP_STARTED", nil)
	req.Header.Set("Authorization", "Bearer "+signAPITestJWT(t, "test-secret", map[string]any{
		"tenant_id": "tenant_001",
		"user_id":   "user_001",
		"exp":       now.Add(time.Hour).Unix(),
	}))
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

// TestStreamTaskEventsRouteUsesLastEventID 验证 SSE 路由会使用 Last-Event-ID 补发历史事件。
func TestStreamTaskEventsRouteUsesLastEventID(t *testing.T) {
	now := time.Now().UTC()
	backfillCalled := false
	fakeEvents := &fakeEventService{
		listAuthorizedTaskEvents: func(_ context.Context, tenantID string, taskID string, afterEventID int64, limit int, eventType string) (eventapi.EventPage, error) {
			if tenantID != "tenant_001" || taskID != "task_001" || afterEventID != 12 || limit != 100 || eventType != "" {
				t.Fatalf("SSE 补发参数不正确: tenant=%s task=%s after=%d limit=%d type=%s", tenantID, taskID, afterEventID, limit, eventType)
			}
			backfillCalled = true
			return eventapi.EventPage{}, nil
		},
	}
	router := newTestTaskRouterWithEvents(&fakeTaskService{}, fakeEvents)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task_001/events/stream", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", "12")
	req.Header.Set("Authorization", "Bearer "+signAPITestJWT(t, "test-secret", map[string]any{
		"tenant_id": "tenant_001",
		"user_id":   "user_001",
		"exp":       now.Add(time.Hour).Unix(),
	}))
	rr := httptest.NewRecorder()
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if !backfillCalled {
		t.Fatal("SSE 未调用历史事件补发")
	}
}

// newTestTaskRouter 创建带 request_id 和任务路由的测试路由器。
func newTestTaskRouter(service taskService) http.Handler {
	return newTestTaskRouterWithEvents(service, &fakeEventService{})
}

// newTestTaskRouterWithEvents 创建带 request_id、任务路由和事件路由的测试路由器。
func newTestTaskRouterWithEvents(service taskService, events eventService) http.Handler {
	router := chi.NewRouter()
	router.Use(httpserver.WithRequestID("X-Request-ID"))
	registerTaskRoutes(router, service, events, "test-secret", slog.Default())
	return router
}

type fakeTaskService struct {
	createTask func(context.Context, taskapi.CreateTaskInput) (taskapi.CreatedTask, error)
	getTask    func(context.Context, taskapi.GetTaskInput) (taskapi.TaskDetail, error)
	listTasks  func(context.Context, taskapi.ListTasksInput) (taskapi.TaskPage, error)
}

// CreateTask 调用测试注入的创建任务逻辑。
func (s *fakeTaskService) CreateTask(ctx context.Context, input taskapi.CreateTaskInput) (taskapi.CreatedTask, error) {
	if s.createTask == nil {
		return taskapi.CreatedTask{}, apperrors.New(apperrors.CodeNotImplemented, "fake 未实现 CreateTask")
	}
	return s.createTask(ctx, input)
}

// GetTask 调用测试注入的任务详情逻辑。
func (s *fakeTaskService) GetTask(ctx context.Context, input taskapi.GetTaskInput) (taskapi.TaskDetail, error) {
	if s.getTask == nil {
		return taskapi.TaskDetail{}, apperrors.New(apperrors.CodeNotImplemented, "fake 未实现 GetTask")
	}
	return s.getTask(ctx, input)
}

// ListTasks 调用测试注入的任务列表逻辑。
func (s *fakeTaskService) ListTasks(ctx context.Context, input taskapi.ListTasksInput) (taskapi.TaskPage, error) {
	if s.listTasks == nil {
		return taskapi.TaskPage{}, apperrors.New(apperrors.CodeNotImplemented, "fake 未实现 ListTasks")
	}
	return s.listTasks(ctx, input)
}

type fakeEventService struct {
	authorizeTask            func(context.Context, eventapi.TaskAccessInput) error
	listTaskEvents           func(context.Context, eventapi.ListTaskEventsInput) (eventapi.EventPage, error)
	listAuthorizedTaskEvents func(context.Context, string, string, int64, int, string) (eventapi.EventPage, error)
	hub                      *eventapi.Hub
}

// AuthorizeTask 调用测试注入的事件鉴权逻辑。
func (s *fakeEventService) AuthorizeTask(ctx context.Context, input eventapi.TaskAccessInput) error {
	if s.authorizeTask == nil {
		return nil
	}
	return s.authorizeTask(ctx, input)
}

// ListTaskEvents 调用测试注入的事件列表逻辑。
func (s *fakeEventService) ListTaskEvents(ctx context.Context, input eventapi.ListTaskEventsInput) (eventapi.EventPage, error) {
	if s.listTaskEvents == nil {
		return eventapi.EventPage{}, apperrors.New(apperrors.CodeNotImplemented, "fake 未实现 ListTaskEvents")
	}
	return s.listTaskEvents(ctx, input)
}

// ListAuthorizedTaskEvents 调用测试注入的已鉴权事件列表逻辑。
func (s *fakeEventService) ListAuthorizedTaskEvents(ctx context.Context, tenantID string, taskID string, afterEventID int64, limit int, eventType string) (eventapi.EventPage, error) {
	if s.listAuthorizedTaskEvents == nil {
		return eventapi.EventPage{}, nil
	}
	return s.listAuthorizedTaskEvents(ctx, tenantID, taskID, afterEventID, limit, eventType)
}

// Hub 返回测试用本地事件 hub。
func (s *fakeEventService) Hub() *eventapi.Hub {
	if s.hub == nil {
		s.hub = eventapi.NewHub(eventapi.DefaultHubBuffer)
	}
	return s.hub
}

// signAPITestJWT 生成 API handler 测试用 HS256 JWT。
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
