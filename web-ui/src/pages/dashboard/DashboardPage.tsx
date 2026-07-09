import { useState } from 'react';
import { Alert, Card, Col, Input, Row, Segmented, Space, Statistic, Tag, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import ReactECharts from 'echarts-for-react';
import { dashboardApi } from '@/api/dashboard';
import type { DashboardRange } from '@/types/dashboard';
import { usePermission } from '@/hooks/usePermission';
import { formatCost, formatDateTime, formatNumber, formatPercent } from '@/utils/format';

const RANGE_OPTIONS: { label: string; value: DashboardRange }[] = [
  { label: '最近 1 小时', value: '1h' },
  { label: '最近 24 小时', value: '24h' },
  { label: '最近 7 天', value: '7d' },
];

// DashboardPage:管理台总览大盘（设计稿第四节第 1 页）。
// 点击统计卡跳转任务列表并带上对应筛选，时间范围切换会联动刷新全部图表和统计。
export function DashboardPage() {
  const navigate = useNavigate();
  const perm = usePermission();
  const [range, setRange] = useState<DashboardRange>('24h');
  const [tenantId, setTenantId] = useState<string | undefined>(undefined);

  const params = { range, tenant_id: perm.isPlatformAdmin ? tenantId : undefined };

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ['dashboard-summary', params],
    queryFn: () => dashboardApi.summary(params),
  });
  const { data: taskStats } = useQuery({
    queryKey: ['dashboard-task-stats', params],
    queryFn: () => dashboardApi.taskStats(params),
  });
  const { data: costStats } = useQuery({
    queryKey: ['dashboard-cost-stats', params],
    queryFn: () => dashboardApi.costStats(params),
  });
  const { data: riskStats } = useQuery({
    queryKey: ['dashboard-risk-stats', params],
    queryFn: () => dashboardApi.riskStats(params),
  });
  const { data: health } = useQuery({
    queryKey: ['dashboard-health', params],
    queryFn: () => dashboardApi.systemHealth(params),
  });

  const trendOption = {
    tooltip: { trigger: 'axis' },
    legend: { data: ['created', 'completed', 'failed', 'tokens'] },
    xAxis: { type: 'category', data: (taskStats?.buckets ?? []).map((b) => b.time) },
    yAxis: [{ type: 'value', name: '任务数' }, { type: 'value', name: 'tokens' }],
    series: [
      { name: 'created', type: 'line', data: (taskStats?.buckets ?? []).map((b) => b.created) },
      { name: 'completed', type: 'line', data: (taskStats?.buckets ?? []).map((b) => b.completed) },
      { name: 'failed', type: 'line', data: (taskStats?.buckets ?? []).map((b) => b.failed) },
      { name: 'tokens', type: 'bar', yAxisIndex: 1, data: (taskStats?.buckets ?? []).map((b) => b.tokens) },
    ],
  };

  const distOption = {
    tooltip: { trigger: 'item' },
    series: [
      {
        type: 'pie',
        radius: '60%',
        data: (taskStats?.status_distribution ?? []).map((d) => ({ name: d.status, value: d.count })),
      },
    ],
  };

  const riskOption = {
    tooltip: { trigger: 'item' },
    xAxis: { type: 'category', data: (riskStats?.risk_distribution ?? []).map((d) => d.risk_level) },
    yAxis: { type: 'value' },
    series: [{ type: 'bar', data: (riskStats?.risk_distribution ?? []).map((d) => d.count) }],
  };

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Card>
        <Space>
          <Segmented options={RANGE_OPTIONS} value={range} onChange={(v) => setRange(v as DashboardRange)} />
          {perm.isPlatformAdmin && (
            <Input placeholder="按 tenant_id 过滤（可选）" style={{ width: 200 }} onPressEnter={(e) => setTenantId((e.target as HTMLInputElement).value || undefined)} />
          )}
        </Space>
      </Card>

      <Row gutter={16}>
        <Col span={5}>
          <Card loading={summaryLoading} onClick={() => navigate('/tasks')} hoverable>
            <Statistic title="总任务数" value={summary?.total_tasks ?? 0} />
          </Card>
        </Col>
        <Col span={5}>
          <Card loading={summaryLoading} onClick={() => navigate('/tasks?status=RUNNING')} hoverable>
            <Statistic title="运行中" value={summary?.running ?? 0} valueStyle={{ color: '#1677ff' }} />
          </Card>
        </Col>
        <Col span={5}>
          <Card loading={summaryLoading} onClick={() => navigate('/approvals')} hoverable>
            <Statistic title="等待审批" value={summary?.waiting_approval ?? 0} valueStyle={{ color: '#fa8c16' }} />
          </Card>
        </Col>
        <Col span={5}>
          <Card loading={summaryLoading} onClick={() => navigate('/tasks?status=FAILED')} hoverable>
            <Statistic title="失败任务" value={summary?.failed ?? 0} valueStyle={{ color: '#ff4d4f' }} />
          </Card>
        </Col>
        <Col span={4}>
          <Card loading={summaryLoading}>
            <Statistic title="成功率" value={formatPercent(summary?.success_rate)} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16}>
        <Col span={8}>
          <Card size="small">
            <Statistic title="今日 Token 消耗" value={formatNumber(costStats?.tokens_today)} />
          </Card>
        </Col>
        <Col span={8}>
          <Card size="small">
            <Statistic title="今日 LLM 成本" value={formatCost(costStats?.cost_today_usd)} />
          </Card>
        </Col>
        <Col span={8}>
          <Card size="small">
            <Statistic title="平均单任务成本" value={formatCost(costStats?.avg_cost_per_task_usd)} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16}>
        <Col span={12}>
          <Card title="Token 消耗 / 任务趋势">
            <ReactECharts option={trendOption} style={{ height: 280 }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card title="任务状态分布">
            <ReactECharts option={distOption} style={{ height: 280 }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card title="工具调用风险分布">
            <ReactECharts option={riskOption} style={{ height: 280 }} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16}>
        <Col span={8}>
          <Card title="风险概览">
            <Space direction="vertical">
              <Typography.Text>待审批：{riskStats?.pending_approvals ?? 0}</Typography.Text>
              <Typography.Text>被拒绝工具调用：{riskStats?.rejected_tool_calls ?? 0}</Typography.Text>
              <Typography.Text>触发 loop detector 任务：{riskStats?.loop_detected_tasks ?? 0}</Typography.Text>
              <Typography.Text>token 超限任务：{riskStats?.token_limit_tasks ?? 0}</Typography.Text>
              <Typography.Text>step 超限任务：{riskStats?.step_limit_tasks ?? 0}</Typography.Text>
            </Space>
          </Card>
        </Col>
        <Col span={16}>
          <Card title="系统健康">
            <Space direction="vertical" style={{ width: '100%' }}>
              {(health?.services ?? []).map((s) => (
                <Space key={s.name} style={{ width: '100%', justifyContent: 'space-between' }}>
                  <Typography.Text onClick={() => navigate('/observability/metrics')} style={{ cursor: 'pointer' }}>
                    {s.name}
                  </Typography.Text>
                  <Tag color={s.status === 'healthy' ? 'green' : s.status === 'degraded' ? 'orange' : 'red'}>{s.status}</Tag>
                </Space>
              ))}
              <Typography.Text type="secondary">
                Queue backlog: {health?.queue_backlog ?? '-'} · Active workers: {health?.active_workers ?? '-'} · 检测时间：
                {formatDateTime(health?.checked_at)}
              </Typography.Text>
            </Space>
          </Card>
        </Col>
      </Row>

      {!summary && !summaryLoading && <Alert type="warning" message="Dashboard 数据暂不可用，请确认后端 /dashboard 系列接口已就绪" />}

      <Row gutter={16}>
        <Col span={12}>
          <Card title="最近失败任务">
            <Typography.Link onClick={() => navigate('/tasks?status=FAILED')}>查看失败任务列表 →</Typography.Link>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="最近待审批">
            <Typography.Link onClick={() => navigate('/approvals')}>查看待审批列表 →</Typography.Link>
          </Card>
        </Col>
      </Row>
    </Space>
  );
}
