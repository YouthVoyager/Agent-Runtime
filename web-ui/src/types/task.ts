// 任务领域类型。字段命名与 api/openapi.yaml / internal/domain/task 中的真实状态保持一致。
export type TaskStatus =
  | 'QUEUED'
  | 'RUNNING'
  | 'WAITING_APPROVAL'
  | 'PAUSED'
  | 'CANCELING'
  | 'CANCELED'
  | 'SUCCEEDED'
  | 'FAILED'
  | 'RETRYABLE_FAILED'
  | 'STOPPED_BY_LIMIT';

// 注意：Budget 在 openapi CreateTaskRequest 中 additionalProperties: false，
// 只允许这四个字段；timeout 等扩展字段一律放进 constraints（额外 JSON,允许任意字段）。
export interface Budget {
  max_steps: number;
  max_tokens: number;
  max_tool_calls: number;
  max_cost_usd: number;
}

export interface BudgetUsage {
  used_steps: number;
  used_tokens: number;
  used_tool_calls: number;
  used_cost_usd: number;
}

export interface TaskSummary {
  task_id: string;
  goal: string;
  status: TaskStatus;
  budget: Budget;
  budget_usage: BudgetUsage;
  trace_id?: string;
  user_id?: string;
  tenant_id?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskDetail {
  task_id: string;
  goal: string;
  status: TaskStatus;
  current_step: number;
  budget: Budget;
  budget_usage: BudgetUsage;
  trace_id?: string;
  workflow_id?: string;
  user_id?: string;
  tenant_id?: string;
  last_error_code?: string;
  last_error_message?: string;
  created_at: string;
  updated_at: string;
  started_at?: string;
  finished_at?: string;
}

export interface ListTasksParams {
  status?: TaskStatus;
  goal?: string;
  user_id?: string;
  tenant_id?: string;
  created_from?: string;
  created_to?: string;
  waiting_approval?: boolean;
  stopped_by_limit?: boolean;
  page?: number;
  page_size?: number;
}

export interface CreateTaskRequest {
  goal: string;
  task_name?: string;
  priority?: 'low' | 'normal' | 'high';
  tags?: string[];
  client_request_id?: string;
  budget: Budget;
  constraints?: Record<string, unknown>;
}

export interface CreatedTask {
  task_id: string;
  status: TaskStatus;
  created_at: string;
}

export interface TaskStatusResult {
  task_id: string;
  status: TaskStatus;
}

export interface ResumeTaskResult {
  task_id: string;
  status: TaskStatus;
  resume_from_checkpoint_id?: string;
}

export interface AgentStateResponse {
  task_id: string;
  version: number;
  current_plan?: unknown;
  current_step: number;
  memory_summary?: string;
  constraints?: Record<string, unknown>;
  artifacts?: unknown[];
  loop_fingerprints?: unknown[];
  state?: Record<string, unknown>;
  updated_at: string;
}

export interface ToolCall {
  call_id: string;
  task_id: string;
  tool_name: string;
  status: string;
  risk_level: string;
  approval_status: string;
  result?: unknown;
  error_code?: string;
  error_message?: string;
  idempotency_key?: string;
  duration_ms?: number;
  arguments?: unknown;
  created_at: string;
  updated_at: string;
}

export interface Checkpoint {
  checkpoint_id: string;
  task_id: string;
  state_version: number;
  event_offset: number;
  state_snapshot?: Record<string, unknown>;
  reason: string;
  trace_id?: string;
  created_at: string;
}

export interface Artifact {
  artifact_id: string;
  task_id: string;
  type: string;
  name: string;
  storage_backend?: string;
  content_json?: unknown;
  size_bytes: number;
  status: string;
  created_by_event_id?: number;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface TaskTemplate {
  template_id: string;
  name: string;
  payload: CreateTaskRequest;
  shared: boolean;
  created_by: string;
  created_at: string;
}
