package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"stableagent/internal/config"
	"stableagent/internal/infra/postgres/db"
	"stableagent/internal/security/authn"
	"stableagent/internal/security/authz"
	httpserver "stableagent/internal/transport/http"
	"stableagent/internal/usecase/auditlog"
	apperrors "stableagent/pkg/errors"
	"stableagent/pkg/ids"
	"stableagent/pkg/jsonx"
)

const (
	defaultAdminPageSize = 20
	maxAdminPageSize     = 100
	inlineArtifactLimit  = 1 << 20
)

type adminHandler struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	cfg     config.Config
	audit   *auditlog.Service
	logger  *slog.Logger
}

type pageEnvelope struct {
	Items    any  `json:"items"`
	Page     int  `json:"page"`
	PageSize int  `json:"page_size"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page,omitempty"`
}

type tenantUsage struct {
	TasksToday   int64   `json:"tasks_today"`
	TokensToday  int64   `json:"tokens_today"`
	CostTodayUSD float64 `json:"cost_today_usd"`
}

type tenantItem struct {
	TenantID  string          `json:"tenant_id"`
	Name      string          `json:"name"`
	Status    string          `json:"status"`
	Config    json.RawMessage `json:"config"`
	Usage     tenantUsage     `json:"usage"`
	CreatedAt time.Time       `json:"created_at"`
}

type userItem struct {
	UserID      string     `json:"user_id"`
	Email       string     `json:"email"`
	Name        *string    `json:"name,omitempty"`
	TenantID    string     `json:"tenant_id"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type toolItem struct {
	ToolName               string          `json:"tool_name"`
	Description            string          `json:"description"`
	Category               string          `json:"category"`
	DefaultRiskLevel       string          `json:"default_risk_level"`
	Enabled                bool            `json:"enabled"`
	Version                string          `json:"version"`
	Owner                  string          `json:"owner"`
	HasSideEffect          bool            `json:"has_side_effect"`
	RequiresIdempotencyKey bool            `json:"requires_idempotency_key"`
	TimeoutSeconds         int32           `json:"timeout_seconds"`
	ParamsSchema           json.RawMessage `json:"params_schema,omitempty"`
	ResultSchema           json.RawMessage `json:"result_schema,omitempty"`
	RetryPolicy            json.RawMessage `json:"retry_policy,omitempty"`
	Stats                  any             `json:"stats,omitempty"`
	Calls7D                int64           `json:"calls_7d,omitempty"`
	ErrorRate7D            float64         `json:"error_rate_7d,omitempty"`
	AvgDurationMs7D        float64         `json:"avg_duration_ms_7d,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

type availableToolItem struct {
	ToolName         string `json:"tool_name"`
	Description      string `json:"description"`
	RiskLevel        string `json:"risk_level"`
	ApprovalRequired bool   `json:"approval_required"`
}

type toolPolicyItem struct {
	PolicyID          string          `json:"policy_id"`
	TenantID          string          `json:"tenant_id"`
	ToolName          string          `json:"tool_name"`
	Role              string          `json:"role"`
	Enabled           bool            `json:"enabled"`
	ApprovalRequired  bool            `json:"approval_required"`
	MaxCallsPerDay    *int32          `json:"max_calls_per_day,omitempty"`
	RiskLevelOverride string          `json:"risk_level_override"`
	ArgumentRules     json.RawMessage `json:"argument_rules"`
	TimeoutSeconds    *int32          `json:"timeout_seconds,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type approvalItem struct {
	ApprovalID     string     `json:"approval_id"`
	TaskID         string     `json:"task_id"`
	CallID         string     `json:"call_id"`
	ToolName       string     `json:"tool_name"`
	RiskLevel      string     `json:"risk_level"`
	ApprovalReason *string    `json:"approval_reason,omitempty"`
	RequesterID    string     `json:"requester_id"`
	ApproverID     *string    `json:"approver_id,omitempty"`
	Status         string     `json:"status"`
	Note           *string    `json:"note,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type approvalDetail struct {
	approvalItem
	Arguments      json.RawMessage `json:"arguments"`
	TaskGoal       string          `json:"task_goal"`
	CurrentStep    int32           `json:"current_step"`
	IdempotencyKey string          `json:"idempotency_key"`
	History        []approvalItem  `json:"history"`
}

type toolCallCrossItem struct {
	CallID         string         `json:"call_id"`
	TaskID         string         `json:"task_id"`
	TenantID       string         `json:"tenant_id"`
	ToolName       string         `json:"tool_name"`
	Status         string         `json:"status"`
	RiskLevel      string         `json:"risk_level"`
	ApprovalStatus string         `json:"approval_status"`
	UserID         string         `json:"user_id"`
	DurationMs     *float64       `json:"duration_ms,omitempty"`
	ErrorCode      *string        `json:"error_code,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
	CreatedAt      time.Time      `json:"created_at"`
	ArgumentsHash  string         `json:"arguments_hash,omitempty"`
	Arguments      any            `json:"arguments,omitempty"`
	Result         any            `json:"result,omitempty"`
	ErrorMessage   *string        `json:"error_message,omitempty"`
	Approvals      []approvalItem `json:"approvals,omitempty"`
	TraceID        *string        `json:"trace_id,omitempty"`
}

type artifactItem struct {
	ArtifactID       string          `json:"artifact_id"`
	TaskID           string          `json:"task_id"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	SizeBytes        int64           `json:"size_bytes"`
	CreatedByEventID *int64          `json:"created_by_event_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
	Content          json.RawMessage `json:"content,omitempty"`
	DownloadURL      *string         `json:"download_url,omitempty"`
}

type eventExplorerItem struct {
	EventID       int64           `json:"event_id"`
	TaskID        string          `json:"task_id"`
	Type          string          `json:"type"`
	InputSummary  string          `json:"input_summary"`
	OutputSummary string          `json:"output_summary"`
	Metadata      json.RawMessage `json:"metadata"`
	TraceID       *string         `json:"trace_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type logItem struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Service   string    `json:"service"`
	Message   string    `json:"message"`
	TaskID    *string   `json:"task_id,omitempty"`
	TraceID   *string   `json:"trace_id,omitempty"`
	TenantID  *string   `json:"tenant_id,omitempty"`
	ErrorCode *string   `json:"error_code,omitempty"`
}

type traceSpanItem struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Start      time.Time `json:"start"`
	DurationMs int64     `json:"duration_ms"`
	Status     string    `json:"status"`
}

