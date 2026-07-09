import { useEffect, useState } from 'react';
import { Button, Card, Input, Space, Tabs, Typography, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { settingsApi } from '@/api/settings';
import type { SettingsSection } from '@/types/admin';
import { JsonViewer } from '@/components/json-viewer/JsonViewer';
import { usePermission } from '@/hooks/usePermission';

const SECTIONS: SettingsSection[] = ['runtime', 'llm', 'tool', 'security', 'retention', 'observability'];

// SettingsPage 实现系统设置页，平台管理员可整组保存，租户管理员只读。
export function SettingsPage() {
  const perm = usePermission();
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['settings'], queryFn: settingsApi.get });
  const [section, setSection] = useState<SettingsSection>('runtime');
  const [text, setText] = useState('{}');

  useEffect(() => {
    if (data?.[section]) {
      setText(JSON.stringify(data[section], null, 2));
    }
  }, [data, section]);

  const saveMutation = useMutation({
    mutationFn: () => settingsApi.update(section, parseSettings(text)),
    onSuccess: () => {
      message.success('设置已保存');
      queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });

  return (
    <Card title="Settings" loading={isLoading}>
      <Tabs activeKey={section} onChange={(key) => setSection(key as SettingsSection)} items={SECTIONS.map((key) => ({ key, label: key }))} />
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        {perm.canWriteSettings ? (
          <>
            <Input.TextArea value={text} rows={18} onChange={(event) => setText(event.target.value)} />
            <Button type="primary" onClick={() => saveMutation.mutate()} loading={saveMutation.isPending}>
              保存 {section}
            </Button>
          </>
        ) : (
          <>
            <Typography.Text type="secondary">当前角色只读。</Typography.Text>
            <JsonViewer value={data?.[section] ?? {}} collapsed={false} />
          </>
        )}
      </Space>
    </Card>
  );
}

// parseSettings 将设置编辑器文本解析为对象。
function parseSettings(raw: string): Record<string, unknown> {
  try {
    return JSON.parse(raw || '{}') as Record<string, unknown>;
  } catch {
    message.error('设置内容不是合法 JSON');
    throw new Error('invalid settings json');
  }
}
