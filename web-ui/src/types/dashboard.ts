export type DashboardRange = '1h' | '24h' | '7d';

export interface DashboardSummary {
  total_tasks: number;
  created_today: number;
  running: number;
  waiting_approval: number;
  failed: number;
  completed: number;
  stopped_by_limit: number;
  cancelled: number;
  success_rate: number;
  avg_duration_seconds: number;
}

export interface TaskStatsBucket {
  time: string;
  created: number;
  completed: number;
  failed: number;
  tokens: number;
  cost_usd: number;
}

export interface TaskStats {
  buckets: TaskStatsBucket[];
  status_distribution: { status: string; count: number }[];
}

export interface CostStats {
  tokens_today: number;
  cost_today_usd: number;
  avg_cost_per_task_usd: number;
  tool_calls_total: number;
  high_risk_tool_calls: number;
  approval_blocked: number;
}

export interface RiskStats {
  pending_approvals: number;
  rejected_tool_calls: number;
  loop_detected_tasks: number;
  token_limit_tasks: number;
  step_limit_tasks: number;
  high_risk_trend: { time: string; count: number }[];
  risk_distribution: { risk_level: string; count: number }[];
}

export interface SystemHealth {
  services: { name: string; status: 'healthy' | 'degraded' | 'down' | string; detail?: string }[];
  queue_backlog: number;
  active_workers: number;
  checked_at: string;
}
