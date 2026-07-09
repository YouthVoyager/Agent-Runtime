import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Badge,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Modal,
  Progress,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { tasksApi } from '@/api/tasks';
import { StatusTag } from '@/components/status-tag/StatusTag';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { TimeAgo } from '@/components/time/TimeAgo';
import { TimelineEventCard } from '@/components/timeline/TimelineEventCard';
import { useTaskEvents } from '@/hooks/useTaskEvents';
import { usePermission } from '@/hooks/usePermission';
import { formatCost, formatDateTime, formatNumber, safeJsonStringify } from '@/utils/format';
import type { Checkpoint, TaskDetail, ToolCall } from '@/types/task';

const RESUMABLE = ['FAILED', 'RETRYABLE_FAILED', 'PAUSED', 'STOPPED_BY_LIMIT'];
const CANCELABLE = ['QUEUED', 'RUNNING', 'WAITING_APPROVAL', 'PAUSED'];

function copyText(text: string, label: string) {
  navigator.clipboard.writeText(text).then(() => message.success(`已复制${label}`));
}

// TaskDetailPage 是管理台价值最高的页面(设计稿优先级 P0 第一位):
// 基本信息 + 状态控制 + Budget + 实时 Timeline + Current State + Tool Calls + Checkpoints + Artifacts + Error。
export function TaskDetailPage() {
  const { taskId = '' } = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const perm = usePermission();
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState(searchParams.get('tab') ?? 'timeline');

  const { data: task, isLoading } = useQuery({
    queryKey: ['task', taskId],
    queryFn: () => tasksApi.get(taskId),
    enabled: !!taskId,
    refetchInterval: 10_000,
  });

  const invalidateTask = () => {
    queryClient.invalidateQueries({ queryKey: ['task', taskId] });
    queryClient.invalidateQueries({ queryKey: ['tasks'] });
  };

  const cancelMutation = useMutation({
    mutationFn: () => tasksApi.cancel(taskId),
    onSuccess: () => {
      message.success('已发起取消');
      invalidateTask();
    },
  });
  const pauseMutation = useMutation({
    mutationFn: () => tasksApi.pause(taskId),
    onSuccess: () => {
      message.success('已暂停');
      invalidateTask();
    },
  });
  const resumeMutation = useMutation({
    mutationFn: () => tasksApi.resume(taskId),
    onSuccess: () => {
      message.success('已恢复');
      invalidateTask();
    },
  });

  const confirmCancel = () => {
    Modal.confirm({
      title: '确认取消任务？',
      content: '任务正在运行中，取消后系统会在安全边界停止；已经完成的工具调用不会回滚。',
      okText: '确认取消',
      okButtonProps: { danger: true },
      cancelText: '再想想',
      onOk: () => cancelMutation.mutateAsync(),
    });
  };

  const confirmResume = async () => {
    let latestInfo = '';
    try {
      const cps = await tasksApi.checkpoints(taskId, { limit: 1 });
      const latest = cps.items[0];
      if (latest) {
        latestInfo = `恢复点：${latest.checkpoint_id}（${formatDateTime(latest.created_at)}，state_version=${latest.state_version}）。`;
      }
    } catch {
      // 拿不到最近 checkpoint 不影响确认弹窗展示，仅缺少细节。
    }
    Modal.confirm({
      title: '确认恢复任务？',
      content: (
        <div>
          <p>将从最近 checkpoint 恢复。{latestInfo}</p>
          <p>恢复后可能会继续执行后续工具调用；已完成的幂等工具不会重复执行。</p>
        </div>
      ),
      okText: '确认恢复',
      cancelText: '再想想',
      onOk: () => resumeMutation.mutateAsync(),
    });
  };

  if (!task && !isLoading) {
    return <Empty description="任务不存在或无权访问" />;
  }

  return (
    <Card
      loading={isLoading}
      title={
        <Space>
          <span>Task Detail</span>
          {task && <StatusTag status={task.status} />}
        </Space>
      }
      extra={
        task && (
          <Space>
            {CANCELABLE.includes(task.status) && (
              <Button danger onClick={confirmCancel}>
                Cancel
              </Button>
            )}
            {task.status === 'RUNNING' && <Button onClick={() => pauseMutation.mutate()}>Pause</Button>}
            {RESUMABLE.includes(task.status) && (
              <Button type="primary" onClick={confirmResume}>
                Resume
              </Button>
            )}
            {task.status === 'WAITING_APPROVAL' && (
              <Button onClick={() => navigate(`/approvals?task_id=${taskId}`)}>View Approval</Button>
            )}
          </Space>
        )
      }
    >
      {task && (
        <>
          <BasicInfoSection task={task} onOpenTrace={() => navigate(`/observability/trace?trace_id=${task.trace_id ?? ''}`)} />
          {task.last_error_code && (
            <Alert
              style={{ margin: '12px 0' }}
              type="error"
              showIcon
              message={`Error: ${task.last_error_code}`}
              description={task.last_error_message}
            />
          )}
          <BudgetSection task={task} />
          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              { key: 'timeline', label: 'Timeline', children: <TimelineSection taskId={taskId} onJumpTab={setActiveTab} /> },
              { key: 'state', label: 'Current State', children: <StateSection taskId={taskId} /> },
              { key: 'toolcalls', label: 'Tool Calls', children: <ToolCallsSection taskId={taskId} /> },
              {
                key: 'checkpoints',
                label: 'Checkpoints',
                children: <CheckpointsSection taskId={taskId} onResumed={invalidateTask} canWrite={perm.canWrite} />,
              },
              { key: 'artifacts', label: 'Artifacts', children: <ArtifactsSection taskId={taskId} /> },
            ]}
          />
        </>
      )}
    </Card>
  );
}

