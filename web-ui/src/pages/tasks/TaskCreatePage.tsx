import { useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
  message,
} from 'antd';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { tasksApi } from '@/api/tasks';
import { toolsApi } from '@/api/tools';
import { taskTemplatesApi } from '@/api/taskTemplates';
import type { CreateTaskRequest } from '@/types/task';
import { RISK_LEVEL_COLOR } from '@/utils/statusMeta';

const DEFAULT_BUDGET = { max_steps: 50, max_tokens: 200000, max_tool_calls: 40, max_cost_usd: 10 };

interface FormValues {
  goal: string;
  task_name?: string;
  priority?: 'low' | 'normal' | 'high';
  tags?: string[];
  max_steps: number;
  max_tokens: number;
  max_tool_calls: number;
  max_cost_usd: number;
  timeout_minutes: number;
  language?: string;
  require_approval_for_high_risk_tools: boolean;
  allowed_tools?: string[];
  disallowed_tools?: string[];
  output_format?: string;
  memory_mode?: string;
  strict_mode: boolean;
  model?: string;
  tool_policy?: string;
  retry_policy?: string;
  webhook_url?: string;
  artifact_storage?: string;
}

// TaskCreatePage 对照设计稿第四节第 3 页四组表单实现。
// 注意:后端 CreateTaskRequest 只接受 goal / budget / constraints 三个顶层字段(additionalProperties: false)，
// 其余"基础信息/执行约束/高级配置"字段统一打包进 constraints(该字段 additionalProperties: true)。
export function TaskCreatePage() {
  const navigate = useNavigate();
  const [form] = Form.useForm<FormValues>();
  const [templateModalOpen, setTemplateModalOpen] = useState(false);
  const [templateName, setTemplateName] = useState('');

  const { data: availableTools } = useQuery({
    queryKey: ['tools', 'available'],
    queryFn: () => toolsApi.available(),
  });
  const { data: templates, refetch: refetchTemplates } = useQuery({
    queryKey: ['task-templates'],
    queryFn: () => taskTemplatesApi.list(),
  });

  const createMutation = useMutation({
    mutationFn: (body: CreateTaskRequest) => tasksApi.create(body),
    onSuccess: (created) => {
      message.success(`任务已创建：${created.task_id}`);
      navigate(`/tasks/${created.task_id}`);
    },
  });

  const buildRequest = (values: FormValues): CreateTaskRequest => ({
    goal: values.goal,
    budget: {
      max_steps: values.max_steps,
      max_tokens: values.max_tokens,
      max_tool_calls: values.max_tool_calls,
      max_cost_usd: values.max_cost_usd,
    },
    constraints: {
      task_name: values.task_name,
      priority: values.priority ?? 'normal',
      tags: values.tags ?? [],
      timeout_seconds: (values.timeout_minutes ?? 30) * 60,
      language: values.language ?? 'zh-CN',
      require_approval_for_high_risk_tools: values.require_approval_for_high_risk_tools,
      allowed_tools: values.allowed_tools ?? [],
      disallowed_tools: values.disallowed_tools ?? [],
      output_format: values.output_format ?? 'markdown',
      memory_mode: values.memory_mode ?? 'summary',
      strict_mode: values.strict_mode ?? false,
      model: values.model || undefined,
      tool_policy: values.tool_policy || undefined,
      retry_policy: values.retry_policy || undefined,
      webhook_url: values.webhook_url || undefined,
      artifact_storage: values.artifact_storage || undefined,
    },
  });

  const handleSubmit = (values: FormValues) => {
    createMutation.mutate(buildRequest(values));
  };

  const saveTemplate = async () => {
    const values = await form.validateFields();
    if (!templateName.trim()) {
      message.error('模板名称不能为空');
      return;
    }
    await taskTemplatesApi.create({ name: templateName.trim(), payload: buildRequest(values), shared: false });
    message.success('模板已保存');
    setTemplateModalOpen(false);
    setTemplateName('');
    refetchTemplates();
  };

  const loadTemplate = (templateId: string) => {
    const tpl = templates?.items.find((t) => t.template_id === templateId);
    if (!tpl) return;
    const payload = tpl.payload as CreateTaskRequest;
    const c = (payload.constraints ?? {}) as Record<string, unknown>;
    form.setFieldsValue({
      goal: payload.goal,
      task_name: c.task_name as string,
      priority: (c.priority as FormValues['priority']) ?? 'normal',
      tags: (c.tags as string[]) ?? [],
      max_steps: payload.budget.max_steps,
      max_tokens: payload.budget.max_tokens,
      max_tool_calls: payload.budget.max_tool_calls,
      max_cost_usd: payload.budget.max_cost_usd,
      timeout_minutes: c.timeout_seconds ? Math.round((c.timeout_seconds as number) / 60) : 30,
      language: (c.language as string) ?? 'zh-CN',
      require_approval_for_high_risk_tools: (c.require_approval_for_high_risk_tools as boolean) ?? true,
      allowed_tools: (c.allowed_tools as string[]) ?? [],
      disallowed_tools: (c.disallowed_tools as string[]) ?? [],
      output_format: (c.output_format as string) ?? 'markdown',
      memory_mode: (c.memory_mode as string) ?? 'summary',
      strict_mode: (c.strict_mode as boolean) ?? false,
      model: c.model as string,
      tool_policy: c.tool_policy as string,
      retry_policy: c.retry_policy as string,
      webhook_url: c.webhook_url as string,
      artifact_storage: c.artifact_storage as string,
    });
    message.success(`已加载模板：${tpl.name}`);
  };

  const toolOptions = (availableTools ?? []).map((t) => ({
    value: t.tool_name,
    label: (
      <span>
        {t.tool_name} <Tag color={RISK_LEVEL_COLOR[t.risk_level] ?? 'default'}>{t.risk_level}</Tag>
      </span>
    ),
  }));

  return (
    <Card title="Task Create">
      <Space style={{ marginBottom: 16 }}>
        <Select
          placeholder="加载已保存模板"
          style={{ width: 240 }}
          options={(templates?.items ?? []).map((t) => ({ value: t.template_id, label: t.name }))}
          onChange={loadTemplate}
          allowClear
        />
        <Button onClick={() => setTemplateModalOpen(true)}>保存为模板</Button>
      </Space>

      <Form<FormValues>
        form={form}
        layout="vertical"
        initialValues={{
          priority: 'normal',
          ...DEFAULT_BUDGET,
          timeout_minutes: 30,
          language: 'zh-CN',
          require_approval_for_high_risk_tools: true,
          output_format: 'markdown',
          memory_mode: 'summary',
          strict_mode: false,
        }}
        onFinish={handleSubmit}
      >
        <Divider orientation="left">基础信息</Divider>
        <Form.Item name="goal" label="Goal" rules={[{ required: true, message: 'Goal 不能为空' }]}>
          <Input.TextArea rows={4} placeholder="描述希望 Agent 完成的目标" />
        </Form.Item>
        <Row gutter={16}>
          <Col span={8}>
            <Form.Item name="task_name" label="Task Name">
              <Input placeholder="方便识别的名称（可选）" />
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item name="priority" label="Priority">
              <Select
                options={[
                  { value: 'low', label: 'low' },
                  { value: 'normal', label: 'normal' },
                  { value: 'high', label: 'high' },
                ]}
              />
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item name="tags" label="Tags">
              <Select mode="tags" placeholder="回车添加标签" />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">执行预算</Divider>
        <Row gutter={16}>
          <Col span={6}>
            <Form.Item name="max_steps" label="Max Steps" rules={[{ required: true }]}>
              <InputNumber min={1} max={1000} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="max_tokens" label="Max Tokens" rules={[{ required: true }]}>
              <InputNumber min={1} max={10000000} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="max_tool_calls" label="Max Tool Calls" rules={[{ required: true }]}>
              <InputNumber min={1} max={10000} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="max_cost_usd" label="Max Cost USD" rules={[{ required: true }]}>
              <InputNumber min={0.01} max={100000} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
        </Row>
        <Row gutter={16}>
          <Col span={6}>
            <Form.Item name="timeout_minutes" label="Timeout (分钟)">
              <InputNumber min={1} max={1440} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">执行约束</Divider>
        <Row gutter={16}>
          <Col span={6}>
            <Form.Item name="language" label="Language">
              <Select
                options={[
                  { value: 'zh-CN', label: 'zh-CN' },
                  { value: 'en-US', label: 'en-US' },
                ]}
              />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="require_approval_for_high_risk_tools" label="Require Approval" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="output_format" label="Output Format">
              <Select
                options={[
                  { value: 'markdown', label: 'markdown' },
                  { value: 'json', label: 'json' },
                  { value: 'file', label: 'file' },
                ]}
              />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="memory_mode" label="Memory Mode">
              <Select
                options={[
                  { value: 'none', label: 'none' },
                  { value: 'summary', label: 'summary' },
                  { value: 'full', label: 'full' },
                ]}
              />
            </Form.Item>
          </Col>
        </Row>
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="allowed_tools" label="Allowed Tools">
              <Select mode="multiple" options={toolOptions} placeholder="不选表示不限制" />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="disallowed_tools" label="Disallowed Tools">
              <Select mode="multiple" options={toolOptions} placeholder="不选表示不限制" />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="strict_mode" label="Strict Mode" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">高级配置</Divider>
        <Row gutter={16}>
          <Col span={6}>
            <Form.Item name="model" label="Model">
              <Input placeholder="默认使用租户配置模型" />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="tool_policy" label="Tool Policy">
              <Input placeholder="策略名称（可选）" />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="retry_policy" label="Retry Policy">
              <Input placeholder="重试策略名称（可选）" />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="webhook_url" label="Webhook URL">
              <Input placeholder="https://..." />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="artifact_storage" label="Artifact Storage">
          <Input placeholder="产物存储位置（可选）" />
        </Form.Item>

        {createMutation.isError && (
          <Alert type="error" showIcon message="创建失败，请检查表单后重试" style={{ marginBottom: 16 }} />
        )}

        <Form.Item>
          <Button type="primary" htmlType="submit" loading={createMutation.isPending}>
            提交
          </Button>
        </Form.Item>
      </Form>

      <Modal
        title="保存为任务模板"
        open={templateModalOpen}
        onOk={saveTemplate}
        onCancel={() => setTemplateModalOpen(false)}
      >
        <Typography.Paragraph type="secondary">保存当前表单内容为可复用模板。</Typography.Paragraph>
        <Input placeholder="模板名称" value={templateName} onChange={(e) => setTemplateName(e.target.value)} />
      </Modal>
    </Card>
  );
}
