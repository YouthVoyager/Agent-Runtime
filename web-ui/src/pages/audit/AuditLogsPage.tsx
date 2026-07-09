import { useState } from 'react';
import { Card, Input, Space } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { auditLogsApi } from '@/api/auditLogs';
import type { AuditLogItem, ListAuditLogsParams } from '@/types/admin';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { PageTable } from '@/components/table/PageTable';
import { formatDateTime } from '@/utils/format';

// AuditLogsPage 展示敏感写操作审计日志，支持按 actor/action/resource 过滤。
export function AuditLogsPage() {
  const [filters, setFilters] = useState<ListAuditLogsParams>({ page: 1, page_size: 20 });
  const { data, isFetching } = useQuery({ queryKey: ['audit-logs', filters], queryFn: () => auditLogsApi.list(filters) });

  return (
    <Card title="Audit Logs">
      <Space wrap style={{ marginBottom: 12 }}>
        <Input.Search
          placeholder="actor_id"
          style={{ width: 180 }}
          onSearch={(actorId) => setFilters((prev) => ({ ...prev, actor_id: actorId || undefined, page: 1 }))}
        />
        <Input.Search
          placeholder="action"
          style={{ width: 220 }}
          onSearch={(action) => setFilters((prev) => ({ ...prev, action: action || undefined, page: 1 }))}
        />
        <Input.Search
          placeholder="resource_type"
          style={{ width: 180 }}
          onSearch={(resourceType) => setFilters((prev) => ({ ...prev, resource_type: resourceType || undefined, page: 1 }))}
        />
        <Input.Search
          placeholder="resource_id"
          style={{ width: 220 }}
          onSearch={(resourceId) => setFilters((prev) => ({ ...prev, resource_id: resourceId || undefined, page: 1 }))}
        />
      </Space>
      <PageTable<AuditLogItem>
        rowKey="audit_id"
        data={data}
        loading={isFetching}
        page={filters.page ?? 1}
        pageSize={filters.page_size ?? 20}
        onPageChange={(page, pageSize) => setFilters((prev) => ({ ...prev, page, page_size: pageSize }))}
        columns={[
          { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
          { title: 'actor_id', dataIndex: 'actor_id' },
          { title: 'tenant_id', dataIndex: 'tenant_id', render: (v?: string) => v ?? '-' },
          { title: 'action', dataIndex: 'action' },
          { title: 'resource_type', dataIndex: 'resource_type' },
          { title: 'resource_id', dataIndex: 'resource_id', render: (v?: string) => v ?? '-' },
          { title: 'before', dataIndex: 'before', render: (v?: unknown) => <JsonViewer value={v ?? null} /> },
          { title: 'after', dataIndex: 'after', render: (v?: unknown) => <JsonViewer value={v ?? null} /> },
          { title: 'ip', dataIndex: 'ip', render: (v?: string) => v ?? '-' },
        ]}
      />
    </Card>
  );
}