function BasicInfoSection({ task, onOpenTrace }: { task: TaskDetail; onOpenTrace: () => void }) {
  return (
    <Descriptions bordered size="small" column={3} style={{ marginBottom: 12 }}>
      <Descriptions.Item label="task_id">
        <Space size={4}>
          {task.task_id}
          <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyText(task.task_id, ' task_id')} />
        </Space>
      </Descriptions.Item>
      <Descriptions.Item label="status">
        <StatusTag status={task.status} />
      </Descriptions.Item>
      <Descriptions.Item label="current_step">{task.current_step}</Descriptions.Item>
      <Descriptions.Item label="goal" span={3}>
        {task.goal}
      </Descriptions.Item>
      <Descriptions.Item label="trace_id">
        <Space size={4}>
          {task.trace_id ?? '-'}
          {task.trace_id && (
            <>
              <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyText(task.trace_id!, ' trace_id')} />
              <Typography.Link onClick={onOpenTrace}>打开 Trace Viewer</Typography.Link>
            </>
          )}
        </Space>
      </Descriptions.Item>
      <Descriptions.Item label="created_at">
        <TimeAgo value={task.created_at} />
      </Descriptions.Item>
      <Descriptions.Item label="updated_at">
        <TimeAgo value={task.updated_at} />
      </Descriptions.Item>
      <Descriptions.Item label="原始 JSON" span={3}>
        <JsonViewer value={task} />
      </Descriptions.Item>
    </Descriptions>
  );
}

function budgetPercent(used: number, max: number) {
  if (!max) return 0;
  return Math.min(100, Math.round((used / max) * 100));
}

