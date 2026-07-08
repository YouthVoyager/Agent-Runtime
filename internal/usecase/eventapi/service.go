package eventapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domainevent "agent-runtime/internal/domain/event"
	"agent-runtime/internal/infra/postgres/db"
	"agent-runtime/internal/security/authn"
	"agent-runtime/internal/security/authz"
	apperrors "agent-runtime/pkg/errors"
)

const (
	defaultEventLimit = 100
	maxEventLimit     = 200
)

type Publisher interface {
	Publish(ctx context.Context, event domainevent.AgentEvent) error
}

type Service struct {
	queries   *db.Queries
	hub       *Hub
	publisher Publisher
	logger    *slog.Logger
}

type AppendEventInput struct {
	TenantID string
	TaskID   string
	Type     string
	Input    json.RawMessage
	Output   json.RawMessage
	Metadata json.RawMessage
	TraceID  *string
	SpanID   *string
}

type TaskAccessInput struct {
	TenantID string
	UserID   string
	UserRole string
	TaskID   string
}

type ListTaskEventsInput struct {
	TenantID     string
	UserID       string
	UserRole     string
	TaskID       string
	AfterEventID int64
	Limit        int
	Type         string
}

type EventPage struct {
	Items            []domainevent.AgentEvent `json:"items"`
	NextAfterEventID int64                    `json:"next_after_event_id"`
	HasMore          bool                     `json:"has_more"`
}

// NewService 创建 AgentEvent 用例服务。
func NewService(database db.DBTX, hub *Hub, publisher Publisher, logger *slog.Logger) *Service {
	if hub == nil {
		hub = NewHub(DefaultHubBuffer)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		queries:   db.New(database),
		hub:       hub,
		publisher: publisher,
		logger:    logger,
	}
}

// Append 追加写入 AgentEvent，并在数据库写入成功后推送到本地和跨实例事件通道。
func (s *Service) Append(ctx context.Context, input AppendEventInput) (domainevent.AgentEvent, error) {
	params, err := appendParams(input)
	if err != nil {
		return domainevent.AgentEvent{}, err
	}

	row, err := s.queries.CreateAgentEvent(ctx, params)
	if err != nil {
		return domainevent.AgentEvent{}, mapPostgresWriteError(err)
	}
	event := eventFromDB(row)
	s.publish(ctx, event)
	return event, nil
}

// AuthorizeTask 校验当前用户是否具备读取任务事件流的权限。
func (s *Service) AuthorizeTask(ctx context.Context, input TaskAccessInput) error {
	if strings.TrimSpace(input.TaskID) == "" {
		return apperrors.New(apperrors.CodeInvalidArg, "task_id 不能为空")
	}
	task, err := s.queries.GetAgentTask(ctx, db.GetAgentTaskParams{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
	})
	if err != nil {
		return mapPostgresReadError(err)
	}
	if !authz.CanReadTask(authn.User{TenantID: input.TenantID, UserID: input.UserID, Role: input.UserRole}, task.UserID) {
		return apperrors.New(apperrors.CodeForbidden, "没有访问该任务事件的权限")
	}
	return nil
}

// ListTaskEvents 查询任务 timeline，并按 event_id 升序返回。
func (s *Service) ListTaskEvents(ctx context.Context, input ListTaskEventsInput) (EventPage, error) {
	if err := s.AuthorizeTask(ctx, TaskAccessInput{
		TenantID: input.TenantID,
		UserID:   input.UserID,
		UserRole: input.UserRole,
		TaskID:   input.TaskID,
	}); err != nil {
		return EventPage{}, err
	}
	return s.listAuthorizedTaskEvents(ctx, input.TenantID, input.TaskID, input.AfterEventID, input.Limit, input.Type)
}

// ListAuthorizedTaskEvents 查询已完成鉴权任务的 timeline，供 SSE 补发历史事件使用。
func (s *Service) ListAuthorizedTaskEvents(ctx context.Context, tenantID string, taskID string, afterEventID int64, limit int, eventType string) (EventPage, error) {
	return s.listAuthorizedTaskEvents(ctx, tenantID, taskID, afterEventID, limit, eventType)
}

// Hub 返回当前进程内的事件推送中心。
func (s *Service) Hub() *Hub {
	return s.hub
}

// PublishLocal 将跨实例收到的事件投递到当前实例本地订阅者。
func (s *Service) PublishLocal(event domainevent.AgentEvent) {
	s.hub.Publish(event)
}

// NotifyPersisted 推送已经由其他事务写入成功的事件。
func (s *Service) NotifyPersisted(ctx context.Context, event domainevent.AgentEvent) {
	s.publish(ctx, event)
}

