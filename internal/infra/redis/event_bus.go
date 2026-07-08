package redis

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	redisclient "github.com/redis/go-redis/v9"

	domainevent "agent-runtime/internal/domain/event"
)

const AgentEventChannel = "agent-runtime:events"

type EventBus struct {
	client  *redisclient.Client
	channel string
	logger  *slog.Logger
}

type eventMessage struct {
	EventID   int64           `json:"event_id"`
	TaskID    string          `json:"task_id"`
	TenantID  string          `json:"tenant_id"`
	Type      string          `json:"type"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	Metadata  json.RawMessage `json:"metadata"`
	TraceID   *string         `json:"trace_id,omitempty"`
	SpanID    *string         `json:"span_id,omitempty"`
	CreatedAt string          `json:"created_at"`
}

// NewEventBus 创建 Redis Pub/Sub 事件总线。
func NewEventBus(addr string, logger *slog.Logger) *EventBus {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &EventBus{
		client: redisclient.NewClient(&redisclient.Options{
			Addr: addr,
		}),
		channel: AgentEventChannel,
		logger:  logger,
	}
}

// Publish 将 AgentEvent 发布到 Redis，供其他 API 实例转发给 SSE 客户端。
func (b *EventBus) Publish(ctx context.Context, event domainevent.AgentEvent) error {
	if b == nil || b.client == nil {
		return nil
	}
	payload, err := json.Marshal(messageFromEvent(event))
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.channel, payload).Err()
}

// Subscribe 持续订阅 Redis 事件通道，并把消息交给 handler。
func (b *EventBus) Subscribe(ctx context.Context, handler func(domainevent.AgentEvent)) error {
	if b == nil || b.client == nil {
		return nil
	}
	sub := b.client.Subscribe(ctx, b.channel)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			event, err := eventFromMessage([]byte(msg.Payload))
			if err != nil {
				b.logger.Warn("解析 Redis AgentEvent 消息失败", "error", err)
				continue
			}
			handler(event)
		}
	}
}

// Close 关闭 Redis 客户端连接。
func (b *EventBus) Close() error {
	if b == nil || b.client == nil {
		return nil
	}
	return b.client.Close()
}

// messageFromEvent 转换为包含 tenant_id 的 Redis 消息结构。
func messageFromEvent(event domainevent.AgentEvent) eventMessage {
	return eventMessage{
		EventID:   event.EventID,
		TaskID:    event.TaskID,
		TenantID:  event.TenantID,
		Type:      event.Type,
		Input:     event.Input,
		Output:    event.Output,
		Metadata:  event.Metadata,
		TraceID:   event.TraceID,
		SpanID:    event.SpanID,
		CreatedAt: event.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

// eventFromMessage 将 Redis 消息转换回领域事件。
func eventFromMessage(payload []byte) (domainevent.AgentEvent, error) {
	var msg eventMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return domainevent.AgentEvent{}, err
	}
	return domainevent.AgentEvent{
		EventID:   msg.EventID,
		TaskID:    msg.TaskID,
		TenantID:  msg.TenantID,
		Type:      msg.Type,
		Input:     domainevent.NormalizeJSONValue(msg.Input),
		Output:    domainevent.NormalizeJSONValue(msg.Output),
		Metadata:  domainevent.NormalizeMetadata(msg.Metadata),
		TraceID:   msg.TraceID,
		SpanID:    msg.SpanID,
		CreatedAt: parseMessageTime(msg.CreatedAt),
	}, nil
}

// parseMessageTime 解析 Redis 消息时间，异常时返回零值避免阻断推送。
func parseMessageTime(raw string) time.Time {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return value.UTC()
}
