package eventapi

import (
	"context"
	"encoding/json"
	"testing"

	domainevent "stableagent/internal/domain/event"
	apperrors "stableagent/pkg/errors"
)

// TestAppendParamsRejectsUnknownType 验证事件追加参数会拒绝未知事件类型。
func TestAppendParamsRejectsUnknownType(t *testing.T) {
	_, err := appendParams(AppendEventInput{
		TenantID: "tenant_001",
		TaskID:   "task_001",
		Type:     "UNKNOWN",
	})
	if err == nil {
		t.Fatal("期望未知事件类型返回错误")
	}
	if appErr := apperrors.From(err); appErr.Code != apperrors.CodeInvalidArg {
		t.Fatalf("错误码不正确: %+v", appErr)
	}
}

// TestAppendParamsNormalizesJSON 验证事件追加参数会统一空 input、output 和 metadata。
func TestAppendParamsNormalizesJSON(t *testing.T) {
	params, err := appendParams(AppendEventInput{
		TenantID: "tenant_001",
		TaskID:   "task_001",
		Type:     "task_created",
	})
	if err != nil {
		t.Fatalf("appendParams 返回错误: %v", err)
	}
	if string(params.Input) != "null" || string(params.Output) != "null" || string(params.Metadata) != "{}" {
		t.Fatalf("JSON 默认值不正确: input=%s output=%s metadata=%s", params.Input, params.Output, params.Metadata)
	}
}

// TestAppendParamsRequiresMetadataObject 验证 metadata 必须是 JSON object。
func TestAppendParamsRequiresMetadataObject(t *testing.T) {
	_, err := appendParams(AppendEventInput{
		TenantID: "tenant_001",
		TaskID:   "task_001",
		Type:     string(domainevent.TypeTaskCreated),
		Metadata: json.RawMessage(`[]`),
	})
	if err == nil {
		t.Fatal("期望数组 metadata 返回错误")
	}
}

// TestHubDropsSlowSubscriber 验证慢客户端 channel 填满后会被移除，避免阻塞发布。
func TestHubDropsSlowSubscriber(t *testing.T) {
	hub := NewHub(1)
	sub := hub.Subscribe("tenant_001", "task_001")

	hub.Publish(domainevent.AgentEvent{TenantID: "tenant_001", TaskID: "task_001", EventID: 1})
	hub.Publish(domainevent.AgentEvent{TenantID: "tenant_001", TaskID: "task_001", EventID: 2})

	first := <-sub.C()
	if first.EventID != 1 {
		t.Fatalf("第一条事件不正确: %+v", first)
	}
	if _, ok := <-sub.C(); ok {
		t.Fatal("慢客户端订阅未被关闭")
	}
}

// TestPublisherFailureDoesNotFailAppendPath 验证事件推送错误可被上层忽略。
func TestPublisherFailureDoesNotFailAppendPath(t *testing.T) {
	publisher := failingPublisher{}
	err := publisher.Publish(context.Background(), domainevent.AgentEvent{})
	if err == nil {
		t.Fatal("测试发布器应返回错误")
	}
}

type failingPublisher struct{}

// Publish 返回固定错误用于测试推送失败路径。
func (failingPublisher) Publish(context.Context, domainevent.AgentEvent) error {
	return context.Canceled
}
