import { useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Drawer,
  Form,
  Input,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { approvalsApi } from '@/api/approvals';
import type { ApprovalItem, ApprovalStatus } from '@/types/approval';
import { StatusTag } from '@/components/status-tag/StatusTag';
import { PageTable } from '@/components/table/PageTable';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { formatDateTime } from '@/utils/format';
import { usePermission } from '@/hooks/usePermission';

const TABS: { key: string; label: string; status?: ApprovalStatus; mine?: boolean }[] = [
  { key: 'pending', label: 'Pending', status: 'PENDING' },
  { key: 'approved', label: 'Approved', status: 'APPROVED' },
  { key: 'rejected', label: 'Rejected', status: 'REJECTED' },
  { key: 'expired', label: 'Expired', status: 'EXPIRED' },
  { key: 'mine', label: 'My Decisions', mine: true },
];

// ApprovalsPage:审批中心，集中处理所有高风险工具调用（设计稿第四节第 5 页）。
export function ApprovalsPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const perm = usePermission();
  const [activeTab, setActiveTab] = useState('pending');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [taskIdFilter, setTaskIdFilter] = useState(searchParams.get('task_id') ?? '');
  const [selectedApprovalId, setSelectedApprovalId] = useState<string | null>(null);

  const tabConfig = TABS.find((t) => t.key === activeTab)!;

  const { data, isFetching, refetch } = useQuery({
    queryKey: ['approvals', activeTab, page, pageSize, taskIdFilter],
    queryFn: () =>
      approvalsApi.list({
        status: tabConfig.status,
        mine: tabConfig.mine,
        task_id: taskIdFilter || undefined,
        page,
        page_size: pageSize,
      }),
  });

  const columns = useMemo(
    () => [
      { title: 'approval_id', dataIndex: 'approval_id' },
      {
        title: 'task_id',
        dataIndex: 'task_id',
        render: (id: string) => <Typography.Link onClick={() => navigate(`/tasks/${id}`)}>{id}</Typography.Link>,
      },
      { title: 'tool_name', dataIndex: 'tool_name' },
      { title: 'risk_level', dataIndex: 'risk_level', render: (v: string) => <StatusTag status={v} kind="risk" /> },
      { title: 'status', dataIndex: 'status', render: (v: string) => <StatusTag status={v} kind="approval" /> },
      { title: 'requester', dataIndex: 'requester_id' },
      { title: 'approver', dataIndex: 'approver_id', render: (v?: string) => v ?? '-' },
      { title: 'created_at', dataIndex: 'created_at', render: (v: string) => formatDateTime(v) },
      { title: 'expires_at', dataIndex: 'expires_at', render: (v?: string) => (v ? formatDateTime(v) : '-') },
      {
        title: 'Actions',
        render: (_: unknown, row: ApprovalItem) => (
          <Button size="small" onClick={() => setSelectedApprovalId(row.approval_id)}>
            查看详情
          </Button>
        ),
      },
    ],
    [navigate]
  );

  return (
    <Card title="Approvals">
      <Space style={{ marginBottom: 12 }}>
        <Input
          placeholder="按 task_id 筛选"
          value={taskIdFilter}
          onChange={(e) => setTaskIdFilter(e.target.value)}
          onPressEnter={() => {
            setPage(1);
            refetch();
          }}
          style={{ width: 240 }}
          allowClear
        />
      </Space>
      <Tabs
        activeKey={activeTab}
        onChange={(key) => {
          setActiveTab(key);
          setPage(1);
        }}
        items={TABS.map((t) => ({ key: t.key, label: t.label }))}
      />
      <PageTable<ApprovalItem>
        rowKey="approval_id"
        columns={columns}
        data={data}
        loading={isFetching}
        page={page}
        pageSize={pageSize}
        onPageChange={(p, ps) => {
          setPage(p);
          setPageSize(ps);
        }}
      />
      {selectedApprovalId && (
        <ApprovalDetailDrawer
          approvalId={selectedApprovalId}
          canDecide={perm.canWrite}
          onClose={() => setSelectedApprovalId(null)}
          onDecided={() => {
            setSelectedApprovalId(null);
            refetch();
          }}
        />
      )}
    </Card>
  );
}

