import { useState } from 'react';
import { Alert, Button, Card, Empty, Input, Space, Table, Tag, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { observabilityApi } from '@/api/observability';
import { formatDateTime, formatDurationMs } from '@/utils/format';

// TraceViewerPage:轻量版 Trace 查看（设计稿第四节第 10 页）。
// 展示系统内保存的关键 span metadata，并提供跳转外部 Tempo/Jaeger 的入口。
export function TraceViewerPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const [traceId, setTraceId] = useState(searchParams.get('trace_id') ?? '');
  const [queryTraceId, setQueryTraceId] = useState(searchParams.get('trace_id') ?? '');

  const { data, isFetching, isError } = useQuery({
    queryKey: ['trace', queryTraceId],
    queryFn: () => observabilityApi.trace(queryTraceId),
    enabled: !!queryTraceId,
  });

  return (
    <Card title="Trace Viewer">
      <Space style={{ marginBottom: 16 }}>
        <Input
          placeholder="输入 trace_id"
          value={traceId}
          onChange={(e) => setTraceId(e.target.value)}
          style={{ width: 320 }}
          onPressEnter={() => setQueryTraceId(traceId.trim())}
        />
        <Button type="primary" onClick={() => setQueryTraceId(traceId.trim())}>
          查询
        </Button>
      </Space>

      {!queryTraceId && <Empty description="输入 trace_id 查询该次任务执行的 span 概览" />}
      {isError && <Alert type="error" message="查询失败或该 trace 不存在" showIcon />}

      {data && (
        <Space direction="vertical" style={{ width: '100%' }}>
          <Space>
            <Typography.Text>
              task_id：<Typography.Link onClick={() => navigate(`/tasks/${data.task_id}`)}>{data.task_id}</Typography.Link>
            </Typography.Text>
            <Typography.Text>总耗时：{formatDurationMs(data.total_duration_ms)}</Typography.Text>
            {data.external_url && (
              <a href={data.external_url} target="_blank" rel="noreferrer">
                在外部 Trace 系统中打开 →
              </a>
            )}
          </Space>
          <Table
            rowKey={(row) => `${row.name}-${row.start}`}
            loading={isFetching}
            dataSource={data.spans}
            columns={[
              { title: 'span', dataIndex: 'name' },
              { title: 'type', dataIndex: 'type', render: (v: string) => <Tag>{v}</Tag> },
              { title: 'start', dataIndex: 'start', render: (v: string) => formatDateTime(v) },
              { title: 'duration', dataIndex: 'duration_ms', render: (v: number) => formatDurationMs(v) },
              { title: 'status', dataIndex: 'status', render: (v: string) => <Tag color={v === 'error' ? 'red' : 'green'}>{v}</Tag> },
            ]}
          />
        </Space>
      )}
    </Card>
  );
}
