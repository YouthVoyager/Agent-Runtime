package eventapi

import (
	"sync"

	domainevent "stableagent/internal/domain/event"
)

const DefaultHubBuffer = 64

type eventKey struct {
	tenantID string
	taskID   string
}

type Hub struct {
	mu     sync.Mutex
	buffer int
	subs   map[eventKey]map[*Subscription]struct{}
}

type Subscription struct {
	hub *Hub
	key eventKey
	ch  chan domainevent.AgentEvent
}

// NewHub 创建进程内事件推送中心，buffer 控制单客户端待发送事件上限。
func NewHub(buffer int) *Hub {
	if buffer <= 0 {
		buffer = DefaultHubBuffer
	}
	return &Hub{
		buffer: buffer,
		subs:   make(map[eventKey]map[*Subscription]struct{}),
	}
}

// Subscribe 订阅指定任务的事件推送。
func (h *Hub) Subscribe(tenantID string, taskID string) *Subscription {
	sub := &Subscription{
		hub: h,
		key: eventKey{tenantID: tenantID, taskID: taskID},
		ch:  make(chan domainevent.AgentEvent, h.buffer),
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[sub.key] == nil {
		h.subs[sub.key] = make(map[*Subscription]struct{})
	}
	h.subs[sub.key][sub] = struct{}{}
	return sub
}

// Publish 将事件推送给当前实例订阅该任务的客户端，慢客户端会被主动移除。
func (h *Hub) Publish(event domainevent.AgentEvent) {
	key := eventKey{tenantID: event.TenantID, taskID: event.TaskID}

	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs[key] {
		select {
		case sub.ch <- event:
		default:
			// 订阅者 channel 已满说明客户端消费过慢，主动断开后由客户端重连补拉。
			h.removeLocked(sub)
		}
	}
}

// C 返回订阅事件 channel。
func (s *Subscription) C() <-chan domainevent.AgentEvent {
	return s.ch
}

// Close 取消订阅并关闭 channel。
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.hub.removeLocked(s)
}

// removeLocked 在持锁状态下移除订阅者。
func (h *Hub) removeLocked(sub *Subscription) {
	items := h.subs[sub.key]
	if items == nil {
		return
	}
	if _, ok := items[sub]; !ok {
		return
	}
	delete(items, sub)
	close(sub.ch)
	if len(items) == 0 {
		delete(h.subs, sub.key)
	}
}
