// 租户 / 用户 / 设置 / 审计日志类型。

export interface TenantConfig {
  max_tasks_per_day?: number;
  max_tokens_per_day?: number;
  max_cost_per_day?: number;
  default_max_steps?: number;
  default_tool_policy?: string;
  approval_policy?: string;
  data_retention_days?: number;
}

export interface TenantUsage {
  tasks_today: number;
  tokens_today: number;
  cost_today_usd: number;
}

export interface Tenant {
  tenant_id: string;
  name: string;
  status: 'active' | 'disabled';
  config: TenantConfig;
  usage?: TenantUsage;
  created_at: string;
}

export interface CreateTenantRequest {
  name: string;
  config?: TenantConfig;
}

export interface UpdateTenantRequest {
  name?: string;
  status?: 'active' | 'disabled';
  config?: TenantConfig;
}

export type UserStatus = 'active' | 'disabled';

export interface AdminUser {
  user_id: string;
  email: string;
  name?: string;
  tenant_id: string;
  role: string;
  status: UserStatus;
  last_login_at?: string;
  created_at: string;
}

export interface InviteUserRequest {
  email: string;
  name?: string;
  role: string;
  tenant_id?: string;
}

export interface UpdateUserRequest {
  role?: string;
  status?: UserStatus;
  name?: string;
}

export interface SettingsGroups {
  runtime: Record<string, unknown>;
  llm: Record<string, unknown>;
  tool: Record<string, unknown>;
  security: Record<string, unknown>;
  retention: Record<string, unknown>;
  observability: Record<string, unknown>;
}

export type SettingsSection = keyof SettingsGroups;

export interface AuditLogItem {
  audit_id: string;
  actor_id: string;
  tenant_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  before?: unknown;
  after?: unknown;
  ip?: string;
  user_agent?: string;
  created_at: string;
}

export interface ListAuditLogsParams {
  actor_id?: string;
  action?: string;
  resource_type?: string;
  resource_id?: string;
  tenant_id?: string;
  created_from?: string;
  created_to?: string;
  page?: number;
  page_size?: number;
}