function BudgetSection({ task }: { task: { budget: { max_steps: number; max_tokens: number; max_tool_calls: number; max_cost_usd: number }; budget_usage: { used_steps: number; used_tokens: number; used_tool_calls: number; used_cost_usd: number } } }) {
  const items: { label: string; used: number; max: number; format: (v: number) => string }[] = [
    { label: 'Steps', used: task.budget_usage.used_steps, max: task.budget.max_steps, format: formatNumber },
    { label: 'Tokens', used: task.budget_usage.used_tokens, max: task.budget.max_tokens, format: formatNumber },
    { label: 'Tool Calls', used: task.budget_usage.used_tool_calls, max: task.budget.max_tool_calls, format: formatNumber },
    { label: 'Cost USD', used: task.budget_usage.used_cost_usd, max: task.budget.max_cost_usd, format: formatCost },
  ];
  return (
    <Card size="small" title="Budget 使用情况" style={{ marginBottom: 12 }}>
      <Space direction="vertical" style={{ width: '100%' }}>
        {items.map((item) => {
          const percent = budgetPercent(item.used, item.max);
          const status = percent >= 100 ? 'exception' : percent >= 90 ? 'active' : 'normal';
          return (
            <div key={item.label}>
              <Space>
                <Typography.Text>{item.label}</Typography.Text>
                <Typography.Text type="secondary">
                  {item.format(item.used)} / {item.format(item.max)}
                </Typography.Text>
                {percent >= 90 && <Tag color={percent >= 100 ? 'red' : 'orange'}>接近上限</Tag>}
              </Space>
              <Progress percent={percent} status={status} size="small" />
            </div>
          );
        })}
      </Space>
    </Card>
  );
}

function TimelineSection({ taskId, onJumpTab }: { taskId: string; onJumpTab: (tab: string) => void }) {
  const { events, connected, error } = useTaskEvents(taskId);
  const [paused, setPaused] = useState(false);
  const [typeFilter, setTypeFilter] = useState<string[]>([]);
  const containerRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    if (typeFilter.length === 0) return events;
    return events.filter((e) => typeFilter.includes(e.type));
  }, [events, typeFilter]);

  useEffect(() => {
    if (paused) return;
    const el = containerRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [filtered, paused]);

  const typeOptions = useMemo(() => {
    const set = new Set(events.map((e) => e.type));
    return Array.from(set).map((t) => ({ value: t, label: t }));
  }, [events]);

  return (
    <div>
      <Space style={{ marginBottom: 12 }} wrap>
        <Badge status={connected ? 'success' : 'default'} text={connected ? 'SSE 已连接' : error ?? '连接中...'} />
        <Select
          mode="multiple"
          allowClear
          placeholder="按 event type 筛选"
          style={{ minWidth: 260 }}
          options={typeOptions}
          value={typeFilter}
          onChange={setTypeFilter}
        />
        <Button size="small" onClick={() => setTypeFilter(['TASK_FAILED', 'STEP_FAILED', 'LOOP_DETECTED', 'LIMIT_EXCEEDED'])}>
          只看错误/警告
        </Button>
        <Button size="small" onClick={() => setTypeFilter(['TOOL_CALL_STARTED', 'TOOL_CALL_COMPLETED', 'TOOL_APPROVAL_REQUIRED', 'TOOL_APPROVAL_DECIDED'])}>
          只看工具调用
        </Button>
        <Button size="small" onClick={() => setTypeFilter([])}>
          清除筛选
        </Button>
        <Space size={4}>
          <span>暂停滚动</span>
          <Switch checked={paused} onChange={setPaused} />
        </Space>
      </Space>
      <div ref={containerRef} style={{ maxHeight: 640, overflowY: 'auto', padding: 4, background: '#fafafa' }}>
        {filtered.length === 0 ? (
          <Empty description="暂无事件" />
        ) : (
          filtered.map((event) => (
            <TimelineEventCard
              key={event.event_id}
              event={event}
              onJumpToolCall={() => onJumpTab('toolcalls')}
              onJumpCheckpoint={() => onJumpTab('checkpoints')}
            />
          ))
        )}
      </div>
    </div>
  );
}

