package repository

import (
	"context"
	"errors"
	"testing"

	"agent-runtime/internal/infra/postgres/db"

	"github.com/jackc/pgx/v5"
)

// TestMapNotFound 验证 pgx 无行错误会映射为仓储层不存在错误。
func TestMapNotFound(t *testing.T) {
	if got := mapNotFound(nil); got != nil {
		t.Fatalf("期望 nil 错误，实际: %v", got)
	}
	if got := mapNotFound(pgx.ErrNoRows); !errors.Is(got, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound，实际: %v", got)
	}

	sourceErr := errors.New("数据库连接失败")
	if got := mapNotFound(sourceErr); !errors.Is(got, sourceErr) {
		t.Fatalf("期望保留原始错误，实际: %v", got)
	}
}

// TestTaskRepositoryGetMapsNotFound 验证任务仓储查询不存在时返回 ErrNotFound。
func TestTaskRepositoryGetMapsNotFound(t *testing.T) {
	repo := NewTaskRepository(taskQueriesStub{
		getAgentTask: func(_ context.Context, arg db.GetAgentTaskParams) (db.AgentTask, error) {
			if arg.TenantID != "tenant_001" || arg.TaskID != "task_001" {
				t.Fatalf("租户或任务参数未正确透传: %+v", arg)
			}
			return db.AgentTask{}, pgx.ErrNoRows
		},
	})

	_, err := repo.Get(context.Background(), "tenant_001", "task_001")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound，实际: %v", err)
	}
}

// TestEventRepositoryAppendDelegates 验证事件仓储会把追加事件参数透传给查询接口。
func TestEventRepositoryAppendDelegates(t *testing.T) {
	repo := NewEventRepository(eventQueriesStub{
		createAgentEvent: func(_ context.Context, arg db.CreateAgentEventParams) (db.AgentEvent, error) {
			if arg.TenantID != "tenant_001" || arg.TaskID != "task_001" || arg.Type != "TASK_CREATED" {
				t.Fatalf("事件参数未正确透传: %+v", arg)
			}
			return db.AgentEvent{EventID: 7, TenantID: arg.TenantID, TaskID: arg.TaskID, Type: arg.Type}, nil
		},
	})

	event, err := repo.Append(context.Background(), db.CreateAgentEventParams{
		TenantID: "tenant_001",
		TaskID:   "task_001",
		Type:     "TASK_CREATED",
	})
	if err != nil {
		t.Fatalf("Append 返回错误: %v", err)
	}
	if event.EventID != 7 {
		t.Fatalf("期望 event_id=7，实际: %d", event.EventID)
	}
}

// TestStateRepositoryUpdateOptimisticMapsVersionConflict 验证状态乐观更新未命中时返回版本冲突。
func TestStateRepositoryUpdateOptimisticMapsVersionConflict(t *testing.T) {
	repo := NewStateRepository(stateQueriesStub{
		updateAgentStateOptimistic: func(_ context.Context, arg db.UpdateAgentStateOptimisticParams) (db.AgentState, error) {
			if arg.TenantID != "tenant_001" || arg.TaskID != "task_001" || arg.Version != 3 {
				t.Fatalf("状态更新参数未正确透传: %+v", arg)
			}
			return db.AgentState{}, pgx.ErrNoRows
		},
	})

	_, err := repo.UpdateOptimistic(context.Background(), db.UpdateAgentStateOptimisticParams{
		TenantID: "tenant_001",
		TaskID:   "task_001",
		Version:  3,
	})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("期望 ErrVersionConflict，实际: %v", err)
	}
}

// TestToolCallRepositoryGetByIdempotencyKeyMapsNotFound 验证工具调用幂等查询不存在时返回 ErrNotFound。
func TestToolCallRepositoryGetByIdempotencyKeyMapsNotFound(t *testing.T) {
	repo := NewToolCallRepository(toolCallQueriesStub{
		getToolCallByIdempotencyKey: func(_ context.Context, arg db.GetToolCallByIdempotencyKeyParams) (db.ToolCall, error) {
			if arg.TenantID != "tenant_001" || arg.IdempotencyKey != "tenant_001:task_001:read_file:001" {
				t.Fatalf("幂等查询参数未正确透传: %+v", arg)
			}
			return db.ToolCall{}, pgx.ErrNoRows
		},
	})

	_, err := repo.GetByIdempotencyKey(context.Background(), "tenant_001", "tenant_001:task_001:read_file:001")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound，实际: %v", err)
	}
}