// listAuthorizedTaskEvents 执行实际 timeline 查询，调用前必须完成任务访问权限校验。
func (s *Service) listAuthorizedTaskEvents(ctx context.Context, tenantID string, taskID string, afterEventID int64, limit int, eventType string) (EventPage, error) {
	limit, err := normalizeEventLimit(limit)
	if err != nil {
		return EventPage{}, err
	}
	if afterEventID < 0 {
		return EventPage{}, apperrors.New(apperrors.CodeInvalidArg, "after_event_id 必须大于等于 0")
	}

	filterType := strings.TrimSpace(eventType)
	if filterType != "" {
		var ok bool
		filterType, ok = domainevent.NormalizeType(filterType)
		if !ok {
			return EventPage{}, apperrors.New(apperrors.CodeInvalidArg, "type 参数不合法")
		}
	}

	rowsLimit := int32(limit + 1)
	var rows []db.AgentEvent
	if filterType == "" {
		rows, err = s.queries.ListAgentEventsByTask(ctx, db.ListAgentEventsByTaskParams{
			TenantID:     tenantID,
			TaskID:       taskID,
			AfterEventID: afterEventID,
			LimitRows:    rowsLimit,
		})
	} else {
		rows, err = s.queries.ListAgentEventsByTaskAndType(ctx, db.ListAgentEventsByTaskAndTypeParams{
			TenantID:     tenantID,
			TaskID:       taskID,
			Type:         filterType,
			AfterEventID: afterEventID,
			LimitRows:    rowsLimit,
		})
	}
	if err != nil {
		return EventPage{}, mapPostgresReadError(err)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	events := make([]domainevent.AgentEvent, 0, len(rows))
	nextAfterEventID := afterEventID
	for _, row := range rows {
		event := eventFromDB(row)
		events = append(events, event)
		nextAfterEventID = event.EventID
	}
	return EventPage{
		Items:            events,
		NextAfterEventID: nextAfterEventID,
		HasMore:          hasMore,
	}, nil
}

// publish 在事件写入成功后进行非阻塞推送，推送失败不影响调用方。
func (s *Service) publish(ctx context.Context, event domainevent.AgentEvent) {
	s.hub.Publish(event)
	if s.publisher == nil {
		return
	}
	if err := s.publisher.Publish(ctx, event); err != nil {
		s.logger.Warn("AgentEvent 跨实例推送失败",
			"tenant_id", event.TenantID,
			"task_id", event.TaskID,
			"event_id", event.EventID,
			"error", err,
		)
	}
}

// appendParams 校验追加事件输入并转换为数据库参数。
func appendParams(input AppendEventInput) (db.CreateAgentEventParams, error) {
	tenantID := strings.TrimSpace(input.TenantID)
	taskID := strings.TrimSpace(input.TaskID)
	if tenantID == "" {
		return db.CreateAgentEventParams{}, apperrors.New(apperrors.CodeInvalidArg, "tenant_id 不能为空")
	}
	if taskID == "" {
		return db.CreateAgentEventParams{}, apperrors.New(apperrors.CodeInvalidArg, "task_id 不能为空")
	}
	eventType, ok := domainevent.NormalizeType(input.Type)
	if !ok {
		return db.CreateAgentEventParams{}, apperrors.New(apperrors.CodeInvalidArg, "事件类型不合法")
	}

	eventInput := domainevent.NormalizeJSONValue(input.Input)
	eventOutput := domainevent.NormalizeJSONValue(input.Output)
	metadata := domainevent.NormalizeMetadata(input.Metadata)
	if err := validateJSON("input", eventInput); err != nil {
		return db.CreateAgentEventParams{}, err
	}
	if err := validateJSON("output", eventOutput); err != nil {
		return db.CreateAgentEventParams{}, err
	}
	if err := validateJSONObject("metadata", metadata); err != nil {
		return db.CreateAgentEventParams{}, err
	}

	return db.CreateAgentEventParams{
		TenantID: tenantID,
		TaskID:   taskID,
		Type:     eventType,
		Input:    eventInput,
		Output:   eventOutput,
		Metadata: metadata,
		TraceID:  trimOptional(input.TraceID),
		SpanID:   trimOptional(input.SpanID),
	}, nil
}

// validateJSON 校验事件 JSON 字段必须是合法 JSON。
func validateJSON(name string, value json.RawMessage) error {
	if !json.Valid(value) {
		return apperrors.New(apperrors.CodeInvalidArg, name+" 必须是合法 JSON")
	}
	return nil
}

// validateJSONObject 校验 metadata 必须是 JSON object，便于前端和审计代码稳定消费。
func validateJSONObject(name string, value json.RawMessage) error {
	if err := validateJSON(name, value); err != nil {
		return err
	}
	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil {
		return apperrors.Wrap(apperrors.CodeInvalidArg, name+" 必须是 JSON object", err)
	}
	if object == nil {
		return apperrors.New(apperrors.CodeInvalidArg, name+" 必须是 JSON object")
	}
	return nil
}

// trimOptional 标准化可选字符串指针，空字符串按 nil 存储。
func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// normalizeEventLimit 规范化 timeline limit。
func normalizeEventLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultEventLimit, nil
	}
	if limit < 1 || limit > maxEventLimit {
		return 0, apperrors.New(apperrors.CodeInvalidArg, fmt.Sprintf("limit 必须在 1 到 %d 之间", maxEventLimit))
	}
	return limit, nil
}

// mapPostgresReadError 将 PostgreSQL 读取错误转换为统一应用错误。
func mapPostgresReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.New(apperrors.CodeNotFound, "任务或事件不存在")
	}
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库读取失败", err)
}

// mapPostgresWriteError 将 PostgreSQL 写入错误转换为统一应用错误。
func mapPostgresWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return apperrors.New(apperrors.CodeNotFound, "任务不存在")
		case "23505":
			return apperrors.New(apperrors.CodeConflict, "事件资源已存在")
		}
	}
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库写入失败", err)
}
