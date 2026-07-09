import { Card, Space, Tag, Typography } from 'antd';
import { LinkOutlined } from '@ant-design/icons';
import type { AgentEvent } from '@/types/event';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { eventTypeLabel, eventTypeVariant, VARIANT_BORDER_COLOR } from '@/utils/statusMeta';
import { formatDateTime, summarize } from '@/utils/format';

interface TimelineEventCardProps {
  event: AgentEvent;
  onJumpToolCall?: (callId: string) => void;
  onJumpCheckpoint?: (checkpointId: string) => void;
}

// TimelineEventCard 按事件类型渲染卡片：不同类型用左侧色条区分（设计稿第六节第二张表），
// 审批卡片突出显示，警告/错误卡片使用醒目颜色。
export function TimelineEventCard({ event, onJumpToolCall, onJumpCheckpoint }: TimelineEventCardProps) {
  const variant = eventTypeVariant(event.type);
  const borderColor = VARIANT_BORDER_COLOR[variant];
  const emphasize = variant === 'approval' || variant === 'error' || variant === 'warning';

  const inputObj = event.input as Record<string, unknown> | undefined;
  const callId = typeof inputObj?.call_id === 'string' ? inputObj.call_id : undefined;
  const checkpointId = typeof inputObj?.checkpoint_id === 'string' ? inputObj.checkpoint_id : undefined;

  return (
    <Card
      size="small"
      style={{
        borderLeft: `4px solid ${borderColor}`,
        marginBottom: 8,
        background: emphasize ? '#fffbe6' : undefined,
      }}
      styles={{ body: { padding: 12 } }}
    >
      <Space direction="vertical" size={4} style={{ width: '100%' }}>
        <Space wrap>
          <Tag color={borderColor}>{eventTypeLabel(event.type)}</Tag>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            #{event.event_id} · {formatDateTime(event.created_at)}
          </Typography.Text>
          {event.metadata?.step_id && <Tag>step: {String(event.metadata.step_id)}</Tag>}
        </Space>
        <Space size={16} wrap style={{ fontSize: 13 }}>
          <span>
            <Typography.Text type="secondary">input: </Typography.Text>
            {summarize(event.input)}
          </span>
          <span>
            <Typography.Text type="secondary">output: </Typography.Text>
            {summarize(event.output)}
          </span>
        </Space>
        <Space size={12}>
          {callId && onJumpToolCall && (
            <Typography.Link onClick={() => onJumpToolCall(callId)}>
              <LinkOutlined /> 查看 ToolCall {callId}
            </Typography.Link>
          )}
          {checkpointId && onJumpCheckpoint && (
            <Typography.Link onClick={() => onJumpCheckpoint(checkpointId)}>
              <LinkOutlined /> 查看 Checkpoint {checkpointId}
            </Typography.Link>
          )}
        </Space>
        <JsonViewer value={event} title="展开事件 JSON" />
      </Space>
    </Card>
  );
}
