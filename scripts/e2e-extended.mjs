import crypto from 'node:crypto';
import { execSync } from 'node:child_process';

const API_BASE = process.env.API_BASE || 'http://localhost:8080';
const JWT_SECRET = process.env.JWT_SECRET || 'local-dev-secret';
const COMPOSE = 'docker compose -f deploy/docker-compose.yml';

// signJWT 生成与 api-service 一致的 HS256 JWT。
function signJWT({ tenantID, userID, role }) {
  const base64url = (input) => Buffer.from(input).toString('base64url');
  const now = Math.floor(Date.now() / 1000);
  const header = base64url(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const payload = base64url(JSON.stringify({ tenant_id: tenantID, user_id: userID, role, iat: now, exp: now + 3600 }));
  const signature = crypto.createHmac('sha256', JWT_SECRET).update(`${header}.${payload}`).digest('base64url');
  return `${header}.${payload}.${signature}`;
}

const TOKEN = signJWT({ tenantID: 'tenant_local', userID: 'admin_local', role: 'admin' });
const OTHER_TOKEN = signJWT({ tenantID: 'tenant_other', userID: 'user_other', role: 'admin' });

// api 调用 StableAgent API,返回 { status, payload }，不抛出 HTTP 错误。
async function apiRaw(path, options = {}) {
  const response = await fetch(`${API_BASE}${path}`, {
    method: options.method || 'GET',
    headers: {
      Authorization: `Bearer ${options.token || TOKEN}`,
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...(options.headers || {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : {};
  return { status: response.status, payload };
}

// api 调用 API 并断言成功,返回 data。
async function api(path, options = {}) {
  const { status, payload } = await apiRaw(path, options);
  if (status >= 300) {
    throw new Error(`${options.method || 'GET'} ${path} failed: ${status} ${payload.error?.message || ''}`);
  }
  return payload.data;
}

// createTask 创建任务,可覆盖 budget 与 client_request_id。
async function createTask(goal, overrides = {}) {
  return api('/api/v1/tasks', {
    method: 'POST',
    body: {
      goal,
      client_request_id: overrides.clientRequestID || `e2e_ext_${Date.now()}_${Math.random().toString(16).slice(2)}`,
      budget: overrides.budget || { max_steps: 50, max_tokens: 200000, max_tool_calls: 40, max_cost_usd: 10 },
      constraints: { language: 'zh-CN' },
    },
  });
}

// waitTask 轮询等待任务进入目标状态之一。
async function waitTask(taskID, statuses, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const task = await api(`/api/v1/tasks/${taskID}`);
    if (statuses.includes(task.status)) {
      return task;
    }
    await sleep(700);
  }
  throw new Error(`任务 ${taskID} 未在 ${timeoutMs}ms 内进入状态: ${statuses.join(',')}`);
}

// waitPendingCall 等待任务出现待审批工具调用。
async function waitPendingCall(taskID) {
  await waitTask(taskID, ['WAITING_APPROVAL']);
  const calls = await api(`/api/v1/tasks/${taskID}/tool-calls`);
  const pending = calls.items.find((item) => item.approval_status === 'PENDING');
  if (!pending) {
    throw new Error(`任务 ${taskID} 没有待审批工具调用`);
  }
  return pending;
}

// psql 通过 docker compose 在业务库执行 SQL,SQL 走 stdin 避免 shell 转义问题。
function psql(sql) {
  return execSync(`${COMPOSE} exec -T postgres psql -U stableagent -d stableagent -v ON_ERROR_STOP=1`, {
    encoding: 'utf8',
    input: sql,
  });
}

// sleep 等待指定毫秒。
function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// assert 校验条件,失败时抛错。
function assert(condition, message) {
  if (!condition) {
    throw new Error(`断言失败: ${message}`);
  }
}

// scenarioCrossTenant 校验多租户隔离:其他租户读取/取消任务必须 404。
async function scenarioCrossTenant() {
  const task = await createTask('生成跨租户隔离验证报告');
  await waitTask(task.task_id, ['SUCCEEDED']);
  const read = await apiRaw(`/api/v1/tasks/${task.task_id}`, { token: OTHER_TOKEN });
  assert(read.status === 404, `跨租户读取应 404,实际 ${read.status}`);
  const cancel = await apiRaw(`/api/v1/tasks/${task.task_id}/cancel`, { method: 'POST', token: OTHER_TOKEN });
  assert(cancel.status === 404, `跨租户取消应 404,实际 ${cancel.status}`);
  const events = await apiRaw(`/api/v1/tasks/${task.task_id}/events`, { token: OTHER_TOKEN });
  assert(events.status === 404, `跨租户 timeline 应 404,实际 ${events.status}`);
  console.log(`场景1 跨租户隔离通过: ${task.task_id}`);
}

// scenarioCreateIdempotency 校验 client_request_id 重复创建返回同一任务。
async function scenarioCreateIdempotency() {
  const clientRequestID = `e2e_ext_idem_${Date.now()}`;
  const first = await createTask('幂等创建验证', { clientRequestID });
  const second = await createTask('幂等创建验证', { clientRequestID });
  assert(first.task_id === second.task_id, `幂等创建返回不同任务: ${first.task_id} vs ${second.task_id}`);
  console.log(`场景2 创建幂等通过: ${first.task_id}`);
}

// scenarioApprovalIdempotency 校验重复审批只生效一次、冲突决策被拒绝。
async function scenarioApprovalIdempotency() {
  const task = await createTask('高风险审批发送邮件,验证重复审批幂等');
  const pending = await waitPendingCall(task.task_id);
  const first = await api(`/api/v1/tool-calls/${pending.call_id}/approve`, { method: 'POST', body: { comment: '第一次' } });
  assert(first.approval_status === 'APPROVED', `首次审批应 APPROVED,实际 ${first.approval_status}`);
  const second = await api(`/api/v1/tool-calls/${pending.call_id}/approve`, { method: 'POST', body: { comment: '第二次' } });
  assert(second.approval_status === 'APPROVED', `重复审批应幂等返回 APPROVED,实际 ${second.approval_status}`);
  const conflict = await apiRaw(`/api/v1/tool-calls/${pending.call_id}/reject`, { method: 'POST', body: { comment: '迟到拒绝' } });
  assert(conflict.status === 409, `审批后再拒绝应 409,实际 ${conflict.status}`);
  await waitTask(task.task_id, ['SUCCEEDED']);
  console.log(`场景3 审批幂等与冲突通过: ${pending.call_id}`);
}

// scenarioApprovalReject 校验审批拒绝后任务失败且工具调用带拒绝错误码。
async function scenarioApprovalReject() {
  const task = await createTask('高风险审批发送邮件,验证审批拒绝');
  const pending = await waitPendingCall(task.task_id);
  const decision = await api(`/api/v1/tool-calls/${pending.call_id}/reject`, { method: 'POST', body: { comment: '不允许发送' } });
  assert(decision.approval_status === 'REJECTED', `拒绝后审批状态应 REJECTED,实际 ${decision.approval_status}`);
  await waitTask(task.task_id, ['FAILED']);
  const calls = await api(`/api/v1/tasks/${task.task_id}/tool-calls`);
  const rejected = calls.items.find((item) => item.call_id === pending.call_id);
  assert(rejected.status === 'FAILED', `被拒工具调用应 FAILED,实际 ${rejected.status}`);
  assert(rejected.error_code === 'TOOL_APPROVAL_REJECTED', `错误码应 TOOL_APPROVAL_REJECTED,实际 ${rejected.error_code}`);
  console.log(`场景4 审批拒绝通过: ${task.task_id}`);
}

// scenarioCancelIdempotency 校验重复取消幂等返回当前状态。
async function scenarioCancelIdempotency() {
  const task = await createTask('高风险审批发送邮件,验证取消幂等');
  await waitPendingCall(task.task_id);
  const first = await api(`/api/v1/tasks/${task.task_id}/cancel`, { method: 'POST' });
  assert(['CANCELING', 'CANCELED'].includes(first.status), `首次取消应进入取消流程,实际 ${first.status}`);
  const canceled = await waitTask(task.task_id, ['CANCELED']);
  const second = await api(`/api/v1/tasks/${task.task_id}/cancel`, { method: 'POST' });
  assert(second.status === 'CANCELED', `终态重复取消应幂等返回 CANCELED,实际 ${second.status}`);
  console.log(`场景5 取消幂等通过: ${canceled.task_id}`);
}

// scenarioResumeGuard 校验等待审批的任务不允许直接 resume。
async function scenarioResumeGuard() {
  const task = await createTask('高风险审批发送邮件,验证 resume 状态校验');
  const pending = await waitPendingCall(task.task_id);
  const resume = await apiRaw(`/api/v1/tasks/${task.task_id}/resume`, { method: 'POST' });
  assert(resume.status === 409, `WAITING_APPROVAL 任务 resume 应 409,实际 ${resume.status}`);
  await api(`/api/v1/tool-calls/${pending.call_id}/reject`, { method: 'POST', body: { comment: '清理' } });
  await waitTask(task.task_id, ['FAILED']);
  console.log(`场景6 resume 状态校验通过: ${task.task_id}`);
}

// scenarioSSE 校验 SSE 补发与 Last-Event-ID 重连语义。
async function scenarioSSE() {
  const task = await createTask('生成 SSE 验证报告');
  await waitTask(task.task_id, ['SUCCEEDED']);
  const page = await api(`/api/v1/tasks/${task.task_id}/events`);
  assert(page.items.length >= 5, `完成任务事件数量过少: ${page.items.length}`);
  const ids = page.items.map((item) => item.event_id);
  for (let i = 1; i < ids.length; i += 1) {
    assert(ids[i] > ids[i - 1], `event_id 必须单调递增: ${ids[i - 1]} -> ${ids[i]}`);
  }
  const midEventID = ids[Math.floor(ids.length / 2)];

  const controller = new AbortController();
  const response = await fetch(`${API_BASE}/api/v1/tasks/${task.task_id}/events/stream`, {
    headers: { Authorization: `Bearer ${TOKEN}`, 'Last-Event-ID': String(midEventID) },
    signal: controller.signal,
  });
  assert(response.status === 200, `SSE 连接应 200,实际 ${response.status}`);
  assert((response.headers.get('content-type') || '').includes('text/event-stream'), 'SSE Content-Type 不正确');

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  const received = [];
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline && received.length < ids.length) {
    const { value, done } = await Promise.race([
      reader.read(),
      sleep(1200).then(() => ({ value: undefined, done: false })),
    ]);
    if (done) {
      break;
    }
    if (!value) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    for (const match of buffer.matchAll(/^id: (\d+)$/gm)) {
      const eventID = Number(match[1]);
      if (!received.includes(eventID)) {
        received.push(eventID);
      }
    }
  }
  controller.abort();
  assert(received.length > 0, 'SSE 未补发任何事件');
  assert(received.every((eventID) => eventID > midEventID), `补发事件必须大于 Last-Event-ID ${midEventID}: ${received.join(',')}`);
  const expected = ids.filter((eventID) => eventID > midEventID);
  assert(received.length === expected.length, `补发数量不符: got ${received.length} want ${expected.length}`);
  console.log(`场景7 SSE Last-Event-ID 通过: 补发 ${received.length} 条`);
}

// scenarioBudgetExceeded 校验预算超限时任务停止并记录 BUDGET_EXCEEDED。
async function scenarioBudgetExceeded() {
  const task = await createTask('高风险审批发送邮件,验证预算超限停止');
  const pending = await waitPendingCall(task.task_id);
  psql(`update agent_tasks set budget = '{"max_steps":1,"max_tokens":200000,"max_tool_calls":40,"max_cost_usd":10}'::jsonb, budget_usage = '{"used_steps":1,"used_tokens":10,"used_tool_calls":0,"used_cost_usd":0.01}'::jsonb where task_id = '${task.task_id}';`);
  await api(`/api/v1/tool-calls/${pending.call_id}/approve`, { method: 'POST', body: { comment: '触发预算检查' } });
  await waitTask(task.task_id, ['FAILED']);
  const events = await api(`/api/v1/tasks/${task.task_id}/events?limit=100`);
  const failedEvent = events.items.find((item) => item.type === 'TASK_FAILED' && item.input?.error_code === 'BUDGET_EXCEEDED');
  assert(failedEvent, '未找到 BUDGET_EXCEEDED 的 TASK_FAILED 事件');
  console.log(`场景8 预算超限通过: ${task.task_id}`);
}

// scenarioTenantPolicyDeny 校验租户 DENY 策略拦截工具调用。
async function scenarioTenantPolicyDeny() {
  psql(`insert into tenant_tool_policies (policy_id, tenant_id, tool_name, effect, max_risk_level, require_approval, config) values ('policy_e2e_deny', 'tenant_local', 'read_document', 'DENY', 'LOW', false, '{}') on conflict (tenant_id, tool_name) do update set effect = 'DENY';`);
  try {
    const task = await createTask('读取设计方案文档,验证租户策略 DENY');
    await waitTask(task.task_id, ['FAILED']);
    const events = await api(`/api/v1/tasks/${task.task_id}/events?limit=100`);
    const failedEvent = events.items.find((item) => item.type === 'TASK_FAILED' && item.input?.error_code === 'TOOL_CALL_FAILED');
    assert(failedEvent, '未找到策略拦截导致的 TASK_FAILED 事件');
    console.log(`场景9 租户策略 DENY 通过: ${task.task_id}`);
  } finally {
    psql(`delete from tenant_tool_policies where policy_id = 'policy_e2e_deny';`);
  }
}

// scenarioWorkerRestart 校验 worker 重启后任务不丢且副作用工具不重复执行。
async function scenarioWorkerRestart() {
  const task = await createTask('生成 worker 重启恢复验证报告');
  execSync(`${COMPOSE} restart runtime-worker`, { encoding: 'utf8', stdio: 'pipe' });
  const done = await waitTask(task.task_id, ['SUCCEEDED'], 90000);
  const calls = await api(`/api/v1/tasks/${task.task_id}/tool-calls`);
  const artifactCalls = calls.items.filter((item) => item.tool_name === 'write_artifact');
  assert(artifactCalls.length === 1, `write_artifact 应恰好执行一次,实际 ${artifactCalls.length} 次`);
  assert(artifactCalls[0].status === 'SUCCEEDED', `工具调用应 SUCCEEDED,实际 ${artifactCalls[0].status}`);
  console.log(`场景10 worker 重启恢复通过: ${done.task_id}`);
}

// main 顺序执行全部扩展场景,单个场景失败不阻断后续场景,最后统一汇报。
async function main() {
  const scenarios = [
    ['场景1 跨租户隔离', scenarioCrossTenant],
    ['场景2 创建幂等', scenarioCreateIdempotency],
    ['场景3 审批幂等与冲突', scenarioApprovalIdempotency],
    ['场景4 审批拒绝', scenarioApprovalReject],
    ['场景5 取消幂等', scenarioCancelIdempotency],
    ['场景6 resume 状态校验', scenarioResumeGuard],
    ['场景7 SSE Last-Event-ID', scenarioSSE],
    ['场景8 预算超限', scenarioBudgetExceeded],
    ['场景9 租户策略 DENY', scenarioTenantPolicyDeny],
    ['场景10 worker 重启恢复', scenarioWorkerRestart],
  ];
  const failures = [];
  for (const [name, run] of scenarios) {
    try {
      await run();
    } catch (error) {
      failures.push({ name, message: error.message });
      console.error(`${name} 失败: ${error.message}`);
    }
  }
  if (failures.length > 0) {
    console.error(`e2e-extended 有 ${failures.length}/${scenarios.length} 个场景失败:`);
    for (const failure of failures) {
      console.error(`  - ${failure.name}: ${failure.message}`);
    }
    process.exit(1);
  }
  console.log('e2e-extended 全部场景通过');
}

await main();
