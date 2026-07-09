import { useState } from 'react';
import { Alert, Button, Card, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tabs, Tag, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toolPoliciesApi } from '@/api/toolPolicies';
import type { SimulatePolicyResult, ToolPolicy } from '@/types/tool';
import { formatDateTime } from '@/utils/format';
import { usePermission } from '@/hooks/usePermission';

const ROLE_OPTIONS = ['user', 'approver', 'auditor', 'tenant_admin', 'platform_admin'];

// ToolPoliciesPage:工具策略配置 + 策略模拟器（设计稿第四节第 7 页）。
export function ToolPoliciesPage() {
  const perm = usePermission();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<ToolPolicy | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();

  const { data, isLoading } = useQuery({ queryKey: ['tool-policies'], queryFn: () => toolPoliciesApi.list({ page: 1, page_size: 100 }) });

  const saveMutation = useMutation({
    mutationFn: (values: Partial<ToolPolicy>) =>
      editing ? toolPoliciesApi.update(editing.policy_id, values) : toolPoliciesApi.create(values),
    onSuccess: () => {
      message.success('策略已保存');
      setModalOpen(false);
      queryClient.invalidateQueries({ queryKey: ['tool-policies'] });
    },
  });

  const disableMutation = useMutation({
    mutationFn: (id: string) => toolPoliciesApi.disable(id),
    onSuccess: () => {
      message.success('已禁用策略');
      queryClient.invalidateQueries({ queryKey: ['tool-policies'] });
    },
  });

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    setModalOpen(true);
  };
  const openEdit = (policy: ToolPolicy) => {
    setEditing(policy);
    form.setFieldsValue(policy);
    setModalOpen(true);
  };
  const copyPolicy = (policy: ToolPolicy) => {
    setEditing(null);
    form.setFieldsValue({ ...policy, policy_id: undefined });
    setModalOpen(true);
  };

  return (
    <Card
      title="Tool Policies"
      extra={
        perm.canWriteToolsAdmin && (
          <Button type="primary" onClick={openCreate}>
            新建策略
          </Button>
        )
      }
    >
      <Tabs
        items={[
          {
            key: 'policies',
            label: '策略列表',
            children: (
              <Table<ToolPolicy>
                rowKey="policy_id"
                loading={isLoading}
                dataSource={data?.items ?? []}
                columns={[
                  { title: 'tenant_id', dataIndex: 'tenant_id' },
                  { title: 'tool_name', dataIndex: 'tool_name' },
                  { title: 'role', dataIndex: 'role' },
                  { title: 'enabled', dataIndex: 'enabled', render: (v: boolean) => <Tag color={v ? 'green' : 'default'}>{v ? '启用' : '禁用'}</Tag> },
                  { title: 'approval_required', dataIndex: 'approval_required', render: (v: boolean) => (v ? '是' : '否') },
                  { title: 'max_calls_per_day', dataIndex: 'max_calls_per_day', render: (v?: number) => v ?? '-' },
                  { title: 'risk_level_override', dataIndex: 'risk_level_override', render: (v?: string) => v ?? '-' },
                  { title: 'updated_at', dataIndex: 'updated_at', render: (v?: string) => (v ? formatDateTime(v) : '-') },
                  {
                    title: 'Actions',
                    render: (_, row) =>
                      perm.canWriteToolsAdmin ? (
                        <Space size={4}>
                          <Button size="small" onClick={() => openEdit(row)}>
                            编辑
                          </Button>
                          <Button size="small" onClick={() => copyPolicy(row)}>
                            复制
                          </Button>
                          <Button size="small" danger onClick={() => disableMutation.mutate(row.policy_id)}>
                            禁用
                          </Button>
                        </Space>
                      ) : (
                        '-'
                      ),
                  },
                ]}
              />
            ),
          },
          { key: 'simulator', label: '策略模拟器', children: <PolicySimulator /> },
        ]}
      />

      <Modal
        title={editing ? '编辑策略' : '新建策略'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.validateFields().then((values) => saveMutation.mutate(values))}
        confirmLoading={saveMutation.isPending}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="tenant_id" label="tenant_id" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="tool_name" label="tool_name" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="role" label="role" rules={[{ required: true }]}>
            <Select options={ROLE_OPTIONS.map((r) => ({ value: r, label: r }))} />
          </Form.Item>
          <Form.Item name="enabled" label="enabled" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
          <Form.Item name="approval_required" label="approval_required" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="max_calls_per_day" label="max_calls_per_day">
            <InputNumber style={{ width: '100%' }} min={0} />
          </Form.Item>
          <Form.Item name="risk_level_override" label="risk_level_override">
            <Select allowClear options={['LOW', 'MEDIUM', 'HIGH', 'CRITICAL'].map((r) => ({ value: r, label: r }))} />
          </Form.Item>
          <Form.Item name="timeout_seconds" label="timeout_seconds">
            <InputNumber style={{ width: '100%' }} min={1} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}

function PolicySimulator() {
  const [form] = Form.useForm();
  const [result, setResult] = useState<SimulatePolicyResult | null>(null);
  const simulateMutation = useMutation({
    mutationFn: (values: { tenant_id?: string; user_id?: string; role: string; tool_name: string; argumentsText?: string }) => {
      let args: Record<string, unknown> = {};
      try {
        args = values.argumentsText ? JSON.parse(values.argumentsText) : {};
      } catch {
        message.error('arguments 不是合法 JSON');
        throw new Error('invalid json');
      }
      return toolPoliciesApi.simulate({
        tenant_id: values.tenant_id,
        user_id: values.user_id,
        role: values.role,
        tool_name: values.tool_name,
        arguments: args,
      });
    },
    onSuccess: setResult,
  });

  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Form form={form} layout="vertical" onFinish={(v) => simulateMutation.mutate(v)}>
        <Space>
          <Form.Item name="tenant_id" label="tenant_id">
            <Input />
          </Form.Item>
          <Form.Item name="user_id" label="user_id">
            <Input />
          </Form.Item>
          <Form.Item name="role" label="role" rules={[{ required: true }]}>
            <Select style={{ width: 160 }} options={ROLE_OPTIONS.map((r) => ({ value: r, label: r }))} />
          </Form.Item>
          <Form.Item name="tool_name" label="tool_name" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
        </Space>
        <Form.Item name="argumentsText" label="arguments (JSON)">
          <Input.TextArea rows={4} placeholder="{}" />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={simulateMutation.isPending}>
          模拟
        </Button>
      </Form>
      {result && (
        <Alert
          type={result.allowed ? 'success' : 'error'}
          showIcon
          message={result.allowed ? '允许调用' : '拒绝调用'}
          description={
            <div>
              <div>risk_level: {result.risk_level}</div>
              <div>approval_required: {result.approval_required ? '是' : '否'}</div>
              <div>matched_policy_id: {result.matched_policy_id ?? '-'}</div>
              {result.deny_reason && <div>deny_reason: {result.deny_reason}</div>}
            </div>
          }
        />
      )}
    </Space>
  );
}