// TestCheckpointRepositoryLatestMapsNotFound 验证最新 checkpoint 查询不存在时返回 ErrNotFound。
func TestCheckpointRepositoryLatestMapsNotFound(t *testing.T) {
	repo := NewCheckpointRepository(checkpointQueriesStub{
		getLatestCheckpoint: func(_ context.Context, arg db.GetLatestCheckpointParams) (db.Checkpoint, error) {
			if arg.TenantID != "tenant_001" || arg.TaskID != "task_001" {
				t.Fatalf("checkpoint 查询参数未正确透传: %+v", arg)
			}
			return db.Checkpoint{}, pgx.ErrNoRows
		},
	})

	_, err := repo.Latest(context.Background(), "tenant_001", "task_001")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound，实际: %v", err)
	}
}

type taskQueriesStub struct {
	createAgentTask            func(context.Context, db.CreateAgentTaskParams) (db.AgentTask, error)
	getAgentTask               func(context.Context, db.GetAgentTaskParams) (db.AgentTask, error)
	listAgentTasksByTenant     func(context.Context, db.ListAgentTasksByTenantParams) ([]db.AgentTask, error)
	listAgentTasksByUserStatus func(context.Context, db.ListAgentTasksByUserAndStatusParams) ([]db.AgentTask, error)
	updateAgentTaskStatus      func(context.Context, db.UpdateAgentTaskStatusParams) (db.AgentTask, error)
	updateAgentTaskWorkflow    func(context.Context, db.UpdateAgentTaskWorkflowParams) (db.AgentTask, error)
	updateAgentTaskBudgetUsage func(context.Context, db.UpdateAgentTaskBudgetUsageParams) (db.AgentTask, error)
	markAgentTaskFailed        func(context.Context, db.MarkAgentTaskFailedParams) (db.AgentTask, error)
}

// CreateAgentTask 调用测试注入的任务创建查询函数。
func (s taskQueriesStub) CreateAgentTask(ctx context.Context, arg db.CreateAgentTaskParams) (db.AgentTask, error) {
	return s.createAgentTask(ctx, arg)
}

// GetAgentTask 调用测试注入的任务详情查询函数。
func (s taskQueriesStub) GetAgentTask(ctx context.Context, arg db.GetAgentTaskParams) (db.AgentTask, error) {
	return s.getAgentTask(ctx, arg)
}

// ListAgentTasksByTenant 调用测试注入的租户任务列表查询函数。
func (s taskQueriesStub) ListAgentTasksByTenant(ctx context.Context, arg db.ListAgentTasksByTenantParams) ([]db.AgentTask, error) {
	return s.listAgentTasksByTenant(ctx, arg)
}

// ListAgentTasksByUserAndStatus 调用测试注入的用户状态任务列表查询函数。
func (s taskQueriesStub) ListAgentTasksByUserAndStatus(ctx context.Context, arg db.ListAgentTasksByUserAndStatusParams) ([]db.AgentTask, error) {
	return s.listAgentTasksByUserStatus(ctx, arg)
}

// UpdateAgentTaskStatus 调用测试注入的任务状态更新函数。
func (s taskQueriesStub) UpdateAgentTaskStatus(ctx context.Context, arg db.UpdateAgentTaskStatusParams) (db.AgentTask, error) {
	return s.updateAgentTaskStatus(ctx, arg)
}

// UpdateAgentTaskWorkflow 调用测试注入的任务 workflow 更新函数。
func (s taskQueriesStub) UpdateAgentTaskWorkflow(ctx context.Context, arg db.UpdateAgentTaskWorkflowParams) (db.AgentTask, error) {
	return s.updateAgentTaskWorkflow(ctx, arg)
}

