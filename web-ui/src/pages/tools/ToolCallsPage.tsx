import { useMemo, useState } from 'react';
import { Button, Card, Descriptions, Drawer, Input, Select, Space, Table, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { toolCallsApi } from '@/api/toolCalls';
import type { ListToolCallsParams, ToolCallCrossTaskItem } from '@/types/tool';
import { StatusTag } from '@/components/status-tag/StatusTag';
import { PageTable } from '@/components/table/PageTable';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { formatDateTime, formatDurationMs } from '@/utils/format';

// ToolCallsPage:跨任务查询工具调用记录，用于审计和排查（设计稿第四节第 8 页）。
export function ToolCallsPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const [filters, setFilters] = useState<ListToolCallsParams>({
    page: 1,
    page_size: 20,
    call_id: searchParams.get('call_id') ?? undefined,
  });
  const [detailId, setDetailId] = useState<string | null>(searchParams.get('call_id'));

  const { data, isFetching } = useQuery({
    queryKey: ['tool-calls', filters],
    queryFn: () => toolCallsApi.list(filters),
  });

  const columns = useMemo(
    () => [
      { title: 'call_id', dataIndex: 'call_id' },
      {
        title: 'task_id',
        dataIndex: 'task_id',
        render: (id: string) => <Typography.Link onClick={() => navigate(`/tasks/${id}`)}>{id}</Typography.Link>,
      },
      { title: 'tool_name', dataIndex: 'tool_name' },
      { title: 'risk_level', dataIndex: 'risk_level', render: (v: string) => <StatusTag status={v} kind="risk" /> },
      { title: 'status', dataIndex: 'status', render: (v: string) => <StatusTag status={v} kind="tool-call" /> },
      { title: 'approval_status', dataIndex: 'approval_status', render: (v: string) => <StatusTag status={v} kind="approval" /> },
      { title: 'duration', dataIndex: 'duration_ms', render: (v?: number) => formatDurationMs(v) },
      { title: 'error_code', dataIndex: 'error_code', render: (v?: string) => v ?? '-' },
      { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
      {
        title: 'Actions',
        render: (_: unknown, row: ToolCallCrossTaskItem) => (
          <Button size="small" onClick={() => setDetailId(row.call_id)}>
            详情
          </Button>
        ),
      },
    ],
    [navigate]
  );

  return (
    <Card title="Tool Calls">
      <Space wrap style={{ marginBottom: 12 }}>
        <Input
          placeholder="task_id"
          style={{ width: 180 }}
          onPressEnter={(e) => setFilters((f) => ({ ...f, task_id: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
        />
        <Input
          placeholder="call_id"
          defaultValue={filters.call_id}
          style={{ width: 180 }}
          onPressEnter={(e) => setFilters((f) => ({ ...f, call_id: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
        />
        <Input
          placeholder="tool_name"
          style={{ width: 160 }}
          onPressEnter={(e) => setFilters((f) => ({ ...f, tool_name: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
        />
        <Select
          allowClear
          placeholder="risk_level"
          style={{ width: 140 }}
          options={['LOW', 'MEDIUM', 'HIGH', 'CRITICAL'].map((v) => ({ value: v, label: v }))}
          onChange={(v) => setFilters((f) => ({ ...f, risk_level: v, page: 1 }))}
        />
        <Input
          placeholder="error_code"
          style={{ width: 160 }}
          onPressEnter={(e) => setFilters((f) => ({ ...f, error_code: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
        />
      </Space>
      <PageTable<ToolCallCrossTaskItem>
        rowKey="call_id"
        columns={columns}
        data={data}
        loading={isFetching}
        page={filters.page ?? 1}
        pageSize={filters.page_size ?? 20}
        onPageChange={(page, pageSize) => setFilters((f) => ({ ...f, page, page_size: pageSize }))}
      />
      {detailId && <ToolCallDetailDrawer callId={detailId} onClose={() => setDetailId(null)} />}
    </Card>
  );
}

function ToolCallDetailDrawer({ callId, onClose }: { callId: string; onClose: () => void }) {
  const { data, isLoading } = useQuery({ queryKey: ['tool-call', callId], queryFn: () => toolCallsApi.get(callId) });
  return (
    <Drawer title={`Tool Call · ${callId}`} open onClose={onClose} width={560} loading={isLoading}>
      {data && (
        <Space direction="vertical" style={{ width: '100%' }}>
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="task_id">{data.task_id}</Descriptions.Item>
            <Descriptions.Item label="tool_name">{data.tool_name}</Descriptions.Item>
            <Descriptions.Item label="status">
              <StatusTag status={data.status} kind="tool-call" />
            </Descriptions.Item>
            <Descriptions.Item label="approval_status">
              <StatusTag status={data.approval_status} kind="approval" />
            </Descriptions.Item>
            <Descriptions.Item label="risk_level">
              <StatusTag status={data.risk_level} kind="risk" />
            </Descriptions.Item>
            <Descriptions.Item label="arguments_hash">{data.arguments_hash ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="idempotency_key">{data.idempotency_key ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="duration">{formatDurationMs(data.duration_ms)}</Descriptions.Item>
            <Descriptions.Item label="trace_id">{data.trace_id ?? '-'}</Descriptions.Item>
            {data.error_code && (
              <Descriptions.Item label="error">
                {data.error_code}: {data.error_message}
              </Descriptions.Item>
            )}
          </Descriptions>
          <Typography.Text strong>arguments</Typography.Text>
          <JsonViewer value={data.arguments ?? {}} />
          <Typography.Text strong>result</Typography.Text>
          <JsonViewer value={data.result ?? {}} />
          {data.approvals && data.approvals.length > 0 && (
            <>
              <Typography.Text strong>approvals</Typography.Text>
              <Table
                size="small"
                rowKey="approval_id"
                pagination={false}
                dataSource={data.approvals}
                columns={[
                  { title: 'status', dataIndex: 'status' },
                  { title: 'approver_id', dataIndex: 'approver_id', render: (v?: string) => v ?? '-' },
                  { title: 'note', dataIndex: 'note', render: (v?: string) => v ?? '-' },
                  { title: 'decided_at', dataIndex: 'decided_at', render: (v?: string) => (v ? formatDateTime(v) : '-') },
                ]}
              />
            </>
          )}
        </Space>
      )}
    </Drawer>
  );
}
