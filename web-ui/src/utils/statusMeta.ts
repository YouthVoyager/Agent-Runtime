// 状态颜色对照表，严格按 docs/admin-console-design.md 第六节。
// AntD Tag color 使用预设色名，与设计稿"灰/蓝/橙/黄/红/紫/绿"语义对应。
import type { TaskStatus } from '@/types/task';

export const TASK_STATUS_COLOR: Record<TaskStatus, string> = {
  QUEUED: 'default',
  RUNNING: 'blue',
  WAITING_APPROVAL: 'orange',
  PAUSED: 'gold',
  CANCELING: 'orange',
  CANCELED: 'default',
  SUCCEEDED: 'green',
  FAILED: 'red',
  RETRYABLE_FAILED: 'red',
  STOPPED_BY_LIMIT: 'purple',
};

export const TASK_STATUS_LABEL: Record<TaskStatus, string> = {
  QUEUED: '排队中',
  RUNNING: '执行中',
  WAITING_APPROVAL: '等待审批',
  PAUSED: '已暂停',
  CANCELING: '取消中',
  CANCELED: '已取消',
  SUCCEEDED: '已完成',
  FAILED: '失败',
  RETRYABLE_FAILED: '可恢复失败',
  STOPPED_BY_LIMIT: '被限制停止',
};

export function taskStatusColor(status: string): string {
  return TASK_STATUS_COLOR[status as TaskStatus] ?? 'default';
}

export function taskStatusLabel(status: string): string {
  return TASK_STATUS_LABEL[status as TaskStatus] ?? status;
}

export const APPROVAL_STATUS_COLOR: Record<string, string> = {
  PENDING: 'orange',
  APPROVED: 'green',
  REJECTED: 'red',
  EXPIRED: 'default',
};

export const TOOL_CALL_STATUS_COLOR: Record<string, string> = {
  PENDING: 'default',
  RUNNING: 'blue',
  SUCCEEDED: 'green',
  FAILED: 'red',
  AWAITING_APPROVAL: 'orange',
};

export const RISK_LEVEL_COLOR: Record<string, string> = {
  LOW: 'green',
  MEDIUM: 'gold',
  HIGH: 'orange',
  CRITICAL: 'red',
};

// Event 类型展示样式，对照设计稿第六节第二张表。
export type EventCardVariant = 'info' | 'step' | 'llm' | 'llm-result' | 'tool' | 'approval' | 'checkpoint' | 'warning' | 'error' | 'success';

export const EVENT_TYPE_VARIANT: Record<string, EventCardVariant> = {
  TASK_CREATED: 'info',
  TASK_STARTED: 'info',
  STEP_STARTED: 'step',
  STEP_COMPLETED: 'step',
  STEP_FAILED: 'warning',
  LLM_CALL_STARTED: 'llm',
  LLM_CALL_COMPLETED: 'llm-result',
  TOOL_CALL_STARTED: 'tool',
  TOOL_CALL_COMPLETED: 'tool',
  TOOL_APPROVAL_REQUIRED: 'approval',
  TOOL_APPROVAL_DECIDED: 'approval',
  CHECKPOINT_CREATED: 'checkpoint',
  LOOP_DETECTED: 'warning',
  LIMIT_EXCEEDED: 'warning',
  TASK_FAILED: 'error',
  TASK_CANCELLED: 'warning',
  TASK_RESUMED: 'info',
  TASK_COMPLETED: 'success',
  ARTIFACT_SAVED: 'success',
};

export const EVENT_TYPE_LABEL: Record<string, string> = {
  TASK_CREATED: '任务创建',
  TASK_STARTED: '任务启动',
  TASK_COMPLETED: '任务完成',
  TASK_FAILED: '任务失败',
  TASK_CANCELLED: '任务取消',
  TASK_RESUMED: '任务恢复',
  STEP_STARTED: 'Step 开始',
  STEP_COMPLETED: 'Step 完成',
  STEP_FAILED: 'Step 失败',
  LLM_CALL_STARTED: 'LLM 调用开始',
  LLM_CALL_COMPLETED: 'LLM 调用完成',
  TOOL_CALL_STARTED: '工具调用开始',
  TOOL_CALL_COMPLETED: '工具调用完成',
  TOOL_APPROVAL_REQUIRED: '需要审批',
  TOOL_APPROVAL_DECIDED: '审批已决策',
  CHECKPOINT_CREATED: 'Checkpoint 创建',
  ARTIFACT_SAVED: 'Artifact 已保存',
  LOOP_DETECTED: '检测到循环',
  LIMIT_EXCEEDED: '超出预算限制',
};

export function eventTypeVariant(type: string): EventCardVariant {
  return EVENT_TYPE_VARIANT[type] ?? 'info';
}

export function eventTypeLabel(type: string): string {
  return EVENT_TYPE_LABEL[type] ?? type;
}

export const VARIANT_BORDER_COLOR: Record<EventCardVariant, string> = {
  info: '#d9d9d9',
  step: '#91caff',
  llm: '#95de64',
  'llm-result': '#52c41a',
  tool: '#69b1ff',
  approval: '#ffa940',
  checkpoint: '#b37feb',
  warning: '#faad14',
  error: '#ff4d4f',
  success: '#52c41a',
};
