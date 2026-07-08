package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	domainevent "agent-runtime/internal/domain/event"
	httpserver "agent-runtime/internal/transport/http"
	"agent-runtime/internal/usecase/eventapi"
	apperrors "agent-runtime/pkg/errors"
)

const (
	defaultSSEBackfillLimit = 100
	sseHeartbeatInterval    = 15 * time.Second
)

type eventService interface {
	AuthorizeTask(ctx context.Context, input eventapi.TaskAccessInput) error
	ListTaskEvents(ctx context.Context, input eventapi.ListTaskEventsInput) (eventapi.EventPage, error)
	ListAuthorizedTaskEvents(ctx context.Context, tenantID string, taskID string, afterEventID int64, limit int, eventType string) (eventapi.EventPage, error)
	Hub() *eventapi.Hub
}

type eventHandler struct {
	service eventService
	logger  *slog.Logger
}

// newEventHandler 创建 AgentEvent HTTP handler。
func newEventHandler(service eventService, logger *slog.Logger) *eventHandler {
	return &eventHandler{service: service, logger: logger}
}

// listTaskEvents 处理任务 timeline 查询请求。
func (h *eventHandler) listTaskEvents(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}

	taskID := strings.TrimSpace(chi.URLParam(r, "task_id"))
	if taskID == "" {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeInvalidArg, "task_id 不能为空"))
		return
	}

	query := r.URL.Query()
	afterEventID, err := parseNonNegativeInt64Query(query.Get("after_event_id"), 0, "after_event_id")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	limit, err := parsePositiveIntQuery(query.Get("limit"), defaultSSEBackfillLimit, "limit")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	page, err := h.service.ListTaskEvents(r.Context(), eventapi.ListTaskEventsInput{
		TenantID:     tenantID,
		UserID:       user.UserID,
		UserRole:     user.Role,
		TaskID:       taskID,
		AfterEventID: afterEventID,
		Limit:        limit,
		Type:         query.Get("type"),
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	httpserver.WriteData(w, http.StatusOK, page)
}

// streamTaskEvents 处理任务事件 SSE 实时推送请求。
func (h *eventHandler) streamTaskEvents(w http.ResponseWriter, r *http.Request) {
	user, tenantID, ok := principalFromRequest(w, r)
	if !ok {
		return
	}

	taskID := strings.TrimSpace(chi.URLParam(r, "task_id"))
	lastEventID, err := parseLastEventID(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	if err := h.service.AuthorizeTask(r.Context(), eventapi.TaskAccessInput{
		TenantID: tenantID,
		UserID:   user.UserID,
		UserRole: user.Role,
		TaskID:   taskID,
	}); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeUnavailable, "当前 HTTP writer 不支持 SSE"))
		return
	}
	// SSE 是长连接，清理本次响应的写 deadline，避免服务级 WriteTimeout 提前关闭连接。
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream; charset=utf-8")
	headers.Set("Cache-Control", "no-cache, no-transform")
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sub := h.service.Hub().Subscribe(tenantID, taskID)
	defer sub.Close()

	lastSentID := lastEventID
	if err := h.sendBackfill(r.Context(), w, flusher, tenantID, taskID, &lastSentID); err != nil {
		h.logger.Debug("SSE 历史事件补发结束", "task_id", taskID, "error", err)
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-sub.C():
			if !ok {
				return
			}
			if event.EventID <= lastSentID {
				continue
			}
			if err := writeSSEEvent(w, event); err != nil {
				h.logger.Debug("SSE 事件写入失败", "task_id", taskID, "event_id", event.EventID, "error", err)
				return
			}
			lastSentID = event.EventID
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				h.logger.Debug("SSE 心跳写入失败", "task_id", taskID, "error", err)
				return
			}
			flusher.Flush()
		}
	}
}

// sendBackfill 从 Last-Event-ID 之后补发数据库中的历史事件。
func (h *eventHandler) sendBackfill(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, tenantID string, taskID string, lastSentID *int64) error {
	for {
		page, err := h.service.ListAuthorizedTaskEvents(ctx, tenantID, taskID, *lastSentID, defaultSSEBackfillLimit, "")
		if err != nil {
			return err
		}
		for _, event := range page.Items {
			if event.EventID <= *lastSentID {
				continue
			}
			if err := writeSSEEvent(w, event); err != nil {
				return err
			}
			*lastSentID = event.EventID
		}
		flusher.Flush()
		if !page.HasMore {
			return nil
		}
	}
}

// writeSSEEvent 按 SSE 协议写出单条 AgentEvent。
func writeSSEEvent(w http.ResponseWriter, event domainevent.AgentEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\n", event.EventID); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "event: agent_event\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return nil
}

// parseLastEventID 解析 SSE 重连游标，优先使用 Last-Event-ID 请求头。
func parseLastEventID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("last_event_id"))
	}
	return parseNonNegativeInt64Query(raw, 0, "Last-Event-ID")
}

// parseNonNegativeInt64Query 解析非负整数查询参数，空值时返回默认值。
func parseNonNegativeInt64Query(raw string, fallback int64, name string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, apperrors.New(apperrors.CodeInvalidArg, name+" 必须是非负整数")
	}
	return value, nil
}
