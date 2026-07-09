import { message } from 'antd';
import { getStoredToken, useAuthStore } from '@/store/authStore';
import type { ApiError } from '@/types/common';

// ApiClientError 统一封装后端 {"error": {code, message, request_id}} 结构，
// 方便调用方（react-query）用 error.code 做逻辑判断，同时保留人类可读 message。
export class ApiClientError extends Error {
  code: string;
  requestId?: string;

  constructor(err: ApiError) {
    super(err.message);
    this.code = err.code;
    this.requestId = err.request_id;
  }
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
  body?: unknown;
  query?: Record<string, string | number | boolean | undefined | null>;
  // 401 之外的错误默认会弹全局 antd message 提示；某些页面希望自己处理时可关闭。
  silent?: boolean;
  signal?: AbortSignal;
}

function buildQueryString(query?: RequestOptions['query']): string {
  if (!query) return '';
  const params = new URLSearchParams();
  Object.entries(query).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return;
    params.set(key, String(value));
  });
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

// redirectToLogin 在 401 时清空本地 token 并跳转登录页，保留当前路径便于登录后返回。
function redirectToLogin() {
  useAuthStore.getState().logout();
  const redirect = encodeURIComponent(window.location.pathname + window.location.search);
  if (!window.location.pathname.startsWith('/login')) {
    window.location.assign(`/login?redirect=${redirect}`);
  }
}

// request 是所有 API 调用的统一入口：自动带 Authorization、统一解包 { data }、
// 统一处理 401 跳转登录、统一把 { error } 转成 ApiClientError 并弹出全局提示。
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, query, silent, signal } = options;
  const token = getStoredToken();
  const headers: Record<string, string> = {};
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }

  let response: Response;
  try {
    response = await fetch(`/api/v1${path}${buildQueryString(query)}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
    });
  } catch (err) {
    const networkErr: ApiError = { code: 'NETWORK_ERROR', message: '网络请求失败，请检查连接' };
    if (!silent) message.error(`${networkErr.message} (${networkErr.code})`);
    throw new ApiClientError(networkErr);
  }

  if (response.status === 401) {
    redirectToLogin();
    const err: ApiError = { code: 'UNAUTHORIZED', message: '登录已过期，请重新登录' };
    throw new ApiClientError(err);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  let json: unknown;
  try {
    json = await response.json();
  } catch {
    json = null;
  }

  if (!response.ok) {
    const errBody = (json as { error?: ApiError } | null)?.error;
    const err: ApiError = errBody ?? { code: `HTTP_${response.status}`, message: response.statusText || '请求失败' };
    if (!silent) {
      message.error(`${err.message} (${err.request_id ?? err.code})`);
    }
    throw new ApiClientError(err);
  }

  return (json as { data: T }).data;
}

export const api = {
  get: <T>(path: string, query?: RequestOptions['query'], options?: RequestOptions) =>
    request<T>(path, { ...options, method: 'GET', query }),
  post: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>(path, { ...options, method: 'POST', body }),
  patch: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>(path, { ...options, method: 'PATCH', body }),
  put: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>(path, { ...options, method: 'PUT', body }),
  delete: <T>(path: string, options?: RequestOptions) => request<T>(path, { ...options, method: 'DELETE' }),
};
