import { useState } from 'react';
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { usersApi } from '@/api/users';
import type { AdminUser, InviteUserRequest, UpdateUserRequest } from '@/types/admin';
import { formatDateTime } from '@/utils/format';
import { usePermission } from '@/hooks/usePermission';

const ROLE_OPTIONS = ['user', 'approver', 'auditor', 'tenant_admin', 'platform_admin'];

// UsersPage 实现用户与角色管理页，按角色显示邀请和编辑入口。
export function UsersPage() {
  const perm = usePermission();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<AdminUser | null>(null);
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm<InviteUserRequest & UpdateUserRequest & { tenant_id?: string }>();
  const { data, isLoading } = useQuery({ queryKey: ['users'], queryFn: usersApi.list });

  const saveMutation = useMutation({
    mutationFn: (values: InviteUserRequest & UpdateUserRequest & { tenant_id?: string }) =>
      editing
        ? usersApi.update(editing.user_id, { role: values.role, status: values.status, name: values.name }, editing.tenant_id)
        : usersApi.invite(values),
    onSuccess: () => {
      message.success('用户已保存');
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });

  const openInvite = () => {
    setEditing(null);
    form.setFieldsValue({ role: 'user', status: 'active' });
    setOpen(true);
  };

  const openEdit = (user: AdminUser) => {
    setEditing(user);
    form.setFieldsValue({ name: user.name, role: user.role, status: user.status, tenant_id: user.tenant_id, email: user.email });
    setOpen(true);
  };

  return (
    <Card
      title="Users & Roles"
      extra={
        perm.canWriteUsers && (
          <Button type="primary" onClick={openInvite}>
            邀请用户
          </Button>
        )
      }
    >
      <Table<AdminUser>
        rowKey="user_id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        columns={[
          { title: 'user_id', dataIndex: 'user_id' },
          { title: 'email', dataIndex: 'email' },
          { title: 'name', dataIndex: 'name', render: (v?: string) => v ?? '-' },
          { title: 'tenant_id', dataIndex: 'tenant_id' },
          { title: 'role', dataIndex: 'role', render: (role: string) => <Tag color="blue">{role}</Tag> },
          { title: 'status', dataIndex: 'status', render: (status: string) => <Tag color={status === 'active' ? 'green' : 'default'}>{status}</Tag> },
          { title: 'last_login_at', dataIndex: 'last_login_at', render: (v?: string) => (v ? formatDateTime(v) : '-') },
          { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
          {
            title: 'Actions',
            render: (_, row) =>
              perm.canWriteUsers ? (
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
        title={editing ? '编辑用户' : '邀请用户'}
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => form.validateFields().then((values) => saveMutation.mutate(values))}
        confirmLoading={saveMutation.isPending}
      >
        <Form form={form} layout="vertical">
          {!editing && (
            <Form.Item name="email" label="email" rules={[{ required: true, type: 'email' }]}>
              <Input />
            </Form.Item>
          )}
          {!editing && perm.isPlatformAdmin && (
            <Form.Item name="tenant_id" label="tenant_id" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
          )}
          <Form.Item name="name" label="name">
            <Input />
          </Form.Item>
          <Form.Item name="role" label="role" rules={[{ required: true }]}>
            <Select options={ROLE_OPTIONS.map((role) => ({ value: role, label: role }))} />
          </Form.Item>
          {editing && (
            <Form.Item name="status" label="status">
              <Select options={['active', 'disabled'].map((status) => ({ value: status, label: status }))} />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </Card>
  );
}
