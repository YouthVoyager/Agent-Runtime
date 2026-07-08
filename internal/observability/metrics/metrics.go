// Package metrics 提供基于 prometheus/client_golang 的统一指标注册。
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ServiceUp 保持与旧手写 /metrics 相同的存活指标。
	ServiceUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stableagent_service_up",
		Help: "Whether the service process is up.",
	}, []string{"service", "env"})

	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stableagent_http_requests_total",
		Help: "HTTP 请求总数。",
	}, []string{"service", "method", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stableagent_http_request_duration_seconds",
		Help:    "HTTP 请求耗时分布。",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method"})

	// TasksFinishedTotal 按终态统计任务结果,监控成功率。
	TasksFinishedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stableagent_tasks_finished_total",
		Help: "任务进入终态的总数,按状态区分。",
	}, []string{"status"})

	// StepsTotal 统计执行的 agent step 数。
	StepsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stableagent_agent_steps_total",
		Help: "已提交完成的 agent step 总数。",
	})

	// LLMCallDuration 统计 LLM Gateway 调用耗时。
	LLMCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stableagent_llm_call_duration_seconds",
		Help:    "Worker 调用 LLM Gateway 的耗时分布。",
		Buckets: prometheus.DefBuckets,
	}, []string{"result"})

	// LLMTokensTotal 统计 LLM token 消耗。
	LLMTokensTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stableagent_llm_tokens_total",
		Help: "LLM 调用消耗的 token 总数。",
	})

	// ToolCallDuration 统计工具调用耗时。
	ToolCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stableagent_tool_call_duration_seconds",
		Help:    "Worker 调用 Tool Gateway 的耗时分布。",
		Buckets: prometheus.DefBuckets,
	}, []string{"tool", "result"})

	// OutboxDispatchTotal 统计 outbox 派发结果。
	OutboxDispatchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stableagent_outbox_dispatch_total",
		Help: "outbox 消息派发结果总数。",
	}, []string{"result"})

	// ApprovalsExpiredTotal 统计自动过期的审批数。
	ApprovalsExpiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stableagent_approvals_expired_total",
		Help: "超时自动过期的审批总数。",
	})
)

// Handler 返回 Prometheus 文本协议的 /metrics 处理器。
func Handler() http.Handler {
	return promhttp.Handler()
}

// ObserveHTTPRequest 记录一次 HTTP 请求。
func ObserveHTTPRequest(service string, method string, status int, duration time.Duration) {
	httpRequestsTotal.WithLabelValues(service, method, strconv.Itoa(status)).Inc()
	httpRequestDuration.WithLabelValues(service, method).Observe(duration.Seconds())
}
