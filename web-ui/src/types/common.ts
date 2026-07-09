// 通用响应封装类型。所有后端接口成功时返回 { data }，失败时返回 { error }。
// 见 docs/admin-console-api-contract.md 第 0 节。
export interface ApiError {
  code: string;
  message: string;
  request_id?: string;
}

export interface ApiErrorEnvelope {
  error: ApiError;
}

export interface ApiDataEnvelope<T> {
  data: T;
}

// 分页列表统一结构。
export interface Page<T> {
  items: T[];
  page: number;
  page_size: number;
  has_more: boolean;
  next_page?: number | null;
}

// 角色取值（小写），历史遗留 member 等价于 user。
export type Role = 'user' | 'member' | 'approver' | 'auditor' | 'tenant_admin' | 'platform_admin';

export type RiskLevel = 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL' | string;

export interface ListQuery {
  page?: number;
  page_size?: number;
}