type auditLogItem struct {
	AuditID      string          `json:"audit_id"`
	ActorID      string          `json:"actor_id"`
	TenantID     *string         `json:"tenant_id,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   *string         `json:"resource_id,omitempty"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	IP           *string         `json:"ip,omitempty"`
	UserAgent    *string         `json:"user_agent,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

type taskTemplateItem struct {
	TemplateID string          `json:"template_id"`
	Name       string          `json:"name"`
	Payload    json.RawMessage `json:"payload"`
	Shared     bool            `json:"shared"`
	CreatedBy  string          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
}

// registerAdminConsoleRoutes 注册管理台契约新增接口，并统一挂载 JWT 鉴权。
func registerAdminConsoleRoutes(router chi.Router, pool *pgxpool.Pool, cfg config.Config, jwtSecret string, logger *slog.Logger, auditLogger *auditlog.Service) {
	handler := &adminHandler{
		pool:    pool,
		queries: db.New(pool),
		cfg:     cfg,
		audit:   auditLogger,
		logger:  logger,
	}
	jwtMiddleware := authn.JWTMiddleware(authn.JWTMiddlewareConfig{Secret: jwtSecret})

	router.Group(func(r chi.Router) {
		r.Use(jwtMiddleware)
		r.Get("/api/v1/dashboard/summary", handler.dashboardSummary)
		r.Get("/api/v1/dashboard/task-stats", handler.dashboardTaskStats)
		r.Get("/api/v1/dashboard/cost-stats", handler.dashboardCostStats)
		r.Get("/api/v1/dashboard/risk-stats", handler.dashboardRiskStats)
		r.Get("/api/v1/dashboard/system-health", handler.dashboardSystemHealth)

		r.Get("/api/v1/approvals", handler.listApprovals)
		r.Get("/api/v1/approvals/pending", handler.listPendingApprovals)
		r.Get("/api/v1/approvals/{approval_id}", handler.getApproval)

		r.Get("/api/v1/tools", handler.listTools)
		r.Get("/api/v1/tools/available", handler.listAvailableTools)
		r.Get("/api/v1/tools/{tool_name}", handler.getTool)
		r.Patch("/api/v1/tools/{tool_name}", handler.updateTool)

		r.Get("/api/v1/tool-policies", handler.listToolPolicies)
		r.Post("/api/v1/tool-policies", handler.createToolPolicy)
		r.Post("/api/v1/tool-policies/simulate", handler.simulateToolPolicy)
		r.Get("/api/v1/tool-policies/{policy_id}", handler.getToolPolicy)
		r.Put("/api/v1/tool-policies/{policy_id}", handler.updateToolPolicy)
		r.Delete("/api/v1/tool-policies/{policy_id}", handler.deleteToolPolicy)

		r.Get("/api/v1/tool-calls", handler.listToolCalls)
		r.Get("/api/v1/tool-calls/{call_id}", handler.getToolCall)

		r.Get("/api/v1/events", handler.listEvents)
		r.Get("/api/v1/observability/trace/{trace_id}", handler.getTrace)
		r.Get("/api/v1/observability/metrics", handler.getMetrics)
		r.Get("/api/v1/observability/logs", handler.listLogs)

		r.Get("/api/v1/artifacts", handler.listArtifacts)
		r.Get("/api/v1/artifacts/{artifact_id}", handler.getArtifact)
		r.Get("/api/v1/artifacts/{artifact_id}/download", handler.downloadArtifact)
		r.Delete("/api/v1/artifacts/{artifact_id}", handler.deleteArtifact)

		r.Get("/api/v1/tenants", handler.listTenants)
		r.Post("/api/v1/tenants", handler.createTenant)
		r.Get("/api/v1/tenants/{tenant_id}", handler.getTenant)
		r.Patch("/api/v1/tenants/{tenant_id}", handler.updateTenant)

		r.Get("/api/v1/users", handler.listUsers)
		r.Post("/api/v1/users", handler.inviteUser)
		r.Patch("/api/v1/users/{user_id}", handler.updateUser)

		r.Get("/api/v1/settings", handler.getSettings)
		r.Put("/api/v1/settings/{section}", handler.updateSettings)
		r.Get("/api/v1/audit-logs", handler.listAuditLogs)

		r.Get("/api/v1/task-templates", handler.listTaskTemplates)
		r.Post("/api/v1/task-templates", handler.createTaskTemplate)
		r.Delete("/api/v1/task-templates/{template_id}", handler.deleteTaskTemplate)
	})
}

// dashboardSummary 返回管理台总览核心任务统计。
func (h *adminHandler) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	conditions, args := h.taskScope(user, r.URL.Query().Get("tenant_id"), "agent_tasks")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	todayArg := addArg(&args, today)
	sql := fmt.Sprintf(`
select
    count(*)::bigint,
    count(*) filter (where created_at >= %s)::bigint,
    count(*) filter (where status = 'RUNNING')::bigint,
    count(*) filter (where status = 'WAITING_APPROVAL')::bigint,
    count(*) filter (where status = 'FAILED')::bigint,
    count(*) filter (where status = 'SUCCEEDED')::bigint,
    count(*) filter (where status = 'STOPPED_BY_LIMIT')::bigint,
    count(*) filter (where status = 'CANCELED')::bigint,
    coalesce(avg(extract(epoch from (updated_at - created_at))) filter (where status in ('SUCCEEDED','FAILED','CANCELED')), 0)::float8
from agent_tasks
%s`, todayArg, whereSQL(conditions))
	var total, createdToday, running, waiting, failed, completed, stopped, cancelled int64
	var avgDuration float64
	if err := h.pool.QueryRow(r.Context(), sql, args...).Scan(&total, &createdToday, &running, &waiting, &failed, &completed, &stopped, &cancelled, &avgDuration); err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	successRate := 0.0
	if total > 0 {
		successRate = float64(completed) / float64(total)
	}
	httpserver.WriteData(w, http.StatusOK, map[string]any{
		"total_tasks":          total,
		"created_today":        createdToday,
		"running":              running,
		"waiting_approval":     waiting,
		"failed":               failed,
		"completed":            completed,
		"stopped_by_limit":     stopped,
		"cancelled":            cancelled,
		"success_rate":         successRate,
		"avg_duration_seconds": avgDuration,
	})
}

// dashboardTaskStats 返回任务趋势和状态分布。
func (h *adminHandler) dashboardTaskStats(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	from, _, bucketExpr := parseDashboardRange(r)
	conditions, args := h.taskScope(user, r.URL.Query().Get("tenant_id"), "agent_tasks")
	conditions = append(conditions, fmt.Sprintf("agent_tasks.created_at >= %s", addArg(&args, from)))
	sql := fmt.Sprintf(`
select date_trunc('%s', agent_tasks.created_at) as bucket,
       count(*)::bigint,
       count(*) filter (where status = 'SUCCEEDED')::bigint,
       count(*) filter (where status = 'FAILED')::bigint,
       coalesce(sum((budget_usage ->> 'used_tokens')::bigint), 0)::bigint,
       coalesce(sum((budget_usage ->> 'used_cost_usd')::float8), 0)::float8
from agent_tasks
%s
group by bucket
order by bucket asc`, bucketExpr, whereSQL(conditions))
	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	defer rows.Close()
	buckets := make([]map[string]any, 0)
	for rows.Next() {
		var bucket pgtype.Timestamptz
		var created, completed, failed, tokens int64
		var cost float64
		if err := rows.Scan(&bucket, &created, &completed, &failed, &tokens, &cost); err != nil {
			httpserver.WriteError(w, r, dbReadError(err))
			return
		}
		buckets = append(buckets, map[string]any{
			"time":      pgTime(bucket),
			"created":   created,
			"completed": completed,
			"failed":    failed,
			"tokens":    tokens,
			"cost_usd":  cost,
		})
	}
	statusDistribution, err := h.taskStatusDistribution(r.Context(), user, r.URL.Query().Get("tenant_id"))
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	httpserver.WriteData(w, http.StatusOK, map[string]any{
		"buckets":             buckets,
		"status_distribution": statusDistribution,
	})
}

// dashboardCostStats 返回 token、成本和工具调用聚合。
func (h *adminHandler) dashboardCostStats(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	from := time.Now().UTC().Truncate(24 * time.Hour)
	taskConditions, taskArgs := h.taskScope(user, r.URL.Query().Get("tenant_id"), "agent_tasks")
	taskConditions = append(taskConditions, fmt.Sprintf("agent_tasks.created_at >= %s", addArg(&taskArgs, from)))
	taskSQL := fmt.Sprintf(`
select coalesce(sum((budget_usage ->> 'used_tokens')::bigint), 0)::bigint,
       coalesce(sum((budget_usage ->> 'used_cost_usd')::float8), 0)::float8,
       coalesce(avg((budget_usage ->> 'used_cost_usd')::float8), 0)::float8
from agent_tasks
%s`, whereSQL(taskConditions))
	var tokensToday int64
	var costToday, avgCost float64
	if err := h.pool.QueryRow(r.Context(), taskSQL, taskArgs...).Scan(&tokensToday, &costToday, &avgCost); err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}

	toolConditions, toolArgs := h.toolCallScope(user, r.URL.Query().Get("tenant_id"), "tool_calls")
	toolConditions = append(toolConditions, fmt.Sprintf("tool_calls.created_at >= %s", addArg(&toolArgs, from)))
	toolSQL := fmt.Sprintf(`
select count(*)::bigint,
       count(*) filter (where risk_level in ('HIGH','CRITICAL'))::bigint,
       count(*) filter (where approval_status in ('REJECTED','EXPIRED'))::bigint
from tool_calls
%s`, whereSQL(toolConditions))
	var toolCalls, highRisk, blocked int64
	if err := h.pool.QueryRow(r.Context(), toolSQL, toolArgs...).Scan(&toolCalls, &highRisk, &blocked); err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	httpserver.WriteData(w, http.StatusOK, map[string]any{
		"tokens_today":          tokensToday,
		"cost_today_usd":        costToday,
		"avg_cost_per_task_usd": avgCost,
		"tool_calls_total":      toolCalls,
		"high_risk_tool_calls":  highRisk,
		"approval_blocked":      blocked,
	})
}

// dashboardRiskStats 返回审批、循环和超限风险聚合。
func (h *adminHandler) dashboardRiskStats(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	pending, rejected, err := h.approvalRiskCounts(r.Context(), user, r.URL.Query().Get("tenant_id"))
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	taskConditions, taskArgs := h.taskScope(user, r.URL.Query().Get("tenant_id"), "agent_tasks")
	taskSQL := fmt.Sprintf(`
select count(*) filter (where last_error_code = 'LOOP_DETECTED')::bigint,
       count(*) filter (where status = 'STOPPED_BY_LIMIT' and coalesce(last_error_code, '') ilike '%%TOKEN%%')::bigint,
       count(*) filter (where status = 'STOPPED_BY_LIMIT' and coalesce(last_error_code, '') ilike '%%STEP%%')::bigint
from agent_tasks
%s`, whereSQL(taskConditions))
	var loopDetected, tokenLimit, stepLimit int64
	if err := h.pool.QueryRow(r.Context(), taskSQL, taskArgs...).Scan(&loopDetected, &tokenLimit, &stepLimit); err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	riskDistribution, err := h.toolRiskDistribution(r.Context(), user, r.URL.Query().Get("tenant_id"))
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	trend, err := h.highRiskTrend(r.Context(), user, r.URL.Query().Get("tenant_id"))
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	httpserver.WriteData(w, http.StatusOK, map[string]any{
		"pending_approvals":   pending,
		"rejected_tool_calls": rejected,
		"loop_detected_tasks": loopDetected,
		"token_limit_tasks":   tokenLimit,
		"step_limit_tasks":    stepLimit,
		"high_risk_trend":     trend,
		"risk_distribution":   riskDistribution,
	})
}

// dashboardSystemHealth 返回轻量健康快照,外部依赖不可探测时使用 degraded 明确表达。
func (h *adminHandler) dashboardSystemHealth(w http.ResponseWriter, r *http.Request) {
	_, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	dbStatus := "healthy"
	dbDetail := ""
	if err := h.pool.Ping(r.Context()); err != nil {
		dbStatus = "down"
		dbDetail = err.Error()
	}
	services := []map[string]any{
		{"name": "api-service", "status": "healthy", "detail": h.cfg.ServiceName},
		{"name": "runtime-worker", "status": "degraded", "detail": "请通过 runtime-worker /runtime/v1/status 验证"},
		{"name": "tool-gateway", "status": "degraded", "detail": h.cfg.ToolGatewayURL},
		{"name": "llm-gateway", "status": "degraded", "detail": h.cfg.LLMGatewayURL},
		{"name": "postgresql", "status": dbStatus, "detail": dbDetail},
		{"name": "redis", "status": optionalStatus(h.cfg.RedisAddr), "detail": h.cfg.RedisAddr},
		{"name": "temporal", "status": "degraded", "detail": "本地轻量探测未连接 Temporal"},
	}
	var backlog int64
	_ = h.pool.QueryRow(r.Context(), `select count(*)::bigint from task_outbox where status in ('PENDING','FAILED')`).Scan(&backlog)
	httpserver.WriteData(w, http.StatusOK, map[string]any{
		"services":       services,
		"queue_backlog":  backlog,
		"active_workers": nil,
		"checked_at":     time.Now().UTC(),
	})
}

// listApprovals 返回审批中心分页列表。
func (h *adminHandler) listApprovals(w http.ResponseWriter, r *http.Request) {
	h.listApprovalsWithDefaultStatus(w, r, strings.TrimSpace(r.URL.Query().Get("status")))
}

// listPendingApprovals 兼容 /approvals/pending 别名。
func (h *adminHandler) listPendingApprovals(w http.ResponseWriter, r *http.Request) {
	h.listApprovalsWithDefaultStatus(w, r, "PENDING")
}

// listApprovalsWithDefaultStatus 执行审批列表查询并按角色收敛范围。
func (h *adminHandler) listApprovalsWithDefaultStatus(w http.ResponseWriter, r *http.Request, defaultStatus string) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	query := r.URL.Query()
	status := strings.ToUpper(strings.TrimSpace(defaultStatus))
	if status == "" {
		status = "PENDING"
	}
	conditions, args := h.approvalScope(user, query.Get("tenant_id"))
	conditions = append(conditions, fmt.Sprintf("approvals.status = %s", addArg(&args, status)))
	addOptionalTextFilter(&conditions, &args, "approvals.task_id", query.Get("task_id"))
	addOptionalTextFilter(&conditions, &args, "tool_calls.tool_name", query.Get("tool_name"))
	addOptionalTextFilter(&conditions, &args, "approvals.risk_level", strings.ToUpper(query.Get("risk_level")))
	if query.Get("mine") == "true" {
		conditions = append(conditions, fmt.Sprintf("approvals.approver_id = %s", addArg(&args, user.UserID)))
	}
	limitArg := addArg(&args, int32(pageSize+1))
	offsetArg := addArg(&args, int32((page-1)*pageSize))
	sql := fmt.Sprintf(`
select approvals.approval_id, approvals.task_id, approvals.call_id, coalesce(tool_calls.tool_name, ''),
       approvals.risk_level, approvals.approval_reason, coalesce(tool_calls.user_id, ''),
       approvals.approver_id, approvals.status, approvals.comment,
       approvals.created_at, approvals.decided_at, approvals.expires_at
from approvals
left join tool_calls on tool_calls.tenant_id = approvals.tenant_id and tool_calls.call_id = approvals.call_id
%s
order by approvals.created_at desc
limit %s offset %s`, whereSQL(conditions), limitArg, offsetArg)
	items, err := h.scanApprovalItems(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// getApproval 返回单条审批详情和同 call 的审批历史。
func (h *adminHandler) getApproval(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	approvalID := strings.TrimSpace(chi.URLParam(r, "approval_id"))
	conditions, args := h.approvalScope(user, "")
	conditions = append(conditions, fmt.Sprintf("approvals.approval_id = %s", addArg(&args, approvalID)))
	sql := fmt.Sprintf(`
select approvals.approval_id, approvals.task_id, approvals.call_id, coalesce(tool_calls.tool_name, ''),
       approvals.risk_level, approvals.approval_reason, coalesce(tool_calls.user_id, ''),
       approvals.approver_id, approvals.status, approvals.comment,
       approvals.created_at, approvals.decided_at, approvals.expires_at,
       coalesce(tool_calls.arguments, '{}'::jsonb), coalesce(agent_tasks.goal, ''),
       coalesce(agent_states.current_step, 0), coalesce(tool_calls.idempotency_key, '')
from approvals
left join tool_calls on tool_calls.tenant_id = approvals.tenant_id and tool_calls.call_id = approvals.call_id
left join agent_tasks on agent_tasks.tenant_id = approvals.tenant_id and agent_tasks.task_id = approvals.task_id
left join agent_states on agent_states.tenant_id = approvals.tenant_id and agent_states.task_id = approvals.task_id
%s
limit 1`, whereSQL(conditions))
	var item approvalItem
	var arguments json.RawMessage
	var goal string
	var currentStep int32
	var idempotencyKey string
	var createdAt, decidedAt, expiresAt pgtype.Timestamptz
	if err := h.pool.QueryRow(r.Context(), sql, args...).Scan(
		&item.ApprovalID, &item.TaskID, &item.CallID, &item.ToolName,
		&item.RiskLevel, &item.ApprovalReason, &item.RequesterID,
		&item.ApproverID, &item.Status, &item.Note,
		&createdAt, &decidedAt, &expiresAt,
		&arguments, &goal, &currentStep, &idempotencyKey,
	); err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	item.CreatedAt = pgTime(createdAt)
	item.DecidedAt = timePtrFromPG(decidedAt)
	item.ExpiresAt = timePtrFromPG(expiresAt)
	historySQL := `
select approvals.approval_id, approvals.task_id, approvals.call_id, coalesce(tool_calls.tool_name, ''),
       approvals.risk_level, approvals.approval_reason, coalesce(tool_calls.user_id, ''),
       approvals.approver_id, approvals.status, approvals.comment,
       approvals.created_at, approvals.decided_at, approvals.expires_at
from approvals
left join tool_calls on tool_calls.tenant_id = approvals.tenant_id and tool_calls.call_id = approvals.call_id
where approvals.tenant_id = $1 and approvals.call_id = $2
order by approvals.created_at asc`
	history, _ := h.scanApprovalItems(r.Context(), historySQL, user.TenantID, item.CallID)
	httpserver.WriteData(w, http.StatusOK, approvalDetail{
		approvalItem:   item,
		Arguments:      jsonx.RedactJSON(arguments),
		TaskGoal:       goal,
		CurrentStep:    currentStep,
		IdempotencyKey: idempotencyKey,
		History:        history,
	})
}

// listTools 返回工具注册表；普通用户只看到 enabled 摘要。
func (h *adminHandler) listTools(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	var tools []db.Tool
	var err error
	if authz.IsAdmin(user) || authz.IsAuditor(user) {
		tools, err = h.queries.ListTools(r.Context())
	} else {
		tools, err = h.queries.ListEnabledTools(r.Context())
	}
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	items := make([]toolItem, 0, len(tools))
	for _, tool := range tools {
		items = append(items, toolFromDB(tool, authz.IsAdmin(user) || authz.IsAuditor(user)))
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, 1, len(items), false))
}

// listAvailableTools 返回创建任务时可选择的工具摘要。
func (h *adminHandler) listAvailableTools(w http.ResponseWriter, r *http.Request) {
	_, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	tools, err := h.queries.ListEnabledTools(r.Context())
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	items := make([]availableToolItem, 0, len(tools))
	for _, tool := range tools {
		items = append(items, availableToolItem{
			ToolName:         tool.ToolName,
			Description:      tool.Description,
			RiskLevel:        string(tool.DefaultRiskLevel),
			ApprovalRequired: tool.DefaultRiskLevel == db.RiskLevelHIGH || tool.DefaultRiskLevel == db.RiskLevelCRITICAL,
		})
	}
	httpserver.WriteData(w, http.StatusOK, items)
}

// getTool 返回工具详情和 7 日调用统计。
func (h *adminHandler) getTool(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	if !authz.IsAdmin(user) && !authz.IsAuditor(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有访问工具详情的权限"))
		return
	}
	toolName := strings.TrimSpace(chi.URLParam(r, "tool_name"))
	tool, err := h.queries.GetTool(r.Context(), toolName)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	stats, _ := h.queries.ToolCallStatsByTool(r.Context(), db.ToolCallStatsByToolParams{
		ToolName: toolName,
		Since:    pgTimestamp(time.Now().UTC().AddDate(0, 0, -7)),
	})
	item := toolFromDB(tool, true)
	errorRate := 0.0
	if stats.CallsTotal > 0 {
		errorRate = float64(stats.ErrorTotal) / float64(stats.CallsTotal)
	}
	item.Stats = map[string]any{
		"calls_7d":           stats.CallsTotal,
		"error_rate_7d":      errorRate,
		"avg_duration_ms_7d": stats.AvgDurationMs,
	}
	item.Calls7D = stats.CallsTotal
	item.ErrorRate7D = errorRate
	item.AvgDurationMs7D = stats.AvgDurationMs
	httpserver.WriteData(w, http.StatusOK, item)
}

// updateTool 修改工具元数据并写入审计日志。
func (h *adminHandler) updateTool(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminWrite(w, r)
	if !ok {
		return
	}
	toolName := strings.TrimSpace(chi.URLParam(r, "tool_name"))
	existing, err := h.queries.GetTool(r.Context(), toolName)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	var req map[string]json.RawMessage
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	updated := existing
	readJSONBool(req, "enabled", &updated.Enabled)
	readJSONString(req, "description", &updated.Description)
	readJSONString(req, "category", &updated.Category)
	readJSONInt32(req, "timeout_seconds", &updated.TimeoutSeconds)
	if raw := req["default_risk_level"]; len(raw) > 0 {
		var risk string
		_ = json.Unmarshal(raw, &risk)
		updated.DefaultRiskLevel = db.RiskLevel(strings.ToUpper(strings.TrimSpace(risk)))
	}
	if raw := req["params_schema"]; len(raw) > 0 {
		updated.ParamsSchema = normalizeRawObject(raw)
	}
	if raw := req["retry_policy"]; len(raw) > 0 {
		updated.RetryPolicy = normalizeRawObject(raw)
	}
	row, err := h.queries.UpdateTool(r.Context(), db.UpdateToolParams{
		Enabled:          updated.Enabled,
		DefaultRiskLevel: updated.DefaultRiskLevel,
		TimeoutSeconds:   updated.TimeoutSeconds,
		Description:      updated.Description,
		Category:         updated.Category,
		ParamsSchema:     updated.ParamsSchema,
		RetryPolicy:      updated.RetryPolicy,
		ToolName:         toolName,
	})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	h.logAudit(r.Context(), user, r, "tool.update", "tool", toolName, toolFromDB(existing, true), toolFromDB(row, true))
	httpserver.WriteData(w, http.StatusOK, toolFromDB(row, true))
}

// listToolPolicies 返回工具策略分页列表。
func (h *adminHandler) listToolPolicies(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireToolPolicyRead(w, r)
	if !ok {
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	tenantID, _ := authz.EffectiveTenantID(user, r.URL.Query().Get("tenant_id"))
	if tenantID == "" && authz.IsPlatformAdmin(user) {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	conditions := []string{}
	args := []any{}
	if strings.TrimSpace(tenantID) != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id = %s", addArg(&args, tenantID)))
	}
	addOptionalTextFilter(&conditions, &args, "tool_name", r.URL.Query().Get("tool_name"))
	addOptionalTextFilter(&conditions, &args, "role", r.URL.Query().Get("role"))
	if enabledRaw := strings.TrimSpace(r.URL.Query().Get("enabled")); enabledRaw != "" {
		if enabledRaw == "true" {
			conditions = append(conditions, "effect <> 'DENY'")
		} else {
			conditions = append(conditions, "effect = 'DENY'")
		}
	}
	limitArg := addArg(&args, int32(pageSize+1))
	offsetArg := addArg(&args, int32((page-1)*pageSize))
	sql := fmt.Sprintf(`
select policy_id, tenant_id, tool_name, effect, max_risk_level, require_approval, config,
       created_at, updated_at, role, max_calls_per_day, argument_rules, timeout_seconds
from tenant_tool_policies
%s
order by tool_name asc, role asc
limit %s offset %s`, whereSQL(conditions), limitArg, offsetArg)
	items, err := h.scanToolPolicies(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// createToolPolicy 新建或覆盖工具策略。
func (h *adminHandler) createToolPolicy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		TenantID          string          `json:"tenant_id"`
		ToolName          string          `json:"tool_name"`
		Role              string          `json:"role"`
		Enabled           *bool           `json:"enabled"`
		ApprovalRequired  bool            `json:"approval_required"`
		MaxCallsPerDay    *int32          `json:"max_calls_per_day"`
		RiskLevelOverride string          `json:"risk_level_override"`
		ArgumentRules     json.RawMessage `json:"argument_rules"`
		TimeoutSeconds    *int32          `json:"timeout_seconds"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	tenantID, err := h.writeTenantID(user, req.TenantID)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	policyID, err := ids.New("policy")
	if err != nil {
		httpserver.WriteError(w, r, apperrors.Wrap(apperrors.CodeInternal, "生成 policy_id 失败", err))
		return
	}
	effect := policyEffectFromRequest(req.Enabled, req.ApprovalRequired)
	risk := db.RiskLevel(strings.ToUpper(defaultString(req.RiskLevelOverride, "HIGH")))
	row, err := h.queries.CreateToolPolicy(r.Context(), db.CreateToolPolicyParams{
		PolicyID:        policyID,
		TenantID:        tenantID,
		ToolName:        strings.TrimSpace(req.ToolName),
		Role:            authz.NormalizeRole(req.Role),
		Effect:          effect,
		MaxRiskLevel:    risk,
		RequireApproval: req.ApprovalRequired,
		MaxCallsPerDay:  req.MaxCallsPerDay,
		ArgumentRules:   normalizeRawObject(req.ArgumentRules),
		TimeoutSeconds:  req.TimeoutSeconds,
	})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	item := toolPolicyFromDB(row)
	h.logAudit(r.Context(), user, r, "tool_policy.create", "tool_policy", row.PolicyID, nil, item)
	httpserver.WriteData(w, http.StatusCreated, item)
}

