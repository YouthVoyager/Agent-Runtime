import { api } from './client';
import type { Page } from '@/types/common';
import type {
  ApprovalDetail,
  ApprovalItem,
  ApproveToolCallRequest,
  ListApprovalsParams,
  RejectToolCallRequest,
  ToolCallDecisionResult,
} from '@/types/approval';

export const approvalsApi = {
  list: (params: ListApprovalsParams) => api.get<Page<ApprovalItem>>('/approvals', params as Record<string, string>),
  get: (approvalId: string) => api.get<ApprovalDetail>(`/approvals/${approvalId}`),
  approve: (callId: string, body: ApproveToolCallRequest) =>
    api.post<ToolCallDecisionResult>(`/tool-calls/${callId}/approve`, body),
  reject: (callId: string, body: RejectToolCallRequest) =>
    api.post<ToolCallDecisionResult>(`/tool-calls/${callId}/reject`, body),
};
