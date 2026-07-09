import { useMemo, useState } from 'react';
import {
  Button,
  Card,
  DatePicker,
  Input,
  Modal,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined, CopyOutlined, PlusOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import dayjs from 'dayjs';
import { tasksApi } from '@/api/tasks';
import type { ListTasksParams, TaskStatus, TaskSummary } from '@/types/task';
import { StatusTag } from '@/components/status-tag/StatusTag';
import { PageTable } from '@/components/table/PageTable';
import { TimeAgo } from '@/components/time/TimeAgo';
import { formatCost, formatNumber } from '@/utils/format';
import { usePermission } from '@/hooks/usePermission';

const STATUS_OPTIONS: TaskStatus[] = [
  'QUEUED',
  'RUNNING',
  'WAITING_APPROVAL',
  'PAUSED',
  'CANCELING',
  'CANCELED',
  'SUCCEEDED',
  'FAILED',
  'RETRYABLE_FAILED',
  'STOPPED_BY_LIMIT',
];

const RESUMABLE: TaskStatus[] = ['FAILED', 'RETRYABLE_FAILED', 'PAUSED', 'STOPPED_BY_LIMIT'];
const CANCELABLE: TaskStatus[] = ['QUEUED', 'RUNNING', 'WAITING_APPROVAL', 'PAUSED'];

// TaskListPage:集中查看和筛选所有 Agent 任务(设计稿第四节第 2 页)。
// 权限:普通用户只能看到自己的任务，管理员看租户内/全部任务——范围由后端按 JWT 角色过滤，
// 前端只负责把筛选条件透传给 /api/v1/tasks。
export function TaskListPage() {
  const navigate = useNavigate();
  const perm = usePermission();
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();

  // 支持从 Dashboard 统计卡跳转时带初始筛选条件，例如 /tasks?status=RUNNING。
  const [filters, setFilters] = useState<ListTasksParams>({
    page: 1,
    page_size: 20,
    status: (searchParams.get('status') as TaskStatus | null) ?? undefined,
    waiting_approval: searchParams.get('waiting_approval') === 'true' || undefined,
    stopped_by_limit: searchParams.get('stopped_by_limit') === 'true' || undefined,
  });
  const [goalInput, setGoalInput] = useState('');
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);
  const [quickJump, setQuickJump] = useState('');

  const { data, isFetching, refetch } = useQuery({
    queryKey: ['tasks', filters],
    queryFn: () => tasksApi.list(filters),
  });

  const cancelMutation = useMutation({
    mutationFn: (taskId: string) => tasksApi.cancel(taskId),
    onSuccess: () => {
      message.success('已发起取消');
      queryClient.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
  const pauseMutation = useMutation({
    mutationFn: (taskId: string) => tasksApi.pause(taskId),
    onSuccess: () => {
      message.success('已暂停');
      queryClient.invalidateQueries({ queryKey: ['tasks'] });
    },
  });
  const resumeMutation = useMutation({
    mutationFn: (taskId: string) => tasksApi.resume(taskId),
    onSuccess: () => {
      message.success('已恢复');
      queryClient.invalidateQueries({ queryKey: ['tasks'] });
    },
  });

  const confirmCancel = (taskId: string) => {
    Modal.confirm({
      title: '确认取消任务？',
      content: '任务正在运行中，取消后系统会在安全边界停止；已经完成的工具调用不会回滚。',
      okText: '确认取消',
      okButtonProps: { danger: true },
      cancelText: '再想想',
      onOk: () => cancelMutation.mutateAsync(taskId),
    });
  };

  const confirmBatchCancel = () => {
    Modal.confirm({
      title: `确认批量取消 ${selectedRowKeys.length} 个任务？`,
      content: '任务正在运行中，取消后系统会在安全边界停止；已经完成的工具调用不会回滚。此操作不可撤销。',
      okText: '确认批量取消',
      okButtonProps: { danger: true },
      cancelText: '再想想',
      onOk: async () => {
        await Promise.allSettled(selectedRowKeys.map((id) => tasksApi.cancel(id)));
        message.success('批量取消已提交');
        setSelectedRowKeys([]);
        queryClient.invalidateQueries({ queryKey: ['tasks'] });
      },
    });
  };

  const copyId = async (taskId: string) => {
    await navigator.clipboard.writeText(taskId);
    message.success('已复制 task_id');
  };

  const columns: ColumnsType<TaskSummary> = useMemo(
    () => [
      {
        title: 'Task ID',
        dataIndex: 'task_id',
        width: 220,
        render: (id: string) => (
          <Space size={4}>
            <Typography.Link onClick={() => navigate(`/tasks/${id}`)}>{id}</Typography.Link>
            <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyId(id)} />
          </Space>
        ),
      },
      { title: 'Goal', dataIndex: 'goal', ellipsis: true },
      { title: 'Status', dataIndex: 'status', width: 140, render: (s: string) => <StatusTag status={s} /> },
      { title: 'User', dataIndex: 'user_id', width: 120, render: (v?: string) => v ?? '-' },
      { title: 'Tenant', dataIndex: 'tenant_id', width: 120, render: (v?: string) => v ?? '-' },
      {
        title: 'Used Tokens',
        width: 110,
        render: (_, row) => formatNumber(row.budget_usage?.used_tokens),
      },
      {
        title: 'Tool Calls',
        width: 100,
        render: (_, row) => formatNumber(row.budget_usage?.used_tool_calls),
      },
      {
        title: 'Cost',
        width: 100,
        render: (_, row) => formatCost(row.budget_usage?.used_cost_usd),
      },
      { title: 'Created At', dataIndex: 'created_at', width: 160, render: (v: string) => <TimeAgo value={v} /> },
      { title: 'Updated At', dataIndex: 'updated_at', width: 160, render: (v: string) => <TimeAgo value={v} /> },
      {
        title: 'Actions',
        fixed: 'right',
        width: 220,
        render: (_, row) => (
          <Space size={4} wrap>
            <Button size="small" onClick={() => navigate(`/tasks/${row.task_id}`)}>
              查看
            </Button>
            {CANCELABLE.includes(row.status) && (
              <Button size="small" danger onClick={() => confirmCancel(row.task_id)}>
                取消
              </Button>
            )}
            {row.status === 'RUNNING' && (
              <Button size="small" onClick={() => pauseMutation.mutate(row.task_id)}>
                暂停
              </Button>
            )}
            {RESUMABLE.includes(row.status) && (
              <Button size="small" type="primary" onClick={() => resumeMutation.mutate(row.task_id)}>
                恢复
              </Button>
            )}
            {row.status === 'WAITING_APPROVAL' && (
              <Button size="small" onClick={() => navigate(`/approvals?task_id=${row.task_id}`)}>
                去审批
              </Button>
            )}
          </Space>
        ),
      },
    ],
    [navigate]
  );

  return (
    <Card
      title="Task List"
      extra={
        perm.canCreateTask ? (
          <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/tasks/create')}>
            创建任务
          </Button>
        ) : undefined
      }
    >
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Space wrap>
          <Input.Search
            placeholder="按 task_id 精确跳转"
            style={{ width: 240 }}
            value={quickJump}
            onChange={(e) => setQuickJump(e.target.value)}
            onSearch={(v) => v.trim() && navigate(`/tasks/${v.trim()}`)}
          />
          <Input.Search
            placeholder="goal 关键词搜索"
            style={{ width: 240 }}
            value={goalInput}
            onChange={(e) => setGoalInput(e.target.value)}
            onSearch={(v) => setFilters((f) => ({ ...f, goal: v || undefined, page: 1 }))}
          />
          <Select
            allowClear
            placeholder="Status"
            style={{ width: 180 }}
            value={filters.status}
            options={STATUS_OPTIONS.map((s) => ({ value: s, label: s }))}
            onChange={(v) => setFilters((f) => ({ ...f, status: v, page: 1 }))}
          />
          {(perm.isTenantAdmin || perm.isPlatformAdmin) && (
            <Input
              placeholder="按 user_id 筛选"
              style={{ width: 160 }}
              onPressEnter={(e) => setFilters((f) => ({ ...f, user_id: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
            />
          )}
          {perm.isPlatformAdmin && (
            <Input
              placeholder="按 tenant_id 筛选"
              style={{ width: 160 }}
              onPressEnter={(e) => setFilters((f) => ({ ...f, tenant_id: (e.target as HTMLInputElement).value || undefined, page: 1 }))}
            />
          )}
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
          <Space size={4}>
            <span>等待审批</span>
            <Switch onChange={(v) => setFilters((f) => ({ ...f, waiting_approval: v || undefined, page: 1 }))} />
          </Space>
          <Space size={4}>
            <span>超限停止</span>
            <Switch onChange={(v) => setFilters((f) => ({ ...f, stopped_by_limit: v || undefined, page: 1 }))} />
          </Space>
          <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
            刷新
          </Button>
        </Space>

        {perm.canBatchCancel && selectedRowKeys.length > 0 && (
          <Tag color="orange">
            已选 {selectedRowKeys.length} 项
            <Button size="small" danger type="link" onClick={confirmBatchCancel}>
              批量取消
            </Button>
          </Tag>
        )}

        <PageTable<TaskSummary>
          rowKey="task_id"
          columns={columns}
          data={data}
          loading={isFetching}
          page={filters.page ?? 1}
          pageSize={filters.page_size ?? 20}
          onPageChange={(page, pageSize) => setFilters((f) => ({ ...f, page, page_size: pageSize }))}
          scroll={{ x: 1400 }}
          rowSelection={
            perm.canBatchCancel
              ? {
                  selectedRowKeys,
                  onChange: (keys) => setSelectedRowKeys(keys as string[]),
                  getCheckboxProps: (row) => ({ disabled: !CANCELABLE.includes(row.status) }),
                }
              : undefined
          }
        />
      </Space>
    </Card>
  );
}