// getToolPolicy 返回单条工具策略。
func (h *adminHandler) getToolPolicy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireToolPolicyRead(w, r)
	if !ok {
		return
	}
	policyID := strings.TrimSpace(chi.URLParam(r, "policy_id"))
	item, err := h.findToolPolicy(r.Context(), user, policyID)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	httpserver.WriteData(w, http.StatusOK, item)
}

// updateToolPolicy 覆盖策略可编辑字段。
func (h *adminHandler) updateToolPolicy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminWrite(w, r)
	if !ok {
		return
	}
	policyID := strings.TrimSpace(chi.URLParam(r, "policy_id"))
	before, err := h.findToolPolicy(r.Context(), user, policyID)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	var req struct {
		Enabled           *bool           `json:"enabled"`
		ApprovalRequired  bool            `json:"approval_required"`
		MaxCallsPerDay    *int32          `json:"max_calls_per_day"`
		RiskLevelOverride string          `json:"risk_level_override"`
		ArgumentRules     json.RawMessage `json:"argument_rules"`
		TimeoutSeconds    *int32          `json:"timeout_seconds"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	effect := policyEffectFromRequest(req.Enabled, req.ApprovalRequired)
	risk := db.RiskLevel(strings.ToUpper(defaultString(req.RiskLevelOverride, before.RiskLevelOverride)))
	row, err := h.queries.UpdateToolPolicyByID(r.Context(), db.UpdateToolPolicyByIDParams{
		Effect:          effect,
		MaxRiskLevel:    risk,
		RequireApproval: req.ApprovalRequired,
		MaxCallsPerDay:  req.MaxCallsPerDay,
		ArgumentRules:   normalizeRawObject(req.ArgumentRules),
		TimeoutSeconds:  req.TimeoutSeconds,
		TenantID:        before.TenantID,
		PolicyID:        policyID,
	})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	after := toolPolicyFromDB(row)
	h.logAudit(r.Context(), user, r, "tool_policy.update", "tool_policy", policyID, before, after)
	httpserver.WriteData(w, http.StatusOK, after)
}

// deleteToolPolicy 软禁用工具策略。
func (h *adminHandler) deleteToolPolicy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminWrite(w, r)
	if !ok {
		return
	}
	policyID := strings.TrimSpace(chi.URLParam(r, "policy_id"))
	before, err := h.findToolPolicy(r.Context(), user, policyID)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	row, err := h.queries.DisableToolPolicyByID(r.Context(), db.DisableToolPolicyByIDParams{
		TenantID: before.TenantID,
		PolicyID: policyID,
	})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	after := toolPolicyFromDB(row)
	h.logAudit(r.Context(), user, r, "tool_policy.delete", "tool_policy", policyID, before, after)
	httpserver.WriteData(w, http.StatusOK, after)
}

// simulateToolPolicy 执行策略模拟并返回命中结果。
func (h *adminHandler) simulateToolPolicy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireToolPolicyRead(w, r)
	if !ok {
		return
	}
	var req struct {
		TenantID  string          `json:"tenant_id"`
		UserID    string          `json:"user_id"`
		Role      string          `json:"role"`
		ToolName  string          `json:"tool_name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	tenantID, err := h.readTenantID(user, req.TenantID)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	role := authz.NormalizeRole(req.Role)
	rows, err := h.queries.ListToolPoliciesForToolRole(r.Context(), db.ListToolPoliciesForToolRoleParams{
		TenantID: tenantID,
		ToolName: strings.TrimSpace(req.ToolName),
		Role:     role,
	})
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	allowed := true
	risk := "LOW"
	approvalRequired := false
	var matched *string
	var denyReason *string
	if len(rows) > 0 {
		row := rows[0]
		risk = string(row.MaxRiskLevel)
		approvalRequired = row.RequireApproval || row.Effect == db.ToolPolicyEffectREQUIREAPPROVAL
		id := row.PolicyID
		matched = &id
		if row.Effect == db.ToolPolicyEffectDENY {
			allowed = false
			reason := "策略显式拒绝该工具"
			denyReason = &reason
		}
	}
	httpserver.WriteData(w, http.StatusOK, map[string]any{
		"allowed":           allowed,
		"risk_level":        risk,
		"approval_required": approvalRequired,
		"matched_policy_id": matched,
		"deny_reason":       denyReason,
	})
}