function StateSection({ taskId }: { taskId: string }) {
  const { data, isLoading } = useQuery({
    queryKey: ['task-state', taskId],
    queryFn: () => tasksApi.state(taskId),
    enabled: !!taskId,
  });
  if (isLoading) return <Typography.Text>加载中...</Typography.Text>;
  if (!data) return <Empty />;
  return (
    <Descriptions bordered size="small" column={2}>
      <Descriptions.Item label="version">{data.version}</Descriptions.Item>
      <Descriptions.Item label="current_step">{data.current_step}</Descriptions.Item>
      <Descriptions.Item label="updated_at">{formatDateTime(data.updated_at)}</Descriptions.Item>
      <Descriptions.Item label="memory_summary" span={2}>
        {data.memory_summary ?? '-'}
      </Descriptions.Item>
      <Descriptions.Item label="current_plan" span={2}>
        <JsonViewer value={data.current_plan ?? {}} />
      </Descriptions.Item>
      <Descriptions.Item label="constraints" span={2}>
        <JsonViewer value={data.constraints ?? {}} />
      </Descriptions.Item>
      <Descriptions.Item label="artifacts" span={2}>
        <JsonViewer value={data.artifacts ?? []} />
      </Descriptions.Item>
      <Descriptions.Item label="loop_fingerprints" span={2}>
        <JsonViewer value={data.loop_fingerprints ?? []} />
      </Descriptions.Item>
      {data.state && (
        <Descriptions.Item label="完整 state（仅管理员/审计员可见，敏感字段已脱敏）" span={2}>
          <JsonViewer value={data.state} />
        </Descriptions.Item>
      )}
    </Descriptions>
  );
}

