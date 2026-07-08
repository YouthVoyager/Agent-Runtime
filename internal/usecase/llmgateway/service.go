package llmgateway

import (
	"context"
	"strings"
	"time"

	"stableagent/internal/domain/runtimeplan"
	apperrors "stableagent/pkg/errors"
)

// Config 定义 LLM Gateway 的生产级控制参数。
type Config struct {
	// Timeout 单次 provider 调用超时。
	Timeout time.Duration
	// MaxRetries 可重试错误的最大重试次数(不含首次调用)。
	MaxRetries int
	// RatePerMinute 每租户每分钟请求上限,<= 0 不限流。
	RatePerMinute int
}

type Service struct {
	provider Provider
	limiter  *tenantRateLimiter
	timeout  time.Duration
	retries  int
	sleep    func(time.Duration)
}

// NewService 创建 LLM Gateway 用例服务,默认使用确定性 Mock Provider。
func NewService() *Service {
	return NewServiceWith(NewMockProvider(), Config{})
}

// NewServiceWith 以指定 Provider 和配置创建服务,生产接入真实模型时使用。
func NewServiceWith(provider Provider, cfg Config) *Service {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}
	if cfg.RatePerMinute == 0 {
		cfg.RatePerMinute = 600
	}
	return &Service{
		provider: provider,
		limiter:  newTenantRateLimiter(cfg.RatePerMinute, nil),
		timeout:  cfg.Timeout,
		retries:  cfg.MaxRetries,
		sleep:    time.Sleep,
	}
}

// Chat 统一入口:限流、超时、重试后调用 Provider,并回填 prompt 版本。
func (s *Service) Chat(ctx context.Context, input runtimeplan.ChatRequest) (runtimeplan.ChatResponse, error) {
	if strings.TrimSpace(input.Goal) == "" {
		return runtimeplan.ChatResponse{}, apperrors.New(apperrors.CodeInvalidArg, "goal 不能为空")
	}
	if !s.limiter.Allow(input.TenantID) {
		return runtimeplan.ChatResponse{}, apperrors.New(apperrors.CodeRateLimited, "租户 LLM 请求超过限流阈值")
	}
	prompt := ActivePrompt()

	var lastErr error
	for attempt := 0; attempt <= s.retries; attempt++ {
		if attempt > 0 {
			// 指数退避:100ms、200ms、400ms……
			s.sleep(time.Duration(100<<(attempt-1)) * time.Millisecond)
		}
		callCtx, cancel := context.WithTimeout(ctx, s.timeout)
		resp, err := s.provider.Chat(callCtx, input, prompt)
		cancel()
		if err == nil {
			if resp.PromptVersion == "" {
				resp.PromptVersion = prompt.Version
			}
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil || !IsRetryable(err) {
			break
		}
	}
	return runtimeplan.ChatResponse{}, apperrors.Wrap(apperrors.CodeUnavailable, "LLM provider 调用失败", lastErr)
}
