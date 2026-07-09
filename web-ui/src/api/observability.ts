import { api } from './client';
import type { Page } from '@/types/common';
import type { EventSummaryItem, ListEventsParams } from '@/types/event';
import type { ListLogsParams, LogItem, MetricsSnapshot, TraceDetail } from '@/types/observability';
import type { DashboardRange } from '@/types/dashboard';

export const observabilityApi = {
  events: (params: ListEventsParams) => api.get<Page<EventSummaryItem>>('/events', params as Record<string, string>),
  trace: (traceId: string) => api.get<TraceDetail>(`/observability/trace/${traceId}`),
  metrics: (range: DashboardRange = '24h') => api.get<MetricsSnapshot>('/observability/metrics', { range }),
  logs: (params: ListLogsParams) => api.get<Page<LogItem>>('/observability/logs', params as Record<string, string>),
};