// UpdateAgentTaskBudgetUsage 调用测试注入的任务预算用量更新函数。
func (s taskQueriesStub) UpdateAgentTaskBudgetUsage(ctx context.Context, arg db.UpdateAgentTaskBudgetUsageParams) (db.AgentTask, error) {
	return s.updateAgentTaskBudgetUsage(ctx, arg)
}

// MarkAgentTaskFailed 调用测试注入的任务失败标记函数。
func (s taskQueriesStub) MarkAgentTaskFailed(ctx context.Context, arg db.MarkAgentTaskFailedParams) (db.AgentTask, error) {
	return s.markAgentTaskFailed(ctx, arg)
}

type eventQueriesStub struct {
	createAgentEvent func(context.Context, db.CreateAgentEventParams) (db.AgentEvent, error)
	getAgentEvent    func(context.Context, db.GetAgentEventParams) (db.AgentEvent, error)
	listEventsByTask func(context.Context, db.ListAgentEventsByTaskParams) ([]db.AgentEvent, error)
	listEventsByType func(context.Context, db.ListAgentEventsByTypeParams) ([]db.AgentEvent, error)
}

// CreateAgentEvent 调用测试注入的事件创建函数。
func (s eventQueriesStub) CreateAgentEvent(ctx context.Context, arg db.CreateAgentEventParams) (db.AgentEvent, error) {
	return s.createAgentEvent(ctx, arg)
}

// GetAgentEvent 调用测试注入的事件详情查询函数。
func (s eventQueriesStub) GetAgentEvent(ctx context.Context, arg db.GetAgentEventParams) (db.AgentEvent, error) {
	return s.getAgentEvent(ctx, arg)
}

// ListAgentEventsByTask 调用测试注入的任务事件列表查询函数。
func (s eventQueriesStub) ListAgentEventsByTask(ctx context.Context, arg db.ListAgentEventsByTaskParams) ([]db.AgentEvent, error) {
	return s.listEventsByTask(ctx, arg)
}

// ListAgentEventsByType 调用测试注入的事件类型列表查询函数。
func (s eventQueriesStub) ListAgentEventsByType(ctx context.Context, arg db.ListAgentEventsByTypeParams) ([]db.AgentEvent, error) {
	return s.listEventsByType(ctx, arg)
}

type stateQueriesStub struct {
	createAgentState           func(context.Context, db.CreateAgentStateParams) (db.AgentState, error)
	getAgentState              func(context.Context, db.GetAgentStateParams) (db.AgentState, error)
	updateAgentStateOptimistic func(context.Context, db.UpdateAgentStateOptimisticParams) (db.AgentState, error)
}

// CreateAgentState 调用测试注入的状态创建函数。
func (s stateQueriesStub) CreateAgentState(ctx context.Context, arg db.CreateAgentStateParams) (db.AgentState, error) {
	return s.createAgentState(ctx, arg)
}

// GetAgentState 调用测试注入的状态查询函数。
func (s stateQueriesStub) GetAgentState(ctx context.Context, arg db.GetAgentStateParams) (db.AgentState, error) {
	return s.getAgentState(ctx, arg)
}

// UpdateAgentStateOptimistic 调用测试注入的状态乐观更新函数。
func (s stateQueriesStub) UpdateAgentStateOptimistic(ctx context.Context, arg db.UpdateAgentStateOptimisticParams) (db.AgentState, error) {
	return s.updateAgentStateOptimistic(ctx, arg)
}

type toolCallQueriesStub struct {
	createToolCall               func(context.Context, db.CreateToolCallParams) (db.ToolCall, error)
	getToolCall                  func(context.Context, db.GetToolCallParams) (db.ToolCall, error)
	getToolCallByIdempotencyKey  func(context.Context, db.GetToolCallByIdempotencyKeyParams) (db.ToolCall, error)
	listToolCallsByTask          func(context.Context, db.ListToolCallsByTaskParams) ([]db.ToolCall, error)
	startToolCall                func(context.Context, db.StartToolCallParams) (db.ToolCall, error)
	completeToolCall             func(context.Context, db.CompleteToolCallParams) (db.ToolCall, error)
	failToolCall                 func(context.Context, db.FailToolCallParams) (db.ToolCall, error)
	updateToolCallApprovalStatus func(context.Context, db.UpdateToolCallApprovalStatusParams) (db.ToolCall, error)
}

