package llmgateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"stableagent/internal/domain/runtimeplan"
	apperrors "stableagent/pkg/errors"
)

// flakyProvider 先失败 failures 次再成功,用于验证重试。
type flakyProvider struct {
	failures  int
	calls     int
	retryable bool
}

func (p *flakyProvider) Name() string { return "flaky" }

func (p *flakyProvider) Chat(ctx context.Context, input runtimeplan.ChatRequest, prompt PromptTemplate) (runtimeplan.ChatResponse, error) {
	p.calls++
	if p.calls <= p.failures {
		err := errors.New("上游临时错误")
		if p.retryable {
			return runtimeplan.ChatResponse{}, MarkRetryable(err)
		}
		return runtimeplan.ChatResponse{}, err
	}
	return runtimeplan.ChatResponse{Model: p.Name(), Content: "ok", IsFinal: true}, nil
}

// newTestService 创建禁用真实 sleep 的测试服务。
func newTestService(provider Provider, cfg Config) *Service {
	service := NewServiceWith(provider, cfg)
	service.sleep = func(time.Duration) {}
	return service
}

func TestChatRetriesRetryableError(t *testing.T) {
	provider := &flakyProvider{failures: 2, retryable: true}
	service := newTestService(provider, Config{MaxRetries: 2})
	resp, err := service.Chat(context.Background(), runtimeplan.ChatRequest{TenantID: "t1", Goal: "目标"})
	if err != nil {
		t.Fatalf("可重试错误应在重试后成功: %v", err)
	}
	if provider.calls != 3 {
		t.Fatalf("应调用 3 次(1 次 + 2 次重试),实际 %d", provider.calls)
	}
	if resp.PromptVersion != activePromptVersion {
		t.Fatalf("响应必须回填 prompt 版本,实际 %q", resp.PromptVersion)
	}
}

func TestChatDoesNotRetryNonRetryable(t *testing.T) {
	provider := &flakyProvider{failures: 5, retryable: false}
	service := newTestService(provider, Config{MaxRetries: 3})
	_, err := service.Chat(context.Background(), runtimeplan.ChatRequest{TenantID: "t1", Goal: "目标"})
	if err == nil {
		t.Fatal("不可重试错误应直接失败")
	}
	if provider.calls != 1 {
		t.Fatalf("不可重试错误不应重试,实际调用 %d 次", provider.calls)
	}
}

func TestChatRetriesExhausted(t *testing.T) {
	provider := &flakyProvider{failures: 10, retryable: true}
	service := newTestService(provider, Config{MaxRetries: 2})
	_, err := service.Chat(context.Background(), runtimeplan.ChatRequest{TenantID: "t1", Goal: "目标"})
	if err == nil {
		t.Fatal("重试耗尽后应返回错误")
	}
	appErr := apperrors.From(err)
	if appErr.Code != apperrors.CodeUnavailable {
		t.Fatalf("重试耗尽应返回 SERVICE_UNAVAILABLE,实际 %s", appErr.Code)
	}
}

func TestChatTenantRateLimit(t *testing.T) {
	service := newTestService(NewMockProvider(), Config{RatePerMinute: 1})
	if _, err := service.Chat(context.Background(), runtimeplan.ChatRequest{TenantID: "t1", Goal: "目标"}); err != nil {
		t.Fatalf("首次调用不应被限流: %v", err)
	}
	_, err := service.Chat(context.Background(), runtimeplan.ChatRequest{TenantID: "t1", Goal: "目标"})
	if err == nil {
		t.Fatal("超过阈值应被限流")
	}
	if apperrors.From(err).Code != apperrors.CodeRateLimited {
		t.Fatalf("限流应返回 RATE_LIMITED,实际 %s", apperrors.From(err).Code)
	}
	// 其他租户独立桶,不受影响。
	if _, err := service.Chat(context.Background(), runtimeplan.ChatRequest{TenantID: "t2", Goal: "目标"}); err != nil {
		t.Fatalf("其他租户不应被限流: %v", err)
	}
}

func TestChatRejectsEmptyGoal(t *testing.T) {
	service := newTestService(NewMockProvider(), Config{})
	_, err := service.Chat(context.Background(), runtimeplan.ChatRequest{TenantID: "t1", Goal: "  "})
	if err == nil || apperrors.From(err).Code != apperrors.CodeInvalidArg {
		t.Fatalf("空 goal 应返回 INVALID_ARGUMENT: %v", err)
	}
}
