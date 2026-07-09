import { api } from './client';
import type { ToolAvailableItem, ToolDetail, ToolListItem, UpdateToolRequest } from '@/types/tool';

export const toolsApi = {
  list: () => api.get<{ items: ToolListItem[] }>('/tools'),
  available: () => api.get<ToolAvailableItem[]>('/tools/available'),
  get: (toolName: string) => api.get<ToolDetail>(`/tools/${toolName}`),
  update: (toolName: string, body: UpdateToolRequest) => api.patch<ToolDetail>(`/tools/${toolName}`, body),
};
