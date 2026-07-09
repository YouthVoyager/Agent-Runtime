import { useMemo, useState } from 'react';
import { Button, Card, Drawer, Image, Input, Modal, Space, Tag, Typography, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { artifactsApi, type ListArtifactsParams } from '@/api/artifacts';
import type { Artifact } from '@/types/task';
import { PageTable } from '@/components/table/PageTable';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { formatBytes, formatDateTime } from '@/utils/format';
import { usePermission } from '@/hooks/usePermission';

// ArtifactsPage:跨任务查看任务产物（设计稿第四节第 13 页），支持预览 markdown/json/图片和下载。
export function ArtifactsPage() {
  const [searchParams] = useSearchParams();
  const perm = usePermission();
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<ListArtifactsParams>({
    page: 1,
    page_size: 20,
    task_id: searchParams.get('task_id') ?? undefined,
  });
  const [previewId, setPreviewId] = useState<string | null>(searchParams.get('artifact_id'));

  const { data, isFetching } = useQuery({
    queryKey: ['artifacts', filters],
    queryFn: () => artifactsApi.list(filters),
  });

  const removeMutation = useMutation({
    mutationFn: (id: string) => artifactsApi.remove(id),
    onSuccess: () => {
      message.success('已删除');
      queryClient.invalidateQueries({ queryKey: ['artifacts'] });
    },
  });

  const columns = useMemo(
    () => [
      { title: 'name', dataIndex: 'name' },
      { title: 'type', dataIndex: 'type', render: (v: string) => <Tag>{v}</Tag> },
      { title: 'task_id', dataIndex: 'task_id' },
      { title: 'size', dataIndex: 'size_bytes', render: (v: number) => formatBytes(v) },
      { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
      {
        title: 'Actions',
        render: (_: unknown, row: Artifact) => (
          <Space size={4}>
            <Button size="small" onClick={() => setPreviewId(row.artifact_id)}>
              预览
            </Button>
            <Button size="small" href={artifactsApi.downloadUrl(row.artifact_id)} target="_blank">
              下载
            </Button>
            <Button
              size="small"
              onClick={() => {
                navigator.clipboard.writeText(`${window.location.origin}${artifactsApi.downloadUrl(row.artifact_id)}`);
                message.success('已复制链接');
              }}
            >
              复制链接
            </Button>
            {(perm.isTenantAdmin || perm.isPlatformAdmin) && (
              <Button
                size="small"
                danger
                onClick={() =>
                  Modal.confirm({
                    title: '确认删除该 artifact？',
                    onOk: () => removeMutation.mutateAsync(row.artifact_id),
                  })
                }
              >
                删除
              </Button>
            )}
          </Space>
        ),
      },
    ],
    [perm.isPlatformAdmin, perm.isTenantAdmin, removeMutation]
  );

  return (
    <Card title="Artifacts">
      <Space wrap style={{ marginBottom: 12 }}>
        <Input
          placeholder="task_id"
          defaultValue={filters.task_id}
          style={{ width: 200 }}
          onPressEnter={(e) => setFilters((f) => ({ ...f, task_id: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
        />
        <Input
          placeholder="type"
          style={{ width: 160 }}
          onPressEnter={(e) => setFilters((f) => ({ ...f, type: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
        />
        <Input
          placeholder="name"
          style={{ width: 160 }}
          onPressEnter={(e) => setFilters((f) => ({ ...f, name: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
        />
      </Space>
      <PageTable<Artifact>
        rowKey="artifact_id"
        columns={columns}
        data={data}
        loading={isFetching}
        page={filters.page ?? 1}
        pageSize={filters.page_size ?? 20}
        onPageChange={(page, pageSize) => setFilters((f) => ({ ...f, page, page_size: pageSize }))}
      />
      {previewId && <ArtifactPreviewDrawer artifactId={previewId} onClose={() => setPreviewId(null)} />}
    </Card>
  );
}

function ArtifactPreviewDrawer({ artifactId, onClose }: { artifactId: string; onClose: () => void }) {
  const { data, isLoading } = useQuery({ queryKey: ['artifact', artifactId], queryFn: () => artifactsApi.get(artifactId) });

  const renderContent = () => {
    if (!data) return null;
    if (data.content === null || data.content === undefined) {
      return (
        <Space direction="vertical">
          <Typography.Text type="secondary">产物过大，未内联返回，请下载查看。</Typography.Text>
          <Button href={data.download_url ?? artifactsApi.downloadUrl(artifactId)} target="_blank">
            下载
          </Button>
        </Space>
      );
    }
    if (data.type === 'image' || /\.(png|jpe?g|gif|svg)$/i.test(data.name)) {
      return <Image src={typeof data.content === 'string' ? data.content : artifactsApi.downloadUrl(artifactId)} width="100%" />;
    }
    if (data.type === 'markdown' || data.name.endsWith('.md')) {
      return (
        <Typography.Paragraph>
          <pre style={{ whiteSpace: 'pre-wrap' }}>{String(data.content)}</pre>
        </Typography.Paragraph>
      );
    }
    return <JsonViewer value={data.content} collapsed={false} />;
  };

  return (
    <Drawer title={`Artifact · ${artifactId}`} open onClose={onClose} width={640} loading={isLoading}>
      {data && (
        <Space direction="vertical" style={{ width: '100%' }}>
          <Typography.Text>
            name: {data.name} · type: <Tag>{data.type}</Tag> · size: {formatBytes(data.size_bytes)}
          </Typography.Text>
          <Typography.Text type="secondary">来源 task_id: {data.task_id}，来源 event_id: {data.created_by_event_id ?? '-'}</Typography.Text>
          {data.metadata && (
            <div>
              <Typography.Text strong>metadata</Typography.Text>
              <JsonViewer value={data.metadata} />
            </div>
          )}
          {renderContent()}
        </Space>
      )}
    </Drawer>
  );
}
