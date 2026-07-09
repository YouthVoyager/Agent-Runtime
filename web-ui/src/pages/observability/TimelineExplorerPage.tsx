import { useMemo, useState } from 'react';
import { Button, Card, DatePicker, Drawer, Input, Space, Typography, message } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import dayjs from 'dayjs';
import { observabilityApi } from '@/api/observability';
import type { EventSummaryItem, ListEventsParams } from '@/types/event';
import { PageTable } from '@/components/table/PageTable';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { formatDateTime } from '@/utils/format';

// TimelineExplorerPage:跨任务查询 AgentEvent，用于排查和审计（设计稿第四节第 9 页）。
export function TimelineExplorerPage() {
  const navigate = useNavigate();
  const [filters, setFilters] = useState<ListEventsParams>({ page: 1, page_size: 20 });
  const [selected, setSelected] = useState<EventSummaryItem | null>(null);

  const { data, isFetching } = useQuery({
    queryKey: ['events', filters],
    queryFn: () => observabilityApi.events(filters),
  });

  const columns = useMemo(
    () => [
      { title: 'event_id', dataIndex: 'event_id', width: 90 },
      {
        title: 'task_id',
        dataIndex: 'task_id',
        render: (id: string) => <Typography.Link onClick={() => navigate(`/tasks/${id}`)}>{id}</Typography.Link>,
      },
      { title: 'type', dataIndex: 'type' },
      { title: 'input_summary', dataIndex: 'input_summary', ellipsis: true },
      { title: 'output_summary', dataIndex: 'output_summary', ellipsis: true },
      { title: 'trace_id', dataIndex: 'trace_id', render: (v?: string) => v ?? '-' },
      { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
      {
        title: 'Actions',
        render: (_: unknown, row: EventSummaryItem) => (
          <Space size={4}>
            <Button size="small" onClick={() => setSelected(row)}>
              展开
            </Button>
            <Button size="small" onClick={() => navigate(`/tasks/${row.task_id}?tab=timeline`)}>
              跳转任务详情
            </Button>
          </Space>
        ),
      },
    ],
    [navigate]
  );

  return (
    <Card title="Timeline Explorer">
      <Space wrap style={{ marginBottom: 12 }}>
        <Input placeholder="task_id" style={{ width: 180 }} onPressEnter={(e) => setFilters((f) => ({ ...f, task_id: (e.target as HTMLInputElement).value || undefined, page: 1 }))} />
        <Input placeholder="event type" style={{ width: 180 }} onPressEnter={(e) => setFilters((f) => ({ ...f, type: (e.target as HTMLInputElement).value || undefined, page: 1 }))} />
        <Input placeholder="tool_name" style={{ width: 160 }} onPressEnter={(e) => setFilters((f) => ({ ...f, tool_name: (e.target as HTMLInputElement).value || undefined, page: 1 }))} />
        <Input placeholder="trace_id" style={{ width: 180 }} onPressEnter={(e) => setFilters((f) => ({ ...f, trace_id: (e.target as HTMLInputElement).value || undefined, page: 1 }))} />
        <Input placeholder="error_code" style={{ width: 160 }} onPressEnter={(e) => setFilters((f) => ({ ...f, error_code: (e.target as HTMLInputElement).value || undefined, page: 1 }))} />
        <DatePicker.RangePicker
          showTime
          onChange={(range) =>
            setFilters((f) => ({
              ...f,
              created_from: range?.[0] ? dayjs(range[0]).toISOString() : undefined,
              created_to: range?.[1] ? dayjs(range[1]).toISOString() : undefined,
              page: 1,
            }))
          }
        />
        <Button
          onClick={() => {
            const text = JSON.stringify(data?.items ?? [], null, 2);
            navigator.clipboard.writeText(text);
            message.success('已复制当前页事件 JSON');
          }}
        >
          导出当前页
        </Button>
      </Space>
      <PageTable<EventSummaryItem>
        rowKey="event_id"
        columns={columns}
        data={data}
        loading={isFetching}
        page={filters.page ?? 1}
        pageSize={filters.page_size ?? 20}
        onPageChange={(page, pageSize) => setFilters((f) => ({ ...f, page, page_size: pageSize }))}
      />
      <Drawer title={`Event #${selected?.event_id}`} open={!!selected} onClose={() => setSelected(null)} width={520}>
        {selected && <JsonViewer value={selected} collapsed={false} />}
      </Drawer>
    </Card>
  );
}
