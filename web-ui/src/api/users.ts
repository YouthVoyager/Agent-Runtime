import { api } from './client';
import type { Page } from '@/types/common';
import type { AdminUser, InviteUserRequest, UpdateUserRequest } from '@/types/admin';

export const usersApi = {
  list: () => api.get<Page<AdminUser>>('/users'),
  invite: (body: InviteUserRequest) => api.post<AdminUser>('/users', body),
  update: (userId: string, body: UpdateUserRequest, tenantId?: string) =>
    api.patch<AdminUser>(`/users/${userId}${tenantId ? `?tenant_id=${encodeURIComponent(tenantId)}` : ''}`, body),
};