function ApprovalDetailDrawer({
  approvalId,
  canDecide,
  onClose,
  onDecided,
}: {
  approvalId: string;
  canDecide: boolean;
  onClose: () => void;
  onDecided: () => void;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [approveOpen, setApproveOpen] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);
  const [approveForm] = Form.useForm();
  const [rejectForm] = Form.useForm();

  const { data, isLoading } = useQuery({
    queryKey: ['approval', approvalId],
    queryFn: () => approvalsApi.get(approvalId),
  });

  const approveMutation = useMutation({
    mutationFn: (note: string) => approvalsApi.approve(data!.call_id, { note }),
    onSuccess: () => {
      message.success('已批准');
      queryClient.invalidateQueries({ queryKey: ['approvals'] });
      setApproveOpen(false);
      onDecided();
    },
  });
  const rejectMutation = useMutation({
    mutationFn: (payload: { reason: string; terminate_task: boolean }) => approvalsApi.reject(data!.call_id, payload),
    onSuccess: () => {
      message.success('已拒绝');
      queryClient.invalidateQueries({ queryKey: ['approvals'] });
      setRejectOpen(false);
      onDecided();
    },
  });

  return (
    <Drawer title={`审批详情 · ${approvalId}`} open onClose={onClose} width={560} loading={isLoading}>
      {data && (
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Alert
            type="warning"
            showIcon
            message={`Agent 想调用工具 ${data.tool_name}，风险等级 ${data.risk_level}`}
            description={data.approval_reason || '（后端未提供审批原因）'}
          />
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="task_id">
              <Typography.Link onClick={() => navigate(`/tasks/${data.task_id}`)}>{data.task_id}</Typography.Link>
            </Descriptions.Item>
            <Descriptions.Item label="task_goal">{data.task_goal ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="current_step">{data.current_step ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="requester_id">{data.requester_id}</Descriptions.Item>
            <Descriptions.Item label="idempotency_key">{data.idempotency_key ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="status">
              <StatusTag status={data.status} kind="approval" />
            </Descriptions.Item>
            <Descriptions.Item label="工具参数（已脱敏）">
              <JsonViewer value={data.arguments ?? {}} collapsed={false} />
            </Descriptions.Item>
          </Descriptions>

          <Typography.Title level={5}>审批历史</Typography.Title>
          <Table
            size="small"
            rowKey="approval_id"
            pagination={false}
            dataSource={data.history}
            columns={[
              { title: 'status', dataIndex: 'status', render: (v: string) => <Tag>{v}</Tag> },
              { title: 'approver_id', dataIndex: 'approver_id', render: (v?: string) => v ?? '-' },
              { title: 'note', dataIndex: 'note', render: (v?: string) => v ?? '-' },
              { title: 'decided_at', dataIndex: 'decided_at', render: (v?: string) => (v ? formatDateTime(v) : '-') },
            ]}
          />

          <Space>
            <Button onClick={() => navigate(`/tasks/${data.task_id}`)}>View Task</Button>
            <Button onClick={() => navigate(`/tools/calls?call_id=${data.call_id}`)}>View Tool Call</Button>
            {canDecide && data.status === 'PENDING' && (
              <>
                <Button type="primary" onClick={() => setApproveOpen(true)}>
                  Approve
                </Button>
                <Button danger onClick={() => setRejectOpen(true)}>
                  Reject
                </Button>
              </>
            )}
          </Space>

          {approveOpen && (
            <Card size="small" title="Approve">
              <Form
                form={approveForm}
                layout="vertical"
                onFinish={(values) => {
                  const extra: string[] = [];
                  if (values.onceOnly) extra.push('[仅批准本次]');
                  if (values.autoApproveSimilar) extra.push('[后续同类工具自动批准]');
                  const note = [values.note, ...extra].filter(Boolean).join(' ');
                  approveMutation.mutate(note);
                }}
              >
                <Form.Item name="note" label="审批备注（可选）">
                  <Input.TextArea rows={2} />
                </Form.Item>
                <Form.Item name="onceOnly" valuePropName="checked" initialValue={true}>
                  <Checkbox>仅批准本次</Checkbox>
                </Form.Item>
                <Form.Item name="autoApproveSimilar" valuePropName="checked" initialValue={false}>
                  <Checkbox>后续同类工具自动批准（管理员可选）</Checkbox>
                </Form.Item>
                <Space>
                  <Button type="primary" htmlType="submit" loading={approveMutation.isPending}>
                    确认批准
                  </Button>
                  <Button onClick={() => setApproveOpen(false)}>取消</Button>
                </Space>
              </Form>
            </Card>
          )}

          {rejectOpen && (
            <Card size="small" title="Reject">
              <Form
                form={rejectForm}
                layout="vertical"
                onFinish={(values) => {
                  const reason = values.allowReplan ? `${values.reason} [允许 Agent 继续调整计划]` : values.reason;
                  rejectMutation.mutate({ reason, terminate_task: !!values.terminateTask });
                }}
              >
                <Form.Item name="reason" label="拒绝原因" rules={[{ required: true, message: '拒绝原因必填' }]}>
                  <Input.TextArea rows={2} />
                </Form.Item>
                <Form.Item name="allowReplan" valuePropName="checked" initialValue={true}>
                  <Checkbox>允许 Agent 继续调整计划</Checkbox>
                </Form.Item>
                <Form.Item name="terminateTask" valuePropName="checked" initialValue={false}>
                  <Checkbox>直接终止任务</Checkbox>
                </Form.Item>
                <Space>
                  <Button danger type="primary" htmlType="submit" loading={rejectMutation.isPending}>
                    确认拒绝
                  </Button>
                  <Button onClick={() => setRejectOpen(false)}>取消</Button>
                </Space>
              </Form>
            </Card>
          )}
        </Space>
      )}
    </Drawer>
  );
}
