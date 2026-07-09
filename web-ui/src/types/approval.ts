export type ApprovalStatus = 'PENDING' | 'APPROVED' | 'REJECTED' | 'EXPIRED';

export interface ApprovalItem {
  approval_id: string;
  task_id: string;
  call_id: string;
  tool_name: string;
  risk_level: string;
  approval_reason?: string;
  requester_id: string;
  approver_id?: string;
  status: ApprovalStatus;
  note?: string;
  created_at: string;
  decided_at?: string;
  expires_at?: string;
}

export interface ApprovalHistoryEntry {
  approval_id: string;
  status: ApprovalStatus;
  approver_id?: string;
  note?: string;
  decided_at?: string;
  created_at: string;
}

export interface ApprovalDetail extends ApprovalItem {
  arguments?: unknown;
  task_goal?: string;
  current_step?: number;
  idempotency_key?: string;
  history: ApprovalHistoryEntry[];
}

export interface ListApprovalsParams {
  status?: ApprovalStatus;
  mine?: boolean;
  task_id?: string;
  tool_name?: string;
  risk_level?: string;
  created_from?: string;
  created_to?: string;
  page?: number;
  page_size?: number;
}

export interface ApproveToolCallRequest {
  note?: string;
}

export interface RejectToolCallRequest {
  reason: string;
  terminate_task?: boolean;
}

export interface ToolCallDecisionResult {
  call_id: string;
  approval_status: string;
  tool_call_status: string;
  task_status: string;
}
