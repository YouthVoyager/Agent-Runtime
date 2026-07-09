import { useState } from 'react';
import { Card, Col, Row, Segmented, Statistic } from 'antd';
import { useQuery } from '@tanstack/react-query';
import ReactECharts from 'echarts-for-react';
import { observabilityApi } from '@/api/observability';
import type { DashboardRange } from '@/types/dashboard';
import { formatCost, formatDurationMs, formatNumber } from '@/utils/format';

const RANGE_OPTIONS: { label: string; value: DashboardRange }[] = [
  { label: '最近 1 小时', value: '1h' },
  { label: '最近 24 小时', value: '24h' },
  { label: '最近 7 天', value: '7d' },
];

// MetricsPage:平台指标监控（设计稿第四节第 11 页）。
export function MetricsPage() {
  const [range, setRange] = useState<DashboardRange>('24h');
  const { data, isLoading } = useQuery({ queryKey: ['metrics', range], queryFn: () => observabilityApi.metrics(range) });

  const modelOption = {
    tooltip: { trigger: 'item' },
    series: [{ type: 'pie', radius: '60%', data: (data?.llm.model_distribution ?? []).map((d) => ({ name: d.model, value: d.count })) }],
  };
  const riskOption = {
    tooltip: { trigger: 'item' },
    xAxis: { type: 'category', data: (data?.tools.risk_distribution ?? []).map((d) => d.risk_level) },
    yAxis: { type: 'value' },
    series: [{ type: 'bar', data: (data?.tools.risk_distribution ?? []).map((d) => d.count) }],
  };

  return (
    <Card title="Metrics" loading={isLoading}>
      <Segmented options={RANGE_OPTIONS} value={range} onChange={(v) => setRange(v as DashboardRange)} style={{ marginBottom: 16 }} />

      <Row gutter={16}>
        <Col span={4}><Card size="small"><Statistic title="任务创建" value={formatNumber(data?.tasks.created_total)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="任务完成" value={formatNumber(data?.tasks.completed_total)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="任务失败" value={formatNumber(data?.tasks.failed_total)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="运行中" value={formatNumber(data?.tasks.running)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="等待审批" value={formatNumber(data?.tasks.waiting_approval)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="P95 耗时" value={data?.tasks.duration_p95_s ? `${data.tasks.duration_p95_s}s` : '-'} /></Card></Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={4}><Card size="small"><Statistic title="LLM 调用" value={formatNumber(data?.llm.calls_total)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="LLM 错误" value={formatNumber(data?.llm.errors_total)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="P95 延迟" value={formatDurationMs(data?.llm.latency_p95_ms)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="Token 总量" value={formatNumber(data?.llm.tokens_total)} /></Card></Col>
        <Col span={4}><Card size="small"><Statistic title="LLM 成本" value={formatCost(data?.llm.cost_total_usd)} /></Card></Col>
        <Col span={4}>
          <Card size="small">
            <Statistic title="Worker" value={data?.workers.active_workers ?? '-'} suffix={`/ backlog ${data?.workers.workflow_backlog ?? '-'}`} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="Model 使用分布">
            <ReactECharts option={modelOption} style={{ height: 280 }} />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="工具风险分布">
            <ReactECharts option={riskOption} style={{ height: 280 }} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={6}><Card size="small"><Statistic title="工具调用总数" value={formatNumber(data?.tools.calls_total)} /></Card></Col>
        <Col span={6}><Card size="small"><Statistic title="工具错误" value={formatNumber(data?.tools.errors_total)} /></Card></Col>
        <Col span={6}><Card size="small"><Statistic title="需审批次数" value={formatNumber(data?.tools.approval_required_total)} /></Card></Col>
        <Col span={6}><Card size="small"><Statistic title="被拒绝次数" value={formatNumber(data?.tools.approval_rejected_total)} /></Card></Col>
      </Row>
    </Card>
  );
}
