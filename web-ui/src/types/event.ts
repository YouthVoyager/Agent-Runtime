// AgentEvent timeline 类型，见 api/openapi.yaml AgentEventType。
export type AgentEventType =
  | 'TASK_CREATED'
  | 'TASK_STARTED'
  | 'TASK_COMPLETED'
  | 'TASK_FAILED'
  | 'TASK_CANCELLED'
  | 'TASK_RESUMED'
  | 'STEP_STARTED'
  | 'STEP_COMPLETED'
  | 'STEP_FAILED'
  | 'LLM_CALL_STARTED'
  | 'LLM_CALL_COMPLETED'
  | 'TOOL_CALL_STARTED'
  | 'TOOL_CALL_COMPLETED'
  | 'TOOL_APPROVAL_REQUIRED'
  | 'TOOL_APPROVAL_DECIDED'
  | 'CHECKPOINT_CREATED'
  | 'ARTIFACT_SAVED'
  | 'LOOP_DETECTED'
  | 'LIMIT_EXCEEDED';

export interface AgentEventMetadata {
  user_id?: string;
  request_id?: string;
  source?: string;
  step_id?: string;
  workflow_id?: string;
  attempt?: number;
  [key: string]: unknown;
}

export interface AgentEvent {
  event_id: number;
  task_id: string;
  type: AgentEventType | string;
  input?: unknown;
  output?: unknown;
  metadata?: AgentEventMetadata;
  trace_id?: string;
  span_id?: string;
  created_at: string;
}

export interface ListTaskEventsResult {
  items: AgentEvent[];
  next_after_event_id: number;
  has_more: boolean;
}

// Timeline Explorer 跨任务事件查询项（截断 + 脱敏后的摘要）。
export interface EventSummaryItem {
  event_id: number;
  task_id: string;
  type: string;
  input_summary?: string;
  output_summary?: string;
  metadata?: AgentEventMetadata;
  trace_id?: string;
  created_at: string;
}

export interface ListEventsParams {
  task_id?: string;
  event_id?: number;
  type?: string;
  tool_name?: string;
  error_code?: string;
  trace_id?: string;
  user_id?: string;
  tenant_id?: string;
  created_from?: string;
  created_to?: string;
  page?: number;
  page_size?: number;
}