// listToolCalls 返回跨任务工具调用记录。
func (h *adminHandler) listToolCalls(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	query := r.URL.Query()
	conditions, args := h.toolCallScope(user, query.Get("tenant_id"), "tool_calls")
	addOptionalTextFilter(&conditions, &args, "tool_calls.task_id", query.Get("task_id"))
	addOptionalTextFilter(&conditions, &args, "tool_calls.call_id", query.Get("call_id"))
	addOptionalTextFilter(&conditions, &args, "tool_calls.tool_name", query.Get("tool_name"))
	addOptionalTextFilter(&conditions, &args, "tool_calls.status", strings.ToUpper(query.Get("status")))
	addOptionalTextFilter(&conditions, &args, "tool_calls.risk_level", strings.ToUpper(query.Get("risk_level")))
	addOptionalTextFilter(&conditions, &args, "tool_calls.approval_status", strings.ToUpper(query.Get("approval_status")))
	addOptionalTextFilter(&conditions, &args, "tool_calls.error_code", query.Get("error_code"))
	addOptionalTextFilter(&conditions, &args, "tool_calls.idempotency_key", query.Get("idempotency_key"))
	if userID := strings.TrimSpace(query.Get("user_id")); userID != "" && !authz.ScopeToSelf(user) {
		conditions = append(conditions, fmt.Sprintf("tool_calls.user_id = %s", addArg(&args, userID)))
	}
	limitArg := addArg(&args, int32(pageSize+1))
	offsetArg := addArg(&args, int32((page-1)*pageSize))
	sql := fmt.Sprintf(`
select call_id, task_id, tenant_id, user_id, tool_name, status, risk_level, approval_status,
       error_code, idempotency_key, created_at,
       case when started_at is not null and finished_at is not null then extract(epoch from (finished_at - started_at)) * 1000 else null end
from tool_calls
%s
order by created_at desc
limit %s offset %s`, whereSQL(conditions), limitArg, offsetArg)
	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	defer rows.Close()
	items := make([]toolCallCrossItem, 0, pageSize+1)
	for rows.Next() {
		var item toolCallCrossItem
		var created pgtype.Timestamptz
		var duration pgtype.Float8
		if err := rows.Scan(&item.CallID, &item.TaskID, &item.TenantID, &item.UserID, &item.ToolName, &item.Status, &item.RiskLevel, &item.ApprovalStatus, &item.ErrorCode, &item.IdempotencyKey, &created, &duration); err != nil {
			httpserver.WriteError(w, r, dbReadError(err))
			return
		}
		item.CreatedAt = pgTime(created)
		item.DurationMs = floatPtrFromPG(duration)
		items = append(items, item)
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// getToolCall 返回单条工具调用详情。
func (h *adminHandler) getToolCall(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	callID := strings.TrimSpace(chi.URLParam(r, "call_id"))
	conditions, args := h.toolCallScope(user, "", "tool_calls")
	conditions = append(conditions, fmt.Sprintf("tool_calls.call_id = %s", addArg(&args, callID)))
	sql := fmt.Sprintf(`
select tool_calls.call_id, tool_calls.task_id, tool_calls.tenant_id, tool_calls.user_id, tool_calls.tool_name,
       tool_calls.status, tool_calls.risk_level, tool_calls.approval_status, tool_calls.error_code,
       tool_calls.idempotency_key, tool_calls.created_at,
       case when started_at is not null and finished_at is not null then extract(epoch from (finished_at - started_at)) * 1000 else null end,
       tool_calls.arguments_hash, tool_calls.arguments, tool_calls.result, tool_calls.error_message, agent_tasks.trace_id
from tool_calls
left join agent_tasks on agent_tasks.tenant_id = tool_calls.tenant_id and agent_tasks.task_id = tool_calls.task_id
%s
limit 1`, whereSQL(conditions))
	var item toolCallCrossItem
	var created pgtype.Timestamptz
	var duration pgtype.Float8
	var argsJSON, resultJSON json.RawMessage
	if err := h.pool.QueryRow(r.Context(), sql, args...).Scan(
		&item.CallID, &item.TaskID, &item.TenantID, &item.UserID, &item.ToolName, &item.Status, &item.RiskLevel,
		&item.ApprovalStatus, &item.ErrorCode, &item.IdempotencyKey, &created, &duration,
		&item.ArgumentsHash, &argsJSON, &resultJSON, &item.ErrorMessage, &item.TraceID,
	); err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	item.CreatedAt = pgTime(created)
	item.DurationMs = floatPtrFromPG(duration)
	item.Arguments = json.RawMessage(jsonx.RedactJSON(argsJSON))
	item.Result = json.RawMessage(jsonx.RedactJSON(resultJSON))
	history, _ := h.scanApprovalItems(r.Context(), `
select approvals.approval_id, approvals.task_id, approvals.call_id, coalesce(tool_calls.tool_name, ''),
       approvals.risk_level, approvals.approval_reason, coalesce(tool_calls.user_id, ''),
       approvals.approver_id, approvals.status, approvals.comment,
       approvals.created_at, approvals.decided_at, approvals.expires_at
from approvals
left join tool_calls on tool_calls.tenant_id = approvals.tenant_id and tool_calls.call_id = approvals.call_id
where approvals.tenant_id = $1 and approvals.call_id = $2
order by approvals.created_at asc`, item.TenantID, item.CallID)
	item.Approvals = history
	httpserver.WriteData(w, http.StatusOK, item)
}

// listEvents 返回跨任务 timeline explorer 列表。
func (h *adminHandler) listEvents(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	query := r.URL.Query()
	conditions, args := h.eventScope(user, query.Get("tenant_id"))
	addOptionalTextFilter(&conditions, &args, "agent_events.task_id", query.Get("task_id"))
	addOptionalInt64Filter(&conditions, &args, "agent_events.event_id", query.Get("event_id"))
	addOptionalTextFilter(&conditions, &args, "agent_events.type", strings.ToUpper(query.Get("type")))
	addOptionalTextFilter(&conditions, &args, "agent_events.trace_id", query.Get("trace_id"))
	addOptionalTextFilter(&conditions, &args, "agent_tasks.user_id", query.Get("user_id"))
	addDateRangeFilters(&conditions, &args, "agent_events.created_at", query.Get("created_from"), query.Get("created_to"))
	limitArg := addArg(&args, int32(pageSize+1))
	offsetArg := addArg(&args, int32((page-1)*pageSize))
	sql := fmt.Sprintf(`
select agent_events.event_id, agent_events.task_id, agent_events.type, agent_events.input,
       agent_events.output, agent_events.metadata, agent_events.trace_id, agent_events.created_at
from agent_events
join agent_tasks on agent_tasks.tenant_id = agent_events.tenant_id and agent_tasks.task_id = agent_events.task_id
%s
order by agent_events.created_at desc, agent_events.event_id desc
limit %s offset %s`, whereSQL(conditions), limitArg, offsetArg)
	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	defer rows.Close()
	items := make([]eventExplorerItem, 0, pageSize+1)
	for rows.Next() {
		var item eventExplorerItem
		var input, output json.RawMessage
		var created pgtype.Timestamptz
		if err := rows.Scan(&item.EventID, &item.TaskID, &item.Type, &input, &output, &item.Metadata, &item.TraceID, &created); err != nil {
			httpserver.WriteError(w, r, dbReadError(err))
			return
		}
		item.InputSummary = truncateSummary(jsonx.RedactJSON(input), 500)
		item.OutputSummary = truncateSummary(jsonx.RedactJSON(output), 500)
		item.Metadata = jsonx.RedactJSON(item.Metadata)
		item.CreatedAt = pgTime(created)
		items = append(items, item)
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// getTrace 从 agent_events 聚合轻量 trace span。
func (h *adminHandler) getTrace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	traceID := strings.TrimSpace(chi.URLParam(r, "trace_id"))
	conditions, args := h.eventScope(user, "")
	conditions = append(conditions, fmt.Sprintf("agent_events.trace_id = %s", addArg(&args, traceID)))
	sql := fmt.Sprintf(`
select agent_events.task_id, agent_events.type, agent_events.created_at, coalesce(agent_events.output, '{}'::jsonb)
from agent_events
join agent_tasks on agent_tasks.tenant_id = agent_events.tenant_id and agent_tasks.task_id = agent_events.task_id
%s
order by agent_events.created_at asc, agent_events.event_id asc`, whereSQL(conditions))
	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	defer rows.Close()
	taskID := ""
	spans := make([]traceSpanItem, 0)
	var first, last time.Time
	for rows.Next() {
		var typ string
		var created pgtype.Timestamptz
		var output json.RawMessage
		if err := rows.Scan(&taskID, &typ, &created, &output); err != nil {
			httpserver.WriteError(w, r, dbReadError(err))
			return
		}
		start := pgTime(created)
		if first.IsZero() {
			first = start
		}
		last = start
		spans = append(spans, traceSpanItem{
			Name:       strings.ToLower(strings.ReplaceAll(typ, "_", "/")),
			Type:       traceSpanType(typ),
			Start:      start,
			DurationMs: durationFromEventOutput(output),
			Status:     traceSpanStatus(typ),
		})
	}
	if taskID == "" {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeNotFound, "trace 不存在"))
		return
	}
	total := int64(0)
	if !first.IsZero() && !last.IsZero() {
		total = last.Sub(first).Milliseconds()
	}
	response := map[string]any{
		"trace_id":          traceID,
		"task_id":           taskID,
		"total_duration_ms": total,
		"spans":             spans,
	}
	if externalURL := h.traceExternalURL(r.Context(), traceID); externalURL != "" {
		response["external_url"] = externalURL
	}
	httpserver.WriteData(w, http.StatusOK, response)
}

// getMetrics 返回平台指标快照。
func (h *adminHandler) getMetrics(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	from, _, _ := parseDashboardRange(r)
	taskConditions, taskArgs := h.taskScope(user, r.URL.Query().Get("tenant_id"), "agent_tasks")
	taskConditions = append(taskConditions, fmt.Sprintf("agent_tasks.created_at >= %s", addArg(&taskArgs, from)))
	taskSQL := fmt.Sprintf(`
select count(*)::bigint,
       count(*) filter (where status = 'SUCCEEDED')::bigint,
       count(*) filter (where status = 'FAILED')::bigint,
       count(*) filter (where status = 'CANCELED')::bigint,
       count(*) filter (where status = 'RUNNING')::bigint,
       count(*) filter (where status = 'WAITING_APPROVAL')::bigint
from agent_tasks
%s`, whereSQL(taskConditions))
	var created, completed, failed, cancelled, running, waiting int64
	if err := h.pool.QueryRow(r.Context(), taskSQL, taskArgs...).Scan(&created, &completed, &failed, &cancelled, &running, &waiting); err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	toolRisk, _ := h.toolRiskDistribution(r.Context(), user, r.URL.Query().Get("tenant_id"))
	cost := h.metricsCost(r.Context(), user, r.URL.Query().Get("tenant_id"), from)
	tools := h.metricsTools(r.Context(), user, r.URL.Query().Get("tenant_id"), from)
	httpserver.WriteData(w, http.StatusOK, map[string]any{
		"tasks": map[string]any{
			"created_total":    created,
			"completed_total":  completed,
			"failed_total":     failed,
			"cancelled_total":  cancelled,
			"running":          running,
			"waiting_approval": waiting,
			"duration_p50_s":   nil,
			"duration_p95_s":   nil,
			"duration_p99_s":   nil,
		},
		"llm": map[string]any{
			"calls_total":        0,
			"errors_total":       0,
			"latency_p95_ms":     nil,
			"tokens_total":       cost["tokens_total"],
			"cost_total_usd":     cost["cost_total_usd"],
			"model_distribution": []any{},
		},
		"tools": map[string]any{
			"calls_total":             tools["calls_total"],
			"errors_total":            tools["errors_total"],
			"latency_p95_ms":          nil,
			"risk_distribution":       toolRisk,
			"approval_required_total": tools["approval_required_total"],
			"approval_rejected_total": tools["approval_rejected_total"],
		},
		"workers": map[string]any{"active_workers": nil, "workflow_backlog": nil, "queue_lag": nil},
	})
}

// listLogs 将 AgentEvent 映射为轻量日志行。
func (h *adminHandler) listLogs(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	query := r.URL.Query()
	conditions, args := h.eventScope(user, query.Get("tenant_id"))
	addOptionalTextFilter(&conditions, &args, "agent_events.task_id", query.Get("task_id"))
	addOptionalTextFilter(&conditions, &args, "agent_events.trace_id", query.Get("trace_id"))
	addOptionalTextFilter(&conditions, &args, "agent_events.type", strings.ToUpper(query.Get("event_type")))
	addDateRangeFilters(&conditions, &args, "agent_events.created_at", query.Get("created_from"), query.Get("created_to"))
	limitArg := addArg(&args, int32(pageSize+1))
	offsetArg := addArg(&args, int32((page-1)*pageSize))
	sql := fmt.Sprintf(`
select agent_events.created_at, agent_events.type, agent_events.task_id, agent_events.trace_id,
       agent_events.tenant_id, agent_tasks.last_error_code, agent_events.output
from agent_events
join agent_tasks on agent_tasks.tenant_id = agent_events.tenant_id and agent_tasks.task_id = agent_events.task_id
%s
order by agent_events.created_at desc, agent_events.event_id desc
limit %s offset %s`, whereSQL(conditions), limitArg, offsetArg)
	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	defer rows.Close()
	items := make([]logItem, 0, pageSize+1)
	for rows.Next() {
		var created pgtype.Timestamptz
		var typ string
		var item logItem
		var output json.RawMessage
		if err := rows.Scan(&created, &typ, &item.TaskID, &item.TraceID, &item.TenantID, &item.ErrorCode, &output); err != nil {
			httpserver.WriteError(w, r, dbReadError(err))
			return
		}
		item.Timestamp = pgTime(created)
		item.Level = logLevelForEvent(typ)
		item.Service = "api-service"
		item.Message = truncateSummary(jsonx.RedactJSON(output), 500)
		if item.Message == "" || item.Message == "null" {
			item.Message = typ
		}
		if level := strings.TrimSpace(query.Get("level")); level != "" && item.Level != level {
			continue
		}
		items = append(items, item)
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// listArtifacts 返回跨任务产物列表。
func (h *adminHandler) listArtifacts(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	query := r.URL.Query()
	conditions, args := h.artifactScope(user, query.Get("tenant_id"))
	addOptionalTextFilter(&conditions, &args, "artifacts.task_id", query.Get("task_id"))
	addOptionalTextFilter(&conditions, &args, "artifacts.type", strings.ToUpper(query.Get("type")))
	addOptionalILikeFilter(&conditions, &args, "artifacts.name", query.Get("name"))
	addDateRangeFilters(&conditions, &args, "artifacts.created_at", query.Get("created_from"), query.Get("created_to"))
	conditions = append(conditions, "artifacts.status = 'AVAILABLE'")
	limitArg := addArg(&args, int32(pageSize+1))
	offsetArg := addArg(&args, int32((page-1)*pageSize))
	sql := fmt.Sprintf(`
select artifacts.artifact_id, artifacts.task_id, artifacts.name, artifacts.type,
       artifacts.size_bytes, artifacts.metadata, artifacts.content_json, artifacts.created_at
from artifacts
join agent_tasks on agent_tasks.tenant_id = artifacts.tenant_id and agent_tasks.task_id = artifacts.task_id
%s
order by artifacts.created_at desc
limit %s offset %s`, whereSQL(conditions), limitArg, offsetArg)
	items, err := h.scanArtifacts(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// getArtifact 返回产物详情,小于 1MB 的内联内容直接返回。
func (h *adminHandler) getArtifact(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	artifactID := strings.TrimSpace(chi.URLParam(r, "artifact_id"))
	conditions, args := h.artifactScope(user, "")
	conditions = append(conditions, fmt.Sprintf("artifacts.artifact_id = %s", addArg(&args, artifactID)))
	sql := fmt.Sprintf(`
select artifacts.artifact_id, artifacts.task_id, artifacts.name, artifacts.type,
       artifacts.size_bytes, artifacts.metadata, artifacts.content_json, artifacts.created_at
from artifacts
join agent_tasks on agent_tasks.tenant_id = artifacts.tenant_id and agent_tasks.task_id = artifacts.task_id
%s
limit 1`, whereSQL(conditions))
	items, err := h.scanArtifacts(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	if len(items) == 0 {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeNotFound, "artifact 不存在"))
		return
	}
	item := items[0]
	url := "/api/v1/artifacts/" + item.ArtifactID + "/download"
	item.DownloadURL = &url
	if item.SizeBytes > inlineArtifactLimit {
		item.Content = nil
	}
	httpserver.WriteData(w, http.StatusOK, item)
}

// downloadArtifact 输出产物内联内容。
func (h *adminHandler) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	artifactID := strings.TrimSpace(chi.URLParam(r, "artifact_id"))
	conditions, args := h.artifactScope(user, "")
	conditions = append(conditions, fmt.Sprintf("artifacts.artifact_id = %s", addArg(&args, artifactID)))
	sql := fmt.Sprintf(`
select artifacts.name, artifacts.media_type, artifacts.content_json
from artifacts
join agent_tasks on agent_tasks.tenant_id = artifacts.tenant_id and agent_tasks.task_id = artifacts.task_id
%s
limit 1`, whereSQL(conditions))
	var name string
	var mediaType *string
	var content json.RawMessage
	if err := h.pool.QueryRow(r.Context(), sql, args...).Scan(&name, &mediaType, &content); err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	if len(content) == 0 {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeNotFound, "artifact 内容不可内联下载"))
		return
	}
	if mediaType != nil && strings.TrimSpace(*mediaType) != "" {
		w.Header().Set("Content-Type", *mediaType)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(name)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// deleteArtifact 软删除产物并写审计日志。
func (h *adminHandler) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminWrite(w, r)
	if !ok {
		return
	}
	artifactID := strings.TrimSpace(chi.URLParam(r, "artifact_id"))
	before, err := h.findArtifactForAdmin(r.Context(), user, artifactID)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	row, err := h.queries.MarkArtifactDeleted(r.Context(), db.MarkArtifactDeletedParams{
		TenantID:   before["tenant_id"].(string),
		ArtifactID: artifactID,
	})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	h.logAudit(r.Context(), user, r, "artifact.delete", "artifact", artifactID, before, row)
	w.WriteHeader(http.StatusNoContent)
}

// listTenants 返回租户列表,平台管理员可看全部,租户管理员只看本租户。
func (h *adminHandler) listTenants(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	if !authz.IsAdmin(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有访问租户管理的权限"))
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	var tenants []db.Tenant
	if authz.IsPlatformAdmin(user) {
		tenants, err = h.queries.ListTenants(r.Context(), db.ListTenantsParams{OffsetRows: int32((page - 1) * pageSize), LimitRows: int32(pageSize + 1)})
	} else {
		var tenant db.Tenant
		tenant, err = h.queries.GetTenant(r.Context(), user.TenantID)
		tenants = []db.Tenant{tenant}
	}
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	hasMore := len(tenants) > pageSize
	if hasMore {
		tenants = tenants[:pageSize]
	}
	items := make([]tenantItem, 0, len(tenants))
	for _, tenant := range tenants {
		items = append(items, h.tenantFromDB(r.Context(), tenant))
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// createTenant 创建租户,仅 platform_admin 可用。
func (h *adminHandler) createTenant(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requirePlatformAdmin(w, r)
	if !ok {
		return
	}
	var req struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	tenantID, err := ids.New("tenant")
	if err != nil {
		httpserver.WriteError(w, r, apperrors.Wrap(apperrors.CodeInternal, "生成 tenant_id 失败", err))
		return
	}
	row, err := h.queries.CreateTenantFull(r.Context(), db.CreateTenantFullParams{
		TenantID: tenantID,
		Name:     strings.TrimSpace(req.Name),
		Status:   db.TenantStatusACTIVE,
		Metadata: json.RawMessage(`{}`),
		Config:   normalizeRawObject(req.Config),
	})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	item := h.tenantFromDB(r.Context(), row)
	h.logAudit(r.Context(), user, r, "tenant.create", "tenant", row.TenantID, nil, item)
	httpserver.WriteData(w, http.StatusCreated, item)
}

// getTenant 返回租户详情。
func (h *adminHandler) getTenant(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	tenantID := strings.TrimSpace(chi.URLParam(r, "tenant_id"))
	if !authz.IsPlatformAdmin(user) && tenantID != user.TenantID {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有访问该租户的权限"))
		return
	}
	row, err := h.queries.GetTenant(r.Context(), tenantID)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	httpserver.WriteData(w, http.StatusOK, h.tenantFromDB(r.Context(), row))
}

// updateTenant 修改租户配置,仅 platform_admin 可用。
func (h *adminHandler) updateTenant(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requirePlatformAdmin(w, r)
	if !ok {
		return
	}
	tenantID := strings.TrimSpace(chi.URLParam(r, "tenant_id"))
	before, err := h.queries.GetTenant(r.Context(), tenantID)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	var req struct {
		Name   *string         `json:"name"`
		Status *string         `json:"status"`
		Config json.RawMessage `json:"config"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	name := before.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	status := before.Status
	if req.Status != nil {
		status = tenantStatusFromAPI(*req.Status)
	}
	configValue := before.Config
	if len(req.Config) > 0 {
		configValue = normalizeRawObject(req.Config)
	}
	after, err := h.queries.UpdateTenant(r.Context(), db.UpdateTenantParams{Name: name, Status: status, Config: configValue, TenantID: tenantID})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	h.logAudit(r.Context(), user, r, "tenant.update", "tenant", tenantID, h.tenantFromDB(r.Context(), before), h.tenantFromDB(r.Context(), after))
	httpserver.WriteData(w, http.StatusOK, h.tenantFromDB(r.Context(), after))
}

// listUsers 返回用户列表。
func (h *adminHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	if !authz.IsAdmin(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有访问用户管理的权限"))
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	var rows []db.User
	if authz.IsPlatformAdmin(user) {
		rows, err = h.queries.ListAllUsers(r.Context(), db.ListAllUsersParams{OffsetRows: int32((page - 1) * pageSize), LimitRows: int32(pageSize + 1)})
	} else {
		rows, err = h.queries.ListUsersByTenant(r.Context(), db.ListUsersByTenantParams{TenantID: user.TenantID, OffsetRows: int32((page - 1) * pageSize), LimitRows: int32(pageSize + 1)})
	}
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	hasMore := len(rows) > pageSize
	if hasMore {
		rows = rows[:pageSize]
	}
	items := make([]userItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, userFromDB(row))
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// inviteUser 创建用户邀请记录。
func (h *adminHandler) inviteUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		TenantID string `json:"tenant_id"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	tenantID, err := h.writeTenantID(user, req.TenantID)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	role := authz.NormalizeRole(req.Role)
	if !authz.IsPlatformAdmin(user) && role == authz.RolePlatformAdmin {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "租户管理员不能创建平台管理员"))
		return
	}
	userID, err := ids.New("user")
	if err != nil {
		httpserver.WriteError(w, r, apperrors.Wrap(apperrors.CodeInternal, "生成 user_id 失败", err))
		return
	}
	name := strings.TrimSpace(req.Name)
	row, err := h.queries.CreateInvitedUser(r.Context(), db.CreateInvitedUserParams{TenantID: tenantID, UserID: userID, Email: strings.TrimSpace(req.Email), DisplayName: stringPtrOrNil(name), Role: role})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	item := userFromDB(row)
	h.logAudit(r.Context(), user, r, "user.invite", "user", row.UserID, nil, item)
	httpserver.WriteData(w, http.StatusCreated, item)
}

// updateUser 修改用户角色、状态或名称。
func (h *adminHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminWrite(w, r)
	if !ok {
		return
	}
	targetUserID := strings.TrimSpace(chi.URLParam(r, "user_id"))
	tenantID := user.TenantID
	if authz.IsPlatformAdmin(user) && strings.TrimSpace(r.URL.Query().Get("tenant_id")) != "" {
		tenantID = strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	}
	before, err := h.queries.GetUser(r.Context(), db.GetUserParams{TenantID: tenantID, UserID: targetUserID})
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	var req struct {
		Role   *string `json:"role"`
		Status *string `json:"status"`
		Name   *string `json:"name"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	role := before.Role
	if req.Role != nil {
		role = authz.NormalizeRole(*req.Role)
	}
	if !authz.IsPlatformAdmin(user) && role == authz.RolePlatformAdmin {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "租户管理员不能设置平台管理员角色"))
		return
	}
	if targetUserID == user.UserID && role != before.Role {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "不能修改自己的角色"))
		return
	}
	status := before.Status
	if req.Status != nil {
		status = userStatusFromAPI(*req.Status)
	}
	name := before.DisplayName
	if req.Name != nil {
		name = stringPtrOrNil(strings.TrimSpace(*req.Name))
	}
	after, err := h.queries.UpdateUser(r.Context(), db.UpdateUserParams{Role: role, Status: status, DisplayName: name, TenantID: tenantID, UserID: targetUserID})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	h.logAudit(r.Context(), user, r, "user.update", "user", targetUserID, userFromDB(before), userFromDB(after))
	httpserver.WriteData(w, http.StatusOK, userFromDB(after))
}

// getSettings 返回全部系统设置,未设置的 section 用代码默认值补齐。
func (h *adminHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	if !authz.IsAdmin(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有访问系统设置的权限"))
		return
	}
	settings := defaultSettings()
	rows, err := h.queries.ListSystemSettings(r.Context())
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	for _, row := range rows {
		settings[row.Section] = json.RawMessage(jsonx.RedactJSON(row.Value))
	}
	httpserver.WriteData(w, http.StatusOK, settings)
}

// updateSettings 覆盖一个设置分组,仅 platform_admin 可用。
func (h *adminHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requirePlatformAdmin(w, r)
	if !ok {
		return
	}
	section := strings.TrimSpace(chi.URLParam(r, "section"))
	if _, exists := defaultSettings()[section]; !exists {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeInvalidArg, "未知 settings section"))
		return
	}
	var value json.RawMessage
	if err := decodeJSONBody(w, r, &value); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	updated, err := h.queries.UpsertSystemSetting(r.Context(), db.UpsertSystemSettingParams{Section: section, Value: normalizeRawObject(value), UpdatedBy: &user.UserID})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	h.logAudit(r.Context(), user, r, "settings.update", "settings", section, nil, updated.Value)
	httpserver.WriteData(w, http.StatusOK, json.RawMessage(jsonx.RedactJSON(updated.Value)))
}

// listAuditLogs 返回审计日志分页列表。
func (h *adminHandler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	if !authz.IsAdmin(user) && !authz.IsAuditor(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有访问审计日志的权限"))
		return
	}
	page, pageSize, err := parsePage(r)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	query := r.URL.Query()
	conditions := []string{}
	args := []any{}
	tenantID, _ := authz.EffectiveTenantID(user, query.Get("tenant_id"))
	if tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id = %s", addArg(&args, tenantID)))
	}
	addOptionalTextFilter(&conditions, &args, "actor_id", query.Get("actor_id"))
	addOptionalTextFilter(&conditions, &args, "action", query.Get("action"))
	addOptionalTextFilter(&conditions, &args, "resource_type", query.Get("resource_type"))
	addOptionalTextFilter(&conditions, &args, "resource_id", query.Get("resource_id"))
	addDateRangeFilters(&conditions, &args, "created_at", query.Get("created_from"), query.Get("created_to"))
	limitArg := addArg(&args, int32(pageSize+1))
	offsetArg := addArg(&args, int32((page-1)*pageSize))
	sql := fmt.Sprintf(`
select audit_id, actor_id, tenant_id, action, resource_type, resource_id, before, after, ip, user_agent, created_at
from audit_logs
%s
order by created_at desc
limit %s offset %s`, whereSQL(conditions), limitArg, offsetArg)
	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	defer rows.Close()
	items := make([]auditLogItem, 0, pageSize+1)
	for rows.Next() {
		var item auditLogItem
		var created pgtype.Timestamptz
		if err := rows.Scan(&item.AuditID, &item.ActorID, &item.TenantID, &item.Action, &item.ResourceType, &item.ResourceID, &item.Before, &item.After, &item.IP, &item.UserAgent, &created); err != nil {
			httpserver.WriteError(w, r, dbReadError(err))
			return
		}
		item.Before = jsonx.RedactJSON(item.Before)
		item.After = jsonx.RedactJSON(item.After)
		item.CreatedAt = pgTime(created)
		items = append(items, item)
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	httpserver.WriteData(w, http.StatusOK, newPage(items, page, pageSize, hasMore))
}

// listTaskTemplates 返回本人和租户共享任务模板。
func (h *adminHandler) listTaskTemplates(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	rows, err := h.queries.ListTaskTemplatesForUser(r.Context(), db.ListTaskTemplatesForUserParams{TenantID: user.TenantID, CreatedBy: user.UserID, LimitRows: 100, OffsetRows: 0})
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	items := make([]taskTemplateItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, taskTemplateFromDB(row))
	}
	httpserver.WriteData(w, http.StatusOK, map[string]any{"items": items})
}

// createTaskTemplate 保存任务创建表单为模板。
func (h *adminHandler) createTaskTemplate(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	if authz.IsAuditor(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "审计员不能创建任务模板"))
		return
	}
	var req struct {
		Name    string          `json:"name"`
		Payload json.RawMessage `json:"payload"`
		Shared  bool            `json:"shared"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	templateID, err := ids.New("tmpl")
	if err != nil {
		httpserver.WriteError(w, r, apperrors.Wrap(apperrors.CodeInternal, "生成 template_id 失败", err))
		return
	}
	row, err := h.queries.CreateTaskTemplate(r.Context(), db.CreateTaskTemplateParams{
		TemplateID: templateID,
		TenantID:   user.TenantID,
		Name:       strings.TrimSpace(req.Name),
		Payload:    normalizeRawObject(req.Payload),
		Shared:     req.Shared,
		CreatedBy:  user.UserID,
	})
	if err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	httpserver.WriteData(w, http.StatusCreated, taskTemplateFromDB(row))
}

// deleteTaskTemplate 删除本人或管理员可管理的任务模板。
func (h *adminHandler) deleteTaskTemplate(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return
	}
	templateID := strings.TrimSpace(chi.URLParam(r, "template_id"))
	template, err := h.queries.GetTaskTemplate(r.Context(), db.GetTaskTemplateParams{TenantID: user.TenantID, TemplateID: templateID})
	if err != nil {
		httpserver.WriteError(w, r, dbReadError(err))
		return
	}
	if template.CreatedBy != user.UserID && !authz.IsAdmin(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有删除该模板的权限"))
		return
	}
	if _, err := h.queries.DeleteTaskTemplate(r.Context(), db.DeleteTaskTemplateParams{TenantID: user.TenantID, TemplateID: templateID}); err != nil {
		httpserver.WriteError(w, r, dbWriteError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// userFromRequest 读取认证用户并做角色归一化。
func (h *adminHandler) userFromRequest(w http.ResponseWriter, r *http.Request) (authn.User, bool) {
	user, _, ok := principalFromRequest(w, r)
	if !ok {
		return authn.User{}, false
	}
	user.Role = authz.NormalizeRole(user.Role)
	return user, true
}

// requireAdminWrite 校验管理员写权限。
func (h *adminHandler) requireAdminWrite(w http.ResponseWriter, r *http.Request) (authn.User, bool) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return authn.User{}, false
	}
	if !authz.IsAdmin(user) || authz.IsAuditor(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有管理写权限"))
		return authn.User{}, false
	}
	return user, true
}

// requirePlatformAdmin 校验平台管理员权限。
func (h *adminHandler) requirePlatformAdmin(w http.ResponseWriter, r *http.Request) (authn.User, bool) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return authn.User{}, false
	}
	if !authz.IsPlatformAdmin(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "该操作仅平台管理员可执行"))
		return authn.User{}, false
	}
	return user, true
}

// requireToolPolicyRead 校验工具策略读取权限。
func (h *adminHandler) requireToolPolicyRead(w http.ResponseWriter, r *http.Request) (authn.User, bool) {
	user, ok := h.userFromRequest(w, r)
	if !ok {
		return authn.User{}, false
	}
	if !authz.IsAdmin(user) && !authz.IsAuditor(user) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeForbidden, "没有访问工具策略的权限"))
		return authn.User{}, false
	}
	return user, true
}

