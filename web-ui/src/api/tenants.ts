import { api } from './client';
import type { Page } from '@/types/common';
import type { CreateTenantRequest, Tenant, UpdateTenantRequest } from '@/types/admin';

export const tenantsApi = {
  list: () => api.get<Page<Tenant>>('/tenants'),
  create: (body: CreateTenantRequest) => api.post<Tenant>('/tenants', body),
  get: (tenantId: string) => api.get<Tenant>(`/tenants/${tenantId}`),
  update: (tenantId: string, body: UpdateTenantRequest) => api.patch<Tenant>(`/tenants/${tenantId}`, body),
};
