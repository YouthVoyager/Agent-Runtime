import { Tag } from 'antd';
import { APPROVAL_STATUS_COLOR, RISK_LEVEL_COLOR, TOOL_CALL_STATUS_COLOR, taskStatusColor, taskStatusLabel } from '@/utils/statusMeta';

interface StatusTagProps {
  status: string;
  kind?: 'task' | 'approval' | 'tool-call' | 'risk';
}

// StatusTag 统一渲染任务 / 审批 / 工具调用 / 风险等级的状态徽标，颜色严格对照设计稿第六节。
export function StatusTag({ status, kind = 'task' }: StatusTagProps) {
  if (!status) return <Tag>-</Tag>;
  if (kind === 'task') {
    return <Tag color={taskStatusColor(status)}>{taskStatusLabel(status)}</Tag>;
  }
  if (kind === 'approval') {
    return <Tag color={APPROVAL_STATUS_COLOR[status] ?? 'default'}>{status}</Tag>;
  }
  if (kind === 'risk') {
    return <Tag color={RISK_LEVEL_COLOR[status] ?? 'default'}>{status}</Tag>;
  }
  return <Tag color={TOOL_CALL_STATUS_COLOR[status] ?? 'default'}>{status}</Tag>;
}