// taskScope 生成任务表的租户和用户隔离条件。
func (h *adminHandler) taskScope(user authn.User, requestedTenantID string, alias string) ([]string, []any) {
	conditions := []string{}
	args := []any{}
	tenantID, _ := authz.EffectiveTenantID(user, requestedTenantID)
	if tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("%s.tenant_id = %s", alias, addArg(&args, tenantID)))
	}
	if authz.ScopeToSelf(user) {
		conditions = append(conditions, fmt.Sprintf("%s.user_id = %s", alias, addArg(&args, user.UserID)))
	}
	return conditions, args
}

// toolCallScope 生成工具调用表的租户和用户隔离条件。
func (h *adminHandler) toolCallScope(user authn.User, requestedTenantID string, alias string) ([]string, []any) {
	conditions := []string{}
	args := []any{}
	tenantID, _ := authz.EffectiveTenantID(user, requestedTenantID)
	if tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("%s.tenant_id = %s", alias, addArg(&args, tenantID)))
	}
	if authz.ScopeToSelf(user) {
		conditions = append(conditions, fmt.Sprintf("%s.user_id = %s", alias, addArg(&args, user.UserID)))
	}
	return conditions, args
}

// approvalScope 生成审批表的租户和用户隔离条件。
func (h *adminHandler) approvalScope(user authn.User, requestedTenantID string) ([]string, []any) {
	conditions := []string{}
	args := []any{}
	tenantID, _ := authz.EffectiveTenantID(user, requestedTenantID)
	if tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("approvals.tenant_id = %s", addArg(&args, tenantID)))
	}
	if authz.ScopeToSelf(user) {
		conditions = append(conditions, fmt.Sprintf("tool_calls.user_id = %s", addArg(&args, user.UserID)))
	}
	return conditions, args
}