function ToolCallsSection({ taskId }: { taskId: string }) {
  const { data, isLoading } = useQuery({
    queryKey: ['task-toolcalls', taskId],
    queryFn: () => tasksApi.toolCalls(taskId, { limit: 100 }),
    enabled: !!taskId,
  });

  return (
    <Table<ToolCall>
      rowKey="call_id"
      loading={isLoading}
      dataSource={data?.items ?? []}
      pagination={{ pageSize: 20 }}
      columns={[
        {
          title: 'call_id',
          dataIndex: 'call_id',
          render: (id: string) => (
            <Space size={4}>
              <Typography.Text copyable={{ text: id }}>{id}</Typography.Text>
            </Space>
          ),
        },
        { title: 'tool_name', dataIndex: 'tool_name' },
        { title: 'risk_level', dataIndex: 'risk_level', render: (v: string) => <StatusTag status={v} kind="risk" /> },
        { title: 'status', dataIndex: 'status', render: (v: string) => <StatusTag status={v} kind="tool-call" /> },
        { title: 'approval_status', dataIndex: 'approval_status', render: (v: string) => <StatusTag status={v} kind="approval" /> },
        { title: 'idempotency_key', dataIndex: 'idempotency_key', render: (v?: string) => v ?? '-' },
        { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
      ]}
      expandable={{
        expandedRowRender: (row) => (
          <Space direction="vertical" style={{ width: '100%' }}>
            {row.error_code && <Alert type="error" message={`${row.error_code}: ${row.error_message ?? ''}`} showIcon />}
            <div>
              <Typography.Text strong>arguments: </Typography.Text>
              <JsonViewer value={row.arguments ?? {}} />
            </div>
            <div>
              <Typography.Text strong>result: </Typography.Text>
              <JsonViewer value={row.result ?? {}} />
            </div>
          </Space>
        ),
      }}
    />
  );
}

function CheckpointsSection({ taskId, onResumed, canWrite }: { taskId: string; onResumed: () => void; canWrite: boolean }) {
  const { data, isLoading } = useQuery({
    queryKey: ['task-checkpoints', taskId],
    queryFn: () => tasksApi.checkpoints(taskId, { limit: 20 }),
    enabled: !!taskId,
  });
  const [compareSelection, setCompareSelection] = useState<Checkpoint[]>([]);
  const [compareOpen, setCompareOpen] = useState(false);
  const [snapshotView, setSnapshotView] = useState<Checkpoint | null>(null);

  const resumeMutation = useMutation({
    mutationFn: (checkpointId: string) => tasksApi.resumeFromCheckpoint(taskId, checkpointId),
    onSuccess: () => {
      message.success('已从 checkpoint 恢复');
      onResumed();
    },
  });

  const confirmResumeFrom = (cp: Checkpoint) => {
    Modal.confirm({
      title: '确认从该 checkpoint 恢复？',
      content: (
        <div>
          <p>
            将从 checkpoint {cp.checkpoint_id} 恢复（{formatDateTime(cp.created_at)}，state_version={cp.state_version}）。
          </p>
          <p>恢复后可能会继续执行后续工具调用；已完成的幂等工具不会重复执行。</p>
        </div>
      ),
      okText: '确认恢复',
      cancelText: '再想想',
      onOk: () => resumeMutation.mutateAsync(cp.checkpoint_id),
    });
  };

  const downloadJson = (cp: Checkpoint) => {
    const blob = new Blob([safeJsonStringify(cp)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${cp.checkpoint_id}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const toggleCompare = (cp: Checkpoint, checked: boolean) => {
    setCompareSelection((prev) => {
      if (checked) {
        const next = [...prev.filter((p) => p.checkpoint_id !== cp.checkpoint_id), cp];
        return next.slice(-2);
      }
      return prev.filter((p) => p.checkpoint_id !== cp.checkpoint_id);
    });
  };

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <Typography.Text type="secondary">默认只展示最近 20 个 checkpoint。勾选两个可对比。</Typography.Text>
        <Button disabled={compareSelection.length !== 2} onClick={() => setCompareOpen(true)}>
          对比所选 Checkpoint
        </Button>
      </Space>
      <Table<Checkpoint>
        rowKey="checkpoint_id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        pagination={{ pageSize: 20 }}
        rowSelection={{
          type: 'checkbox',
          selectedRowKeys: compareSelection.map((c) => c.checkpoint_id),
          onSelect: (record, checked) => toggleCompare(record, checked),
        }}
        columns={[
          { title: 'checkpoint_id', dataIndex: 'checkpoint_id' },
          { title: 'reason', dataIndex: 'reason', render: (v: string) => <Tag>{v}</Tag> },
          { title: 'state_version', dataIndex: 'state_version' },
          { title: 'event_offset', dataIndex: 'event_offset' },
          { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
          {
            title: 'Actions',
            render: (_, cp) => (
              <Space size={4}>
                <Button size="small" onClick={() => setSnapshotView(cp)}>
                  查看 snapshot
                </Button>
                <Button size="small" onClick={() => downloadJson(cp)}>
                  下载 JSON
                </Button>
                {canWrite && (
                  <Button size="small" type="primary" onClick={() => confirmResumeFrom(cp)}>
                    从此恢复
                  </Button>
                )}
              </Space>
            ),
          },
        ]}
      />
      <Drawer title={snapshotView?.checkpoint_id} open={!!snapshotView} onClose={() => setSnapshotView(null)} width={520}>
        {snapshotView && <JsonViewer value={snapshotView.state_snapshot ?? {}} collapsed={false} />}
      </Drawer>
      <Modal title="Checkpoint 对比" open={compareOpen} onCancel={() => setCompareOpen(false)} footer={null} width={900}>
        <Space align="start" size="large" style={{ width: '100%' }}>
          {compareSelection.map((cp) => (
            <div key={cp.checkpoint_id} style={{ flex: 1 }}>
              <Typography.Text strong>
                {cp.checkpoint_id}（v{cp.state_version}）
              </Typography.Text>
              <JsonViewer value={cp.state_snapshot ?? {}} collapsed={false} />
            </div>
          ))}
        </Space>
      </Modal>
    </div>
  );
}

function ArtifactsSection({ taskId }: { taskId: string }) {
  const { data, isLoading } = useQuery({
    queryKey: ['task-artifacts', taskId],
    queryFn: () => tasksApi.artifacts(taskId, { limit: 100 }),
    enabled: !!taskId,
  });
  const navigate = useNavigate();

  return (
    <Table
      rowKey="artifact_id"
      loading={isLoading}
      dataSource={data?.items ?? []}
      pagination={{ pageSize: 20 }}
      columns={[
        { title: 'name', dataIndex: 'name' },
        { title: 'type', dataIndex: 'type', render: (v: string) => <Tag>{v}</Tag> },
        { title: 'size_bytes', dataIndex: 'size_bytes' },
        { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
        {
          title: 'Actions',
          render: (_, row: { artifact_id: string }) => (
            <Button size="small" onClick={() => navigate(`/artifacts?artifact_id=${row.artifact_id}`)}>
              查看
            </Button>
          ),
        },
      ]}
    />
  );
}
