import { useState } from 'react';
import { Button, Card, Descriptions, Drawer, Form, Input, InputNumber, Space, Switch, Table, Tag, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toolsApi } from '@/api/tools';
import type { ToolListItem, UpdateToolRequest } from '@/types/tool';
import { StatusTag } from '@/components/status-tag/StatusTag';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { formatDateTime, formatDurationMs, formatPercent } from '@/utils/format';
import { usePermission } from '@/hooks/usePermission';

// ToolRegistryPage:工具注册表管理（设计稿第四节第 6 页）。auditor 只读，管理员可编辑。
export function ToolRegistryPage() {
  const perm = usePermission();
  const [selected, setSelected] = useState<string | null>(null);
  const { data, isLoading } = useQuery({ queryKey: ['tools'], queryFn: () => toolsApi.list() });

  return (
    <Card title="Tool Registry">
      <Table<ToolListItem>
        rowKey="tool_name"
        loading={isLoading}
        dataSource={data?.items ?? []}
        columns={[
          { title: 'Tool Name', dataIndex: 'tool_name' },
          { title: 'Description', dataIndex: 'description', ellipsis: true },
          { title: 'Category', dataIndex: 'category', render: (v?: string) => v ?? '-' },
          { title: 'Risk Level', dataIndex: 'default_risk_level', render: (v: string) => <StatusTag status={v} kind="risk" /> },
          { title: 'Enabled', dataIndex: 'enabled', render: (v: boolean) => <Tag color={v ? 'green' : 'default'}>{v ? '启用' : '禁用'}</Tag> },
          { title: 'Version', dataIndex: 'version', render: (v?: string) => v ?? '-' },
          { title: 'Owner', dataIndex: 'owner', render: (v?: string) => v ?? '-' },
          { title: 'Created At', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
          {
            title: 'Actions',
            render: (_, row) => (
              <Button size="small" onClick={() => setSelected(row.tool_name)}>
                {perm.canWriteToolsAdmin ? '查看/编辑' : '查看'}
              </Button>
            ),
          },
        ]}
      />
      {selected && <ToolDetailDrawer toolName={selected} canWrite={perm.canWriteToolsAdmin} onClose={() => setSelected(null)} />}
    </Card>
  );
}

function ToolDetailDrawer({ toolName, canWrite, onClose }: { toolName: string; canWrite: boolean; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [form] = Form.useForm<UpdateToolRequest>();
  const { data, isLoading } = useQuery({ queryKey: ['tool', toolName], queryFn: () => toolsApi.get(toolName) });

  const updateMutation = useMutation({
    mutationFn: (body: UpdateToolRequest) => toolsApi.update(toolName, body),
    onSuccess: () => {
      message.success('已更新');
      queryClient.invalidateQueries({ queryKey: ['tools'] });
      queryClient.invalidateQueries({ queryKey: ['tool', toolName] });
    },
  });

  return (
    <Drawer title={`Tool · ${toolName}`} open onClose={onClose} width={560} loading={isLoading}>
      {data && (
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="calls_7d">{data.calls_7d ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="error_rate_7d">{formatPercent(data.error_rate_7d)}</Descriptions.Item>
            <Descriptions.Item label="avg_duration_ms_7d">{formatDurationMs(data.avg_duration_ms_7d)}</Descriptions.Item>
            <Descriptions.Item label="has_side_effect">{data.has_side_effect ? '是' : '否'}</Descriptions.Item>
            <Descriptions.Item label="requires_idempotency_key">{data.requires_idempotency_key ? '是' : '否'}</Descriptions.Item>
          </Descriptions>
          <div>
            <b>params_schema</b>
            <JsonViewer value={data.params_schema ?? {}} />
          </div>
          <div>
            <b>result_schema</b>
            <JsonViewer value={data.result_schema ?? {}} />
          </div>
          {canWrite && (
            <Form
              form={form}
              layout="vertical"
              initialValues={{
                enabled: data.enabled,
                default_risk_level: data.default_risk_level,
                timeout_seconds: data.timeout_seconds,
                description: data.description,
                category: data.category,
              }}
              onFinish={(values) => updateMutation.mutate(values)}
            >
              <Form.Item name="enabled" label="启用工具" valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item name="default_risk_level" label="默认风险等级">
                <Input />
              </Form.Item>
              <Form.Item name="timeout_seconds" label="超时时间(秒)">
                <InputNumber style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item name="category" label="Category">
                <Input />
              </Form.Item>
              <Form.Item name="description" label="Description">
                <Input.TextArea rows={3} />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={updateMutation.isPending}>
                保存
              </Button>
            </Form>
          )}
        </Space>
      )}
    </Drawer>
  );
}
