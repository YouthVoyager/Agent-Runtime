export interface ToolListItem {
  tool_name: string;
  description: string;
  category?: string;
  default_risk_level: string;
  enabled: boolean;
  version?: string;
  owner?: string;
  has_side_effect?: boolean;
  requires_idempotency_key?: boolean;
  timeout_seconds?: number;
  created_at: string;
  updated_at: string;
}

export interface ToolAvailableItem {
  tool_name: string;
  description: string;
  risk_level: string;
  approval_required: boolean;
}

export interface ToolDetail extends ToolListItem {
  params_schema?: unknown;
  result_schema?: unknown;
  retry_policy?: unknown;
  calls_7d?: number;
  error_rate_7d?: number;
  avg_duration_ms_7d?: number;
}

export interface UpdateToolRequest {
  enabled?: boolean;
  default_risk_level?: string;
  timeout_seconds?: number;
  description?: string;
  category?: string;
  params_schema?: unknown;
  retry_policy?: unknown;
}

export interface ToolPolicy {
  policy_id: string;
  tenant_id: string;
  tool_name: string;
  role: string;
  enabled: boolean;
  approval_required: boolean;
  max_calls_per_day?: number;
  risk_level_override?: string;
  argument_rules?: Record<string, unknown>;
  timeout_seconds?: number;
  created_at?: string;
  updated_at?: string;
}

export interface ListToolPoliciesParams {
  tenant_id?: string;
  tool_name?: string;
  role?: string;
  enabled?: boolean;
  page?: number;
  page_size?: number;
}

export interface SimulatePolicyRequest {
  tenant_id?: string;
  user_id?: string;
  role: string;
  tool_name: string;
  arguments?: Record<string, unknown>;
}

export interface SimulatePolicyResult {
  allowed: boolean;
  risk_level: string;
  approval_required: boolean;
  matched_policy_id?: string;
  deny_reason?: string;
}

export interface ToolCallCrossTaskItem {
  call_id: string;
  task_id: string;
  tenant_id?: string;
  tool_name: string;
  risk_level: string;
  status: string;
  approval_status: string;
  duration_ms?: number;
  error_code?: string;
  idempotency_key?: string;
  created_at: string;
}

export interface ListToolCallsParams {
  task_id?: string;
  call_id?: string;
  tool_name?: string;
  status?: string;
  risk_level?: string;
  approval_status?: string;
  user_id?: string;
  tenant_id?: string;
  error_code?: string;
  idempotency_key?: string;
  created_from?: string;
  created_to?: string;
  page?: number;
  page_size?: number;
}

export interface ToolCallDetail extends ToolCallCrossTaskItem {
  arguments?: unknown;
  arguments_hash?: string;
  result?: unknown;
  error_message?: string;
  approvals?: ApprovalHistoryLike[];
  trace_id?: string;
}

export interface ApprovalHistoryLike {
  approval_id: string;
  status: string;
  approver_id?: string;
  note?: string;
  decided_at?: string;
}