// eventScope 生成事件表的租户和用户隔离条件。
func (h *adminHandler) eventScope(user authn.User, requestedTenantID string) ([]string, []any) {
	conditions := []string{}
	args := []any{}
	tenantID, _ := authz.EffectiveTenantID(user, requestedTenantID)
	if tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("agent_events.tenant_id = %s", addArg(&args, tenantID)))
	}
	if authz.ScopeToSelf(user) {
		conditions = append(conditions, fmt.Sprintf("agent_tasks.user_id = %s", addArg(&args, user.UserID)))
	}
	return conditions, args
}

// artifactScope 生成产物表的租户和用户隔离条件。
func (h *adminHandler) artifactScope(user authn.User, requestedTenantID string) ([]string, []any) {
	conditions := []string{}
	args := []any{}
	tenantID, _ := authz.EffectiveTenantID(user, requestedTenantID)
	if tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("artifacts.tenant_id = %s", addArg(&args, tenantID)))
	}
	if authz.ScopeToSelf(user) {
		conditions = append(conditions, fmt.Sprintf("agent_tasks.user_id = %s", addArg(&args, user.UserID)))
	}
	return conditions, args
}

// writeTenantID 校验写操作租户范围。
func (h *adminHandler) writeTenantID(user authn.User, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if authz.IsPlatformAdmin(user) {
		if requested == "" {
			return "", apperrors.New(apperrors.CodeInvalidArg, "tenant_id 不能为空")
		}
		return requested, nil
	}
	if requested != "" && requested != user.TenantID {
		return "", apperrors.New(apperrors.CodeForbidden, "不能跨租户写入")
	}
	return user.TenantID, nil
}

