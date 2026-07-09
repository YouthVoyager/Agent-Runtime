import { useState } from 'react';
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { tenantsApi } from '@/api/tenants';
import type { Tenant } from '@/types/admin';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { formatCost, formatDateTime, formatNumber } from '@/utils/format';
import { usePermission } from '@/hooks/usePermission';

interface TenantFormValues {
  name: string;
  status?: 'active' | 'disabled';
  configText?: string;
}

// TenantsPage 实现租户管理页，平台管理员可创建/修改租户，租户管理员只读查看本租户。
export function TenantsPage() {
  const perm = usePermission();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Tenant | null>(null);
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm<TenantFormValues>();
  const { data, isLoading } = useQuery({ queryKey: ['tenants'], queryFn: tenantsApi.list });

  const saveMutation = useMutation({
    mutationFn: (values: TenantFormValues) => {
      const config = parseConfig(values.configText);
      return editing
        ? tenantsApi.update(editing.tenant_id, { name: values.name, status: values.status, config })
        : tenantsApi.create({ name: values.name, config });
    },
    onSuccess: () => {
      message.success('租户已保存');
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ['tenants'] });
    },
  });

  const openCreate = () => {
    setEditing(null);
    form.setFieldsValue({ name: '', status: 'active', configText: '{}' });
    setOpen(true);
  };

  const openEdit = (tenant: Tenant) => {
    setEditing(tenant);
    form.setFieldsValue({ name: tenant.name, status: tenant.status, configText: JSON.stringify(tenant.config ?? {}, null, 2) });
    setOpen(true);
  };

  return (
    <Card
      title="Tenants"
      extra={
        perm.canWriteTenants && (
          <Button type="primary" onClick={openCreate}>
            创建租户
          </Button>
        )
      }
    >
      <Table<Tenant>
        rowKey="tenant_id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        columns={[
          { title: 'tenant_id', dataIndex: 'tenant_id' },
          { title: 'name', dataIndex: 'name' },
          { title: 'status', dataIndex: 'status', render: (status: string) => <Tag color={status === 'active' ? 'green' : 'default'}>{status}</Tag> },
          { title: 'tasks_today', dataIndex: ['usage', 'tasks_today'], render: (v?: number) => formatNumber(v) },
          { title: 'tokens_today', dataIndex: ['usage', 'tokens_today'], render: (v?: number) => formatNumber(v) },
          { title: 'cost_today_usd', dataIndex: ['usage', 'cost_today_usd'], render: (v?: number) => formatCost(v) },
          { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
          {
            title: 'config',
            render: (_, row) => <JsonViewer value={row.config ?? {}} />,
          },
          {
            title: 'Actions',
            render: (_, row) =>
              perm.canWriteTenants ? (
                <Button size="small" onClick={() => openEdit(row)}>
                  编辑
                </Button>
              ) : (
                '-'
              ),
          },
        ]}
      />
      <Modal
        title={editing ? '编辑租户' : '创建租户'}
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => form.validateFields().then((values) => saveMutation.mutate(values))}
        confirmLoading={saveMutation.isPending}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="name" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          {editing && (
            <Form.Item name="status" label="status">
              <Select options={['active', 'disabled'].map((status) => ({ value: status, label: status }))} />
            </Form.Item>
          )}
          <Form.Item name="configText" label="config JSON">
            <Input.TextArea rows={8} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}

// parseConfig 将表单文本解析为租户配置对象，解析失败时抛出前端校验错误。
function parseConfig(raw?: string) {
  if (!raw?.trim()) return {};
  try {
    return JSON.parse(raw);
  } catch {
    message.error('config 不是合法 JSON');
    throw new Error('invalid config json');
  }
}
