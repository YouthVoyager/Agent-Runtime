export interface TraceSpan {
  name: string;
  type: 'LLM' | 'TOOL' | 'STEP' | 'CHECKPOINT' | string;
  start: string;
  duration_ms: number;
  status: 'ok' | 'error' | string;
}

export interface TraceDetail {
  trace_id: string;
  task_id: string;
  total_duration_ms: number;
  spans: TraceSpan[];
  external_url?: string;
}

export interface MetricsSnapshot {
  tasks: {
    created_total: number;
    completed_total: number;
    failed_total: number;
    cancelled_total: number;
    running: number;
    waiting_approval: number;
    duration_p50_s?: number;
    duration_p95_s?: number;
    duration_p99_s?: number;
  };
  llm: {
    calls_total: number;
    errors_total: number;
    latency_p95_ms?: number;
    tokens_total: number;
    cost_total_usd: number;
    model_distribution: { model: string; count: number }[];
  };
  tools: {
    calls_total: number;
    errors_total: number;
    latency_p95_ms?: number;
    risk_distribution: { risk_level: string; count: number }[];
    approval_required_total: number;
    approval_rejected_total: number;
  };
  workers: {
    active_workers?: number | null;
    workflow_backlog?: number | null;
    queue_lag?: number | null;
  };
}

export interface LogItem {
  timestamp: string;
  level: 'info' | 'warn' | 'error' | string;
  service: string;
  message: string;
  task_id?: string;
  trace_id?: string;
  tenant_id?: string;
  error_code?: string;
}

export interface ListLogsParams {
  task_id?: string;
  trace_id?: string;
  tenant_id?: string;
  level?: string;
  service?: string;
  error_code?: string;
  event_type?: string;
  created_from?: string;
  created_to?: string;
  page?: number;
  page_size?: number;
}