// readTenantID 校验读取或模拟操作租户范围。
func (h *adminHandler) readTenantID(user authn.User, requested string) (string, error) {
	tenantID, _ := authz.EffectiveTenantID(user, requested)
	if tenantID == "" {
		return "", apperrors.New(apperrors.CodeInvalidArg, "tenant_id 不能为空")
	}
	return tenantID, nil
}

// logAudit 写入审计日志,未配置审计服务时跳过。
func (h *adminHandler) logAudit(ctx context.Context, user authn.User, r *http.Request, action string, resourceType string, resourceID string, before any, after any) {
	if h.audit == nil {
		return
	}
	_ = h.audit.Log(ctx, auditlog.LogInput{
		ActorID:      user.UserID,
		TenantID:     user.TenantID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       before,
		After:        after,
		IP:           httpserver.ClientIP(r),
		UserAgent:    r.UserAgent(),
	})
}

// scanApprovalItems 扫描审批列表查询结果。
func (h *adminHandler) scanApprovalItems(ctx context.Context, sql string, args ...any) ([]approvalItem, error) {
	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []approvalItem{}
	for rows.Next() {
		var item approvalItem
		var created pgtype.Timestamptz
		var decided, expires pgtype.Timestamptz
		if err := rows.Scan(&item.ApprovalID, &item.TaskID, &item.CallID, &item.ToolName, &item.RiskLevel, &item.ApprovalReason, &item.RequesterID, &item.ApproverID, &item.Status, &item.Note, &created, &decided, &expires); err != nil {
			return nil, err
		}
		item.CreatedAt = pgTime(created)
		item.DecidedAt = timePtrFromPG(decided)
		item.ExpiresAt = timePtrFromPG(expires)
		items = append(items, item)
	}
	return items, rows.Err()
}

// scanToolPolicies 扫描工具策略查询结果。
func (h *adminHandler) scanToolPolicies(ctx context.Context, sql string, args ...any) ([]toolPolicyItem, error) {
	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []toolPolicyItem{}
	for rows.Next() {
		var row db.TenantToolPolicy
		if err := rows.Scan(&row.PolicyID, &row.TenantID, &row.ToolName, &row.Effect, &row.MaxRiskLevel, &row.RequireApproval, &row.Config, &row.CreatedAt, &row.UpdatedAt, &row.Role, &row.MaxCallsPerDay, &row.ArgumentRules, &row.TimeoutSeconds); err != nil {
			return nil, err
		}
		items = append(items, toolPolicyFromDB(row))
	}
	return items, rows.Err()
}

// scanArtifacts 扫描 artifact 列表或详情查询结果。
func (h *adminHandler) scanArtifacts(ctx context.Context, sql string, args ...any) ([]artifactItem, error) {
	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []artifactItem{}
	for rows.Next() {
		var item artifactItem
		var created pgtype.Timestamptz
		if err := rows.Scan(&item.ArtifactID, &item.TaskID, &item.Name, &item.Type, &item.SizeBytes, &item.Metadata, &item.Content, &created); err != nil {
			return nil, err
		}
		item.Metadata = jsonx.RedactJSON(item.Metadata)
		item.Content = jsonx.RedactJSON(item.Content)
		item.CreatedAt = pgTime(created)
		items = append(items, item)
	}
	return items, rows.Err()
}

// taskStatusDistribution 聚合任务状态分布。
func (h *adminHandler) taskStatusDistribution(ctx context.Context, user authn.User, requestedTenantID string) ([]map[string]any, error) {
	conditions, args := h.taskScope(user, requestedTenantID, "agent_tasks")
	sql := fmt.Sprintf(`select status, count(*)::bigint from agent_tasks %s group by status order by status`, whereSQL(conditions))
	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"status": status, "count": count})
	}
	return items, rows.Err()
}

// approvalRiskCounts 聚合 pending/rejected 审批数量。
func (h *adminHandler) approvalRiskCounts(ctx context.Context, user authn.User, requestedTenantID string) (int64, int64, error) {
	conditions, args := h.approvalScope(user, requestedTenantID)
	sql := fmt.Sprintf(`
select count(*) filter (where approvals.status = 'PENDING')::bigint,
       count(*) filter (where approvals.status = 'REJECTED')::bigint
from approvals
left join tool_calls on tool_calls.tenant_id = approvals.tenant_id and tool_calls.call_id = approvals.call_id
%s`, whereSQL(conditions))
	var pending, rejected int64
	err := h.pool.QueryRow(ctx, sql, args...).Scan(&pending, &rejected)
	return pending, rejected, err
}

// toolRiskDistribution 聚合工具调用风险等级分布。
func (h *adminHandler) toolRiskDistribution(ctx context.Context, user authn.User, requestedTenantID string) ([]map[string]any, error) {
	conditions, args := h.toolCallScope(user, requestedTenantID, "tool_calls")
	sql := fmt.Sprintf(`select risk_level, count(*)::bigint from tool_calls %s group by risk_level order by risk_level`, whereSQL(conditions))
	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var risk string
		var count int64
		if err := rows.Scan(&risk, &count); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"risk_level": risk, "count": count})
	}
	return items, rows.Err()
}

// highRiskTrend 返回 24 小时高风险调用趋势。
func (h *adminHandler) highRiskTrend(ctx context.Context, user authn.User, requestedTenantID string) ([]map[string]any, error) {
	conditions, args := h.toolCallScope(user, requestedTenantID, "tool_calls")
	conditions = append(conditions, "tool_calls.risk_level in ('HIGH','CRITICAL')")
	conditions = append(conditions, fmt.Sprintf("tool_calls.created_at >= %s", addArg(&args, time.Now().UTC().Add(-24*time.Hour))))
	sql := fmt.Sprintf(`
select date_trunc('hour', created_at), count(*)::bigint
from tool_calls
%s
group by 1
order by 1 asc`, whereSQL(conditions))
	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var bucket pgtype.Timestamptz
		var count int64
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"time": pgTime(bucket), "count": count})
	}
	return items, rows.Err()
}

// metricsCost 聚合指标页 LLM token 与成本。
func (h *adminHandler) metricsCost(ctx context.Context, user authn.User, requestedTenantID string, from time.Time) map[string]any {
	conditions, args := h.taskScope(user, requestedTenantID, "agent_tasks")
	conditions = append(conditions, fmt.Sprintf("agent_tasks.created_at >= %s", addArg(&args, from)))
	sql := fmt.Sprintf(`select coalesce(sum((budget_usage ->> 'used_tokens')::bigint),0)::bigint, coalesce(sum((budget_usage ->> 'used_cost_usd')::float8),0)::float8 from agent_tasks %s`, whereSQL(conditions))
	var tokens int64
	var cost float64
	_ = h.pool.QueryRow(ctx, sql, args...).Scan(&tokens, &cost)
	return map[string]any{"tokens_total": tokens, "cost_total_usd": cost}
}

// metricsTools 聚合指标页工具调用数量。
func (h *adminHandler) metricsTools(ctx context.Context, user authn.User, requestedTenantID string, from time.Time) map[string]any {
	conditions, args := h.toolCallScope(user, requestedTenantID, "tool_calls")
	conditions = append(conditions, fmt.Sprintf("tool_calls.created_at >= %s", addArg(&args, from)))
	sql := fmt.Sprintf(`
select count(*)::bigint,
       count(*) filter (where status = 'FAILED')::bigint,
       count(*) filter (where approval_status = 'PENDING')::bigint,
       count(*) filter (where approval_status = 'REJECTED')::bigint
from tool_calls
%s`, whereSQL(conditions))
	var total, errorsCount, approvals, rejected int64
	_ = h.pool.QueryRow(ctx, sql, args...).Scan(&total, &errorsCount, &approvals, &rejected)
	return map[string]any{"calls_total": total, "errors_total": errorsCount, "approval_required_total": approvals, "approval_rejected_total": rejected}
}

// traceExternalURL 读取 observability.trace_url_template 并替换 trace_id。
func (h *adminHandler) traceExternalURL(ctx context.Context, traceID string) string {
	row, err := h.queries.GetSystemSetting(ctx, "observability")
	if err != nil {
		return ""
	}
	var value map[string]any
	if err := json.Unmarshal(row.Value, &value); err != nil {
		return ""
	}
	template, _ := value["trace_url_template"].(string)
	if template == "" {
		return ""
	}
	return strings.ReplaceAll(template, "{trace_id}", traceID)
}

// tenantFromDB 转换租户行并补充今日用量。
func (h *adminHandler) tenantFromDB(ctx context.Context, row db.Tenant) tenantItem {
	usageRow, _ := h.queries.TenantUsageToday(ctx, db.TenantUsageTodayParams{Since: pgTimestamp(time.Now().UTC().Truncate(24 * time.Hour)), TenantID: row.TenantID})
	return tenantItem{
		TenantID:  row.TenantID,
		Name:      row.Name,
		Status:    tenantStatusToAPI(row.Status),
		Config:    jsonx.RedactJSON(row.Config),
		Usage:     tenantUsage{TasksToday: usageRow.TasksToday, TokensToday: usageRow.TokensToday, CostTodayUSD: usageRow.CostTodayUsd},
		CreatedAt: pgTime(row.CreatedAt),
	}
}

// findToolPolicy 按 policy_id 查找策略并做租户权限校验。
func (h *adminHandler) findToolPolicy(ctx context.Context, user authn.User, policyID string) (toolPolicyItem, error) {
	conditions := []string{fmt.Sprintf("policy_id = %s", "$1")}
	args := []any{policyID}
	if !authz.IsPlatformAdmin(user) {
		conditions = append(conditions, fmt.Sprintf("tenant_id = %s", addArg(&args, user.TenantID)))
	}
	sql := fmt.Sprintf(`
select policy_id, tenant_id, tool_name, effect, max_risk_level, require_approval, config,
       created_at, updated_at, role, max_calls_per_day, argument_rules, timeout_seconds
from tenant_tool_policies
%s
limit 1`, whereSQL(conditions))
	items, err := h.scanToolPolicies(ctx, sql, args...)
	if err != nil {
		return toolPolicyItem{}, err
	}
	if len(items) == 0 {
		return toolPolicyItem{}, pgx.ErrNoRows
	}
	return items[0], nil
}

// findArtifactForAdmin 查询产物归属并校验管理员跨租户写权限。
func (h *adminHandler) findArtifactForAdmin(ctx context.Context, user authn.User, artifactID string) (map[string]any, error) {
	conditions := []string{fmt.Sprintf("artifact_id = %s", "$1")}
	args := []any{artifactID}
	if !authz.IsPlatformAdmin(user) {
		conditions = append(conditions, fmt.Sprintf("tenant_id = %s", addArg(&args, user.TenantID)))
	}
	sql := fmt.Sprintf(`select artifact_id, tenant_id, task_id, name from artifacts %s limit 1`, whereSQL(conditions))
	var id, tenantID, taskID, name string
	if err := h.pool.QueryRow(ctx, sql, args...).Scan(&id, &tenantID, &taskID, &name); err != nil {
		return nil, dbReadError(err)
	}
	return map[string]any{"artifact_id": id, "tenant_id": tenantID, "task_id": taskID, "name": name}, nil
}

// parsePage 解析统一分页参数。
func parsePage(r *http.Request) (int, int, error) {
	page, err := parsePositiveIntQuery(r.URL.Query().Get("page"), 1, "page")
	if err != nil {
		return 0, 0, err
	}
	pageSize, err := parsePositiveIntQuery(r.URL.Query().Get("page_size"), defaultAdminPageSize, "page_size")
	if err != nil {
		return 0, 0, err
	}
	if pageSize > maxAdminPageSize {
		return 0, 0, apperrors.New(apperrors.CodeInvalidArg, fmt.Sprintf("page_size 必须在 1 到 %d 之间", maxAdminPageSize))
	}
	return page, pageSize, nil
}

