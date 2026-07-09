import { api } from './client';
import type { Page } from '@/types/common';
import type { ListToolCallsParams, ToolCallCrossTaskItem, ToolCallDetail } from '@/types/tool';

export const toolCallsApi = {
  list: (params: ListToolCallsParams) =>
    api.get<Page<ToolCallCrossTaskItem>>('/tool-calls', params as Record<string, string>),
  get: (callId: string) => api.get<ToolCallDetail>(`/tool-calls/${callId}`),
};
