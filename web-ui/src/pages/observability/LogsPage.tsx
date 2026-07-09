import { useState } from 'react';
import { Card, Input, Select, Space, Tag } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { observabilityApi } from '@/api/observability';
import type { ListLogsParams, LogItem } from '@/types/observability';
import { PageTable } from '@/components/table/PageTable';
import { formatDateTime } from '@/utils/format';

// LogsPage 展示由 AgentEvent 映射的轻量日志，支持按任务、trace、级别和事件类型过滤。
export function LogsPage() {
  const [filters, setFilters] = useState<ListLogsParams>({ page: 1, page_size: 20 });
  const { data, isFetching } = useQuery({ queryKey: ['logs', filters], queryFn: () => observabilityApi.logs(filters) });

  return (
    <Card title="Logs">
      <Space wrap style={{ marginBottom: 12 }}>
        <Input.Search
          placeholder="task_id"
          style={{ width: 220 }}
          onSearch={(taskId) => setFilters((prev) => ({ ...prev, task_id: taskId || undefined, page: 1 }))}
        />
        <Input.Search
          placeholder="trace_id"
          style={{ width: 220 }}
          onSearch={(traceId) => setFilters((prev) => ({ ...prev, trace_id: traceId || undefined, page: 1 }))}
        />
        <Select
          allowClear
          placeholder="level"
          style={{ width: 140 }}
          options={['info', 'warn', 'error'].map((level) => ({ value: level, label: level }))}
          onChange={(level) => setFilters((prev) => ({ ...prev, level, page: 1 }))}
        />
        <Input.Search
          placeholder="event_type"
          style={{ width: 180 }}
          onSearch={(eventType) => setFilters((prev) => ({ ...prev, event_type: eventType || undefined, page: 1 }))}
        />
      </Space>
      <PageTable<LogItem>
        rowKey="timestamp"
        data={data}
        loading={isFetching}
        page={filters.page ?? 1}
        pageSize={filters.page_size ?? 20}
        onPageChange={(page, pageSize) => setFilters((prev) => ({ ...prev, page, page_size: pageSize }))}
        columns={[
          { title: 'timestamp', dataIndex: 'timestamp', render: (v: string) => formatDateTime(v) },
          {
            title: 'level',
            dataIndex: 'level',
            render: (level: string) => <Tag color={level === 'error' ? 'red' : level === 'warn' ? 'orange' : 'blue'}>{level}</Tag>,
          },
          { title: 'service', dataIndex: 'service' },
          { title: 'message', dataIndex: 'message', ellipsis: true },
          { title: 'task_id', dataIndex: 'task_id', ellipsis: true },
          { title: 'trace_id', dataIndex: 'trace_id', ellipsis: true },
          { title: 'error_code', dataIndex: 'error_code', render: (v?: string) => v ?? '-' },
        ]}
      />
    </Card>
  );
}
