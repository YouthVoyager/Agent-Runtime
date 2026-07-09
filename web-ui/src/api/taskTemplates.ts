import { api } from './client';
import type { TaskTemplate } from '@/types/task';

export const taskTemplatesApi = {
  list: () => api.get<{ items: TaskTemplate[] }>('/task-templates'),
  create: (body: { name: string; payload: unknown; shared?: boolean }) =>
    api.post<TaskTemplate>('/task-templates', body),
  remove: (templateId: string) => api.delete<void>(`/task-templates/${templateId}`),
};
