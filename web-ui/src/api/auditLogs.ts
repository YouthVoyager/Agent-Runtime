import { api } from './client';
import type { Page } from '@/types/common';
import type { AuditLogItem, ListAuditLogsParams } from '@/types/admin';

export const auditLogsApi = {
  list: (params: ListAuditLogsParams) =>
    api.get<Page<AuditLogItem>>('/audit-logs', params as Record<string, string>),
};
