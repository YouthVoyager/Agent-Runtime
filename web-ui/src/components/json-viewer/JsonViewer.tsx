import { useState } from 'react';
import { Button, Space, Typography, message } from 'antd';
import { CopyOutlined, DownOutlined, RightOutlined } from '@ant-design/icons';
import { safeJsonStringify } from '@/utils/format';

interface JsonViewerProps {
  value: unknown;
  collapsed?: boolean;
  title?: string;
}

// JsonViewer 折叠展示任意 JSON 值，带一键复制。用于 timeline event、tool call、checkpoint 等原始数据展示。
export function JsonViewer({ value, collapsed = true, title }: JsonViewerProps) {
  const [open, setOpen] = useState(!collapsed);
  const text = safeJsonStringify(value);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      message.success('已复制 JSON');
    } catch {
      message.error('复制失败');
    }
  };

  return (
    <div>
      <Space size="small" style={{ marginBottom: 4 }}>
        <Button size="small" type="text" icon={open ? <DownOutlined /> : <RightOutlined />} onClick={() => setOpen(!open)}>
          {title ?? (open ? '收起 JSON' : '展开 JSON')}
        </Button>
        <Button size="small" type="text" icon={<CopyOutlined />} onClick={copy}>
          复制
        </Button>
      </Space>
      {open && (
        <Typography.Paragraph>
          <pre
            style={{
              background: '#f5f5f5',
              padding: 12,
              borderRadius: 4,
              maxHeight: 400,
              overflow: 'auto',
              fontSize: 12,
              margin: 0,
            }}
          >
            {text}
          </pre>
        </Typography.Paragraph>
      )}
    </div>
  );
}