// CreateToolCall 调用测试注入的工具调用创建函数。
func (s toolCallQueriesStub) CreateToolCall(ctx context.Context, arg db.CreateToolCallParams) (db.ToolCall, error) {
	return s.createToolCall(ctx, arg)
}

// GetToolCall 调用测试注入的工具调用查询函数。
func (s toolCallQueriesStub) GetToolCall(ctx context.Context, arg db.GetToolCallParams) (db.ToolCall, error) {
	return s.getToolCall(ctx, arg)
}

// GetToolCallByIdempotencyKey 调用测试注入的工具调用幂等查询函数。
func (s toolCallQueriesStub) GetToolCallByIdempotencyKey(ctx context.Context, arg db.GetToolCallByIdempotencyKeyParams) (db.ToolCall, error) {
	return s.getToolCallByIdempotencyKey(ctx, arg)
}

// ListToolCallsByTask 调用测试注入的任务工具调用列表查询函数。
func (s toolCallQueriesStub) ListToolCallsByTask(ctx context.Context, arg db.ListToolCallsByTaskParams) ([]db.ToolCall, error) {
	return s.listToolCallsByTask(ctx, arg)
}

// StartToolCall 调用测试注入的工具调用开始函数。
func (s toolCallQueriesStub) StartToolCall(ctx context.Context, arg db.StartToolCallParams) (db.ToolCall, error) {
	return s.startToolCall(ctx, arg)
}

// CompleteToolCall 调用测试注入的工具调用完成函数。
func (s toolCallQueriesStub) CompleteToolCall(ctx context.Context, arg db.CompleteToolCallParams) (db.ToolCall, error) {
	return s.completeToolCall(ctx, arg)
}

// FailToolCall 调用测试注入的工具调用失败函数。
func (s toolCallQueriesStub) FailToolCall(ctx context.Context, arg db.FailToolCallParams) (db.ToolCall, error) {
	return s.failToolCall(ctx, arg)
}

// UpdateToolCallApprovalStatus 调用测试注入的工具调用审批状态更新函数。
func (s toolCallQueriesStub) UpdateToolCallApprovalStatus(ctx context.Context, arg db.UpdateToolCallApprovalStatusParams) (db.ToolCall, error) {
	return s.updateToolCallApprovalStatus(ctx, arg)
}

type checkpointQueriesStub struct {
	createCheckpoint    func(context.Context, db.CreateCheckpointParams) (db.Checkpoint, error)
	getCheckpoint       func(context.Context, db.GetCheckpointParams) (db.Checkpoint, error)
	getLatestCheckpoint func(context.Context, db.GetLatestCheckpointParams) (db.Checkpoint, error)
	listByTask          func(context.Context, db.ListCheckpointsByTaskParams) ([]db.Checkpoint, error)
}

// CreateCheckpoint 调用测试注入的 checkpoint 创建函数。
func (s checkpointQueriesStub) CreateCheckpoint(ctx context.Context, arg db.CreateCheckpointParams) (db.Checkpoint, error) {
	return s.createCheckpoint(ctx, arg)
}

// GetCheckpoint 调用测试注入的 checkpoint 查询函数。
func (s checkpointQueriesStub) GetCheckpoint(ctx context.Context, arg db.GetCheckpointParams) (db.Checkpoint, error) {
	return s.getCheckpoint(ctx, arg)
}

// GetLatestCheckpoint 调用测试注入的最新 checkpoint 查询函数。
func (s checkpointQueriesStub) GetLatestCheckpoint(ctx context.Context, arg db.GetLatestCheckpointParams) (db.Checkpoint, error) {
	return s.getLatestCheckpoint(ctx, arg)
}

// ListCheckpointsByTask 调用测试注入的任务 checkpoint 列表查询函数。
func (s checkpointQueriesStub) ListCheckpointsByTask(ctx context.Context, arg db.ListCheckpointsByTaskParams) ([]db.Checkpoint, error) {
	return s.listByTask(ctx, arg)
}