// newPage 构造统一分页响应。
func newPage(items any, page int, pageSize int, hasMore bool) pageEnvelope {
	var next *int
	if hasMore {
		value := page + 1
		next = &value
	}
	return pageEnvelope{Items: items, Page: page, PageSize: pageSize, HasMore: hasMore, NextPage: next}
}

// addArg 追加 SQL 参数并返回对应占位符。
func addArg(args *[]any, value any) string {
	*args = append(*args, value)
	return fmt.Sprintf("$%d", len(*args))
}

// whereSQL 拼接 WHERE 子句。
func whereSQL(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	return "where " + strings.Join(conditions, " and ")
}

// addOptionalTextFilter 添加等值文本过滤条件。
func addOptionalTextFilter(conditions *[]string, args *[]any, field string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	*conditions = append(*conditions, fmt.Sprintf("%s = %s", field, addArg(args, value)))
}

// addOptionalILikeFilter 添加模糊文本过滤条件。
func addOptionalILikeFilter(conditions *[]string, args *[]any, field string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	*conditions = append(*conditions, fmt.Sprintf("%s ilike %s", field, addArg(args, "%"+value+"%")))
}

// addOptionalInt64Filter 添加 int64 等值过滤条件。
func addOptionalInt64Filter(conditions *[]string, args *[]any, field string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return
	}
	*conditions = append(*conditions, fmt.Sprintf("%s = %s", field, addArg(args, parsed)))
}

// addDateRangeFilters 添加 created_from/created_to 时间范围过滤。
func addDateRangeFilters(conditions *[]string, args *[]any, field string, fromRaw string, toRaw string) {
	if from, err := time.Parse(time.RFC3339, strings.TrimSpace(fromRaw)); err == nil {
		*conditions = append(*conditions, fmt.Sprintf("%s >= %s", field, addArg(args, from.UTC())))
	}
	if to, err := time.Parse(time.RFC3339, strings.TrimSpace(toRaw)); err == nil {
		*conditions = append(*conditions, fmt.Sprintf("%s <= %s", field, addArg(args, to.UTC())))
	}
}

// parseDashboardRange 解析 dashboard/metrics 时间范围。
func parseDashboardRange(r *http.Request) (time.Time, time.Time, string) {
	now := time.Now().UTC()
	if from, err := time.Parse(time.RFC3339, strings.TrimSpace(r.URL.Query().Get("from"))); err == nil {
		to := now
		if parsedTo, err := time.Parse(time.RFC3339, strings.TrimSpace(r.URL.Query().Get("to"))); err == nil {
			to = parsedTo.UTC()
		}
		return from.UTC(), to, "hour"
	}
	switch strings.TrimSpace(r.URL.Query().Get("range")) {
	case "1h":
		return now.Add(-1 * time.Hour), now, "minute"
	case "7d":
		return now.AddDate(0, 0, -7), now, "day"
	default:
		return now.Add(-24 * time.Hour), now, "hour"
	}
}

// pgTimestamp 将 time.Time 转换为 pgtype.Timestamptz。
func pgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

// pgTime 将 pgtype.Timestamptz 转换为 UTC time.Time。
func pgTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

// timePtrFromPG 将 pg 时间转换为可空时间指针。
func timePtrFromPG(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

// floatPtrFromPG 将 pg nullable float8 转换为 *float64。
func floatPtrFromPG(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	v := value.Float64
	return &v
}

// dbReadError 将数据库读取错误转换为应用错误。
func dbReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.New(apperrors.CodeNotFound, "资源不存在")
	}
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库读取失败", err)
}

// dbWriteError 将数据库写入错误转换为应用错误。
func dbWriteError(err error) error {
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库写入失败", err)
}

// toolFromDB 将数据库工具行转换为 API 响应。
func toolFromDB(tool db.Tool, includeSchemas bool) toolItem {
	item := toolItem{
		ToolName:               tool.ToolName,
		Description:            tool.Description,
		Category:               tool.Category,
		DefaultRiskLevel:       string(tool.DefaultRiskLevel),
		Enabled:                tool.Enabled,
		Version:                tool.Version,
		Owner:                  tool.Owner,
		HasSideEffect:          tool.HasSideEffect,
		RequiresIdempotencyKey: tool.RequiresIdempotencyKey,
		TimeoutSeconds:         tool.TimeoutSeconds,
		CreatedAt:              pgTime(tool.CreatedAt),
		UpdatedAt:              pgTime(tool.UpdatedAt),
	}
	if includeSchemas {
		item.ParamsSchema = jsonx.RedactJSON(tool.ParamsSchema)
		item.ResultSchema = jsonx.RedactJSON(tool.ResultSchema)
		item.RetryPolicy = jsonx.RedactJSON(tool.RetryPolicy)
	}
	return item
}

// toolPolicyFromDB 将数据库工具策略行转换为 API 响应。
func toolPolicyFromDB(row db.TenantToolPolicy) toolPolicyItem {
	return toolPolicyItem{
		PolicyID:          row.PolicyID,
		TenantID:          row.TenantID,
		ToolName:          row.ToolName,
		Role:              row.Role,
		Enabled:           row.Effect != db.ToolPolicyEffectDENY,
		ApprovalRequired:  row.RequireApproval || row.Effect == db.ToolPolicyEffectREQUIREAPPROVAL,
		MaxCallsPerDay:    row.MaxCallsPerDay,
		RiskLevelOverride: string(row.MaxRiskLevel),
		ArgumentRules:     jsonx.RedactJSON(row.ArgumentRules),
		TimeoutSeconds:    row.TimeoutSeconds,
		CreatedAt:         pgTime(row.CreatedAt),
		UpdatedAt:         pgTime(row.UpdatedAt),
	}
}

// userFromDB 将数据库用户行转换为 API 响应。
func userFromDB(row db.User) userItem {
	return userItem{
		UserID:      row.UserID,
		Email:       row.Email,
		Name:        row.DisplayName,
		TenantID:    row.TenantID,
		Role:        authz.NormalizeRole(row.Role),
		Status:      userStatusToAPI(row.Status),
		LastLoginAt: timePtrFromPG(row.LastLoginAt),
		CreatedAt:   pgTime(row.CreatedAt),
	}
}

// taskTemplateFromDB 将数据库任务模板转换为前端契约响应。
func taskTemplateFromDB(row db.TaskTemplate) taskTemplateItem {
	return taskTemplateItem{
		TemplateID: row.TemplateID,
		Name:       row.Name,
		Payload:    jsonx.RedactJSON(row.Payload),
		Shared:     row.Shared,
		CreatedBy:  row.CreatedBy,
		CreatedAt:  pgTime(row.CreatedAt),
	}
}

// tenantStatusToAPI 将数据库租户状态映射为契约状态。
func tenantStatusToAPI(status db.TenantStatus) string {
	switch status {
	case db.TenantStatusACTIVE:
		return "active"
	default:
		return "disabled"
	}
}

// tenantStatusFromAPI 将契约租户状态映射为数据库枚举。
func tenantStatusFromAPI(status string) db.TenantStatus {
	if strings.ToLower(strings.TrimSpace(status)) == "active" {
		return db.TenantStatusACTIVE
	}
	return db.TenantStatusSUSPENDED
}

// userStatusToAPI 将数据库用户状态映射为契约状态。
func userStatusToAPI(status db.UserStatus) string {
	if status == db.UserStatusACTIVE {
		return "active"
	}
	return "disabled"
}

// userStatusFromAPI 将契约用户状态映射为数据库枚举。
func userStatusFromAPI(status string) db.UserStatus {
	if strings.ToLower(strings.TrimSpace(status)) == "active" {
		return db.UserStatusACTIVE
	}
	return db.UserStatusDISABLED
}

// policyEffectFromRequest 根据 enabled/approval_required 推导数据库策略效果。
func policyEffectFromRequest(enabled *bool, approvalRequired bool) db.ToolPolicyEffect {
	if enabled != nil && !*enabled {
		return db.ToolPolicyEffectDENY
	}
	if approvalRequired {
		return db.ToolPolicyEffectREQUIREAPPROVAL
	}
	return db.ToolPolicyEffectALLOW
}

// normalizeRawObject 将空 JSON 归一为对象。
func normalizeRawObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return json.RawMessage(`{}`)
	}
	return raw
}

// readJSONBool 从 RawMessage map 中读取 bool 字段。
func readJSONBool(values map[string]json.RawMessage, key string, target *bool) {
	if raw := values[key]; len(raw) > 0 {
		_ = json.Unmarshal(raw, target)
	}
}

// readJSONString 从 RawMessage map 中读取 string 字段。
func readJSONString(values map[string]json.RawMessage, key string, target *string) {
	if raw := values[key]; len(raw) > 0 {
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			*target = value
		}
	}
}

// readJSONInt32 从 RawMessage map 中读取 int32 字段。
func readJSONInt32(values map[string]json.RawMessage, key string, target *int32) {
	if raw := values[key]; len(raw) > 0 {
		var value int32
		if err := json.Unmarshal(raw, &value); err == nil {
			*target = value
		}
	}
}

// defaultString 返回非空字符串或默认值。
func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

// stringPtrOrNil 将空字符串归一为空指针。
func stringPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// optionalStatus 根据配置项是否存在返回健康状态。
func optionalStatus(value string) string {
	if strings.TrimSpace(value) == "" {
		return "degraded"
	}
	return "degraded"
}

// truncateSummary 对 JSON 摘要做长度截断。
func truncateSummary(raw json.RawMessage, limit int) string {
	text := strings.TrimSpace(string(raw))
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

// traceSpanType 根据事件类型推导 span 类型。
func traceSpanType(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "LLM_"):
		return "LLM"
	case strings.HasPrefix(eventType, "TOOL_"):
		return "TOOL"
	case strings.Contains(eventType, "CHECKPOINT"):
		return "CHECKPOINT"
	default:
		return "STEP"
	}
}

// traceSpanStatus 根据事件类型推导 span 状态。
func traceSpanStatus(eventType string) string {
	if strings.Contains(eventType, "FAILED") || strings.Contains(eventType, "ERROR") {
		return "error"
	}
	return "ok"
}

// durationFromEventOutput 从事件 output 中提取 duration_ms。
func durationFromEventOutput(output json.RawMessage) int64 {
	var value map[string]any
	if err := json.Unmarshal(output, &value); err != nil {
		return 0
	}
	if raw, ok := value["duration_ms"].(float64); ok {
		return int64(raw)
	}
	return 0
}

// logLevelForEvent 根据事件类型映射日志级别。
func logLevelForEvent(eventType string) string {
	switch {
	case strings.Contains(eventType, "FAILED"), strings.Contains(eventType, "ERROR"):
		return "error"
	case eventType == "LOOP_DETECTED", eventType == "LIMIT_EXCEEDED":
		return "warn"
	default:
		return "info"
	}
}

// sanitizeFilename 清理下载文件名中的引号。
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), `"`, "")
	if name == "" {
		return "artifact.json"
	}
	return name
}

// defaultSettings 返回设计稿第 16 节对应的默认分组。
func defaultSettings() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"runtime":       json.RawMessage(`{"default_max_steps":50,"default_max_tokens":200000,"default_timeout":"30m","checkpoint_retention":20,"loop_detector_threshold":3,"worker_concurrency_limit":1}`),
		"llm":           json.RawMessage(`{"default_model":"mock","fallback_model":"","provider_config":{},"token_price":{},"request_timeout":"60s","retry_count":2,"rate_limit":{}}`),
		"tool":          json.RawMessage(`{"default_approval_policy":"high_risk","high_risk_tools":[],"disabled_tools":[],"tool_timeout":"30s","tool_retry":1}`),
		"security":      json.RawMessage(`{"jwt_expiration":"24h","oidc":{},"api_key_policy":{},"audit_log_retention_days":365,"redact_fields":["token","secret","password","api_key","authorization"]}`),
		"retention":     json.RawMessage(`{"event_days":90,"checkpoint_days":30,"artifact_days":90,"logs_days":30}`),
		"observability": json.RawMessage(`{"trace_url_template":""}`),
	}
}

// sortLogItems 按时间倒序排序日志行。
func sortLogItems(items []logItem) {
	sort.Slice(items, func(i int, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})
}
