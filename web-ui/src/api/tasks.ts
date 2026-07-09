import { api } from './client';
import type { Page } from '@/types/common';
import type {
  AgentStateResponse,
  Artifact,
  Checkpoint,
  CreatedTask,
  CreateTaskRequest,
  ListTasksParams,
  ResumeTaskResult,
  TaskDetail,
  TaskStatusResult,
  TaskSummary,
  ToolCall,
} from '@/types/task';
import type { AgentEvent, ListTaskEventsResult } from '@/types/event';

export const tasksApi = {
  list: (params: ListTasksParams) => api.get<Page<TaskSummary>>('/tasks', params as Record<string, string>),
  create: (body: CreateTaskRequest) => api.post<CreatedTask>('/tasks', body),
  get: (taskId: string) => api.get<TaskDetail>(`/tasks/${taskId}`),
  events: (taskId: string, params?: { after_event_id?: number; limit?: number; type?: string }) =>
    api.get<ListTaskEventsResult>(`/tasks/${taskId}/events`, params as Record<string, string>),
  state: (taskId: string) => api.get<AgentStateResponse>(`/tasks/${taskId}/state`),
  toolCalls: (taskId: string, params?: { limit?: number; offset?: number }) =>
    api.get<{ items: ToolCall[] }>(`/tasks/${taskId}/tool-calls`, params as Record<string, number>),
  checkpoints: (taskId: string, params?: { limit?: number; offset?: number }) =>
    api.get<{ items: Checkpoint[] }>(`/tasks/${taskId}/checkpoints`, params as Record<string, number>),
  artifacts: (taskId: string, params?: { limit?: number; offset?: number }) =>
    api.get<{ items: Artifact[] }>(`/tasks/${taskId}/artifacts`, params as Record<string, number>),
  cancel: (taskId: string) => api.post<TaskStatusResult>(`/tasks/${taskId}/cancel`),
  pause: (taskId: string) => api.post<TaskStatusResult>(`/tasks/${taskId}/pause`),
  resume: (taskId: string) => api.post<ResumeTaskResult>(`/tasks/${taskId}/resume`),
  resumeFromCheckpoint: (taskId: string, checkpointId: string) =>
    api.post<ResumeTaskResult>(`/tasks/${taskId}/resume-from-checkpoint`, { checkpoint_id: checkpointId }),
};

export type { AgentEvent };
