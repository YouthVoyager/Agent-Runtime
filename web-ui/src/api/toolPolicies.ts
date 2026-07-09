import { api } from './client';
import type { Page } from '@/types/common';
import type { ListToolPoliciesParams, SimulatePolicyRequest, SimulatePolicyResult, ToolPolicy } from '@/types/tool';

export const toolPoliciesApi = {
  list: (params: ListToolPoliciesParams) =>
    api.get<Page<ToolPolicy>>('/tool-policies', params as Record<string, string>),
  create: (body: Partial<ToolPolicy>) => api.post<ToolPolicy>('/tool-policies', body),
  get: (policyId: string) => api.get<ToolPolicy>(`/tool-policies/${policyId}`),
  update: (policyId: string, body: Partial<ToolPolicy>) => api.put<ToolPolicy>(`/tool-policies/${policyId}`, body),
  disable: (policyId: string) => api.delete<void>(`/tool-policies/${policyId}`),
  simulate: (body: SimulatePolicyRequest) => api.post<SimulatePolicyResult>('/tool-policies/simulate', body),
};
