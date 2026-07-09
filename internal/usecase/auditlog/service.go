// Package auditlog 提供统一的审计日志写入能力。
// 契约 docs/admin-console-api-contract.md 第 12 节要求:
// task.create / task.cancel / task.pause / task.resume / task.resume_from_checkpoint /
// approval.approve / approval.reject / tool.update / tool_policy.create|update|delete /
// tenant.create|update / user.invite|update / artifact.delete / settings.update
// 等动作必须落 audit_logs，字段包含 actor、tenant、action、resource、before/after、ip、user_agent。
package auditlog

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"stableagent/internal/infra/postgres/db"
	"stableagent/pkg/ids"
)

type Service struct {
	queries *db.Queries
	logger  *slog.Logger
}

// LogInput 描述一条审计记录的全部字段。
type LogInput struct {
	ActorID      string
	TenantID     string
	Action       string
	ResourceType string
	ResourceID   string
	Before       any
	After        any
	IP           string
	UserAgent    string
}

// NewService 创建审计日志用例服务。
func NewService(pool *pgxpool.Pool, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{queries: db.New(pool), logger: logger}
}

// Log 写入一条审计记录。审计写入失败不应阻断主业务流程，这里内部吞掉错误并记录 warn 日志，
// 调用方按需以 `_ = auditSvc.Log(...)` 的方式使用，与仓库内 appendEvent 系列的尽力而为风格保持一致。
func (s *Service) Log(ctx context.Context, input LogInput) error {
	if s == nil || s.queries == nil {
		return nil
	}
	actorID := strings.TrimSpace(input.ActorID)
	action := strings.TrimSpace(input.Action)
	resourceType := strings.TrimSpace(input.ResourceType)
	if actorID == "" || action == "" || resourceType == "" {
		s.logger.WarnContext(ctx, "审计日志缺少必填字段，已跳过写入", "action", action, "resource_type", resourceType)
		return nil
	}

	auditID, err := ids.New("audit")
	if err != nil {
		s.logger.WarnContext(ctx, "生成 audit_id 失败", "error", err)
		return err
	}

	before, err := marshalOptional(input.Before)
	if err != nil {
		s.logger.WarnContext(ctx, "序列化审计 before 失败", "error", err)
	}
	after, err := marshalOptional(input.After)
	if err != nil {
		s.logger.WarnContext(ctx, "序列化审计 after 失败", "error", err)
	}

	params := db.CreateAuditLogParams{
		AuditID:      auditID,
		ActorID:      actorID,
		Action:       action,
		ResourceType: resourceType,
		Before:       before,
		After:        after,
	}
	if tenantID := strings.TrimSpace(input.TenantID); tenantID != "" {
		params.TenantID = &tenantID
	}
	if resourceID := strings.TrimSpace(input.ResourceID); resourceID != "" {
		params.ResourceID = &resourceID
	}
	if ip := strings.TrimSpace(input.IP); ip != "" {
		params.Ip = &ip
	}
	if ua := strings.TrimSpace(input.UserAgent); ua != "" {
		params.UserAgent = &ua
	}

	if _, err := s.queries.CreateAuditLog(ctx, params); err != nil {
		s.logger.WarnContext(ctx, "写入审计日志失败", "action", action, "resource_type", resourceType, "error", err)
		return err
	}
	return nil
}

// marshalOptional 将任意值编码为 JSON，nil 返回 nil 保持列可空。
func marshalOptional(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		if len(raw) == 0 {
			return nil, nil
		}
		return raw, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
