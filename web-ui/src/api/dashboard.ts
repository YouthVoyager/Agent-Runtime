import { api } from './client';
import type { CostStats, DashboardRange, DashboardSummary, RiskStats, SystemHealth, TaskStats } from '@/types/dashboard';

export interface DashboardParams {
  range?: DashboardRange;
  tenant_id?: string;
}

export const dashboardApi = {
  summary: (params: DashboardParams) => api.get<DashboardSummary>('/dashboard/summary', params as Record<string, string>),
  taskStats: (params: DashboardParams) => api.get<TaskStats>('/dashboard/task-stats', params as Record<string, string>),
  costStats: (params: DashboardParams) => api.get<CostStats>('/dashboard/cost-stats', params as Record<string, string>),
  riskStats: (params: DashboardParams) => api.get<RiskStats>('/dashboard/risk-stats', params as Record<string, string>),
  systemHealth: (params: DashboardParams) =>
    api.get<SystemHealth>('/dashboard/system-health', params as Record<string, string>),
};
