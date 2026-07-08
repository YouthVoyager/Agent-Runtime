const API_BASE = process.env.API_BASE || 'http://localhost:8080';
const TOKEN = process.env.STABLEAGENT_JWT || process.env.JWT || '';

if (!TOKEN) {
  throw new Error('缺少 STABLEAGENT_JWT，请先运行: node scripts/generate-jwt.mjs');
}

// api 调用 StableAgent API 并返回 data。
async function api(path, options = {}) {
  const response = await fetch(`${API_BASE}${path}`, {
    method: options.method || 'GET',
    headers: {
      Authorization: `Bearer ${TOKEN}`,
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : {};
  if (!response.ok) {
    throw new Error(`${options.method || 'GET'} ${path} failed: ${response.status} ${payload.error?.message || text}`);
  }
  return payload.data;
}

// createTask 创建指定目标的任务。
async function createTask(goal) {
  return api('/api/v1/tasks', {
    method: 'POST',
    body: {
      goal,
      client_request_id: `e2e_${Date.now()}_${Math.random().toString(16).slice(2)}`,
      budget: {
        max_steps: 50,
        max_tokens: 200000,
        max_tool_calls: 40,
        max_cost_usd: 10,
      },
      constraints: { language: 'zh-CN' },
    },
  });
}

// waitTask 等待任务进入目标状态之一。
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

// sleep 等待指定毫秒。
function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// main 执行本地闭环端到端验证。
async function main() {
  const normal = await createTask('生成 StableAgent 本地生产闭环报告');
  const normalDone = await waitTask(normal.task_id, ['SUCCEEDED']);
  const normalEvents = await api(`/api/v1/tasks/${normal.task_id}/events`);
  const normalArtifacts = await api(`/api/v1/tasks/${normal.task_id}/artifacts`);
  console.log(`普通任务完成: ${normalDone.task_id}, events=${normalEvents.items.length}, artifacts=${normalArtifacts.items.length}`);

  const high = await createTask('高风险审批发送邮件，验证 Tool Gateway 人工审批流程');
  await waitTask(high.task_id, ['WAITING_APPROVAL']);
  const calls = await api(`/api/v1/tasks/${high.task_id}/tool-calls`);
  const pending = calls.items.find((item) => item.approval_status === 'PENDING');
  if (!pending) {
    throw new Error('高风险任务未产生待审批工具调用');
  }
  await api(`/api/v1/tool-calls/${pending.call_id}/approve`, { method: 'POST', body: { comment: 'e2e approve' } });
  const highDone = await waitTask(high.task_id, ['SUCCEEDED']);
  console.log(`高风险审批任务完成: ${highDone.task_id}, call=${pending.call_id}`);

  const cancel = await createTask('高风险审批发送邮件，用于取消流程验证');
  await waitTask(cancel.task_id, ['WAITING_APPROVAL']);
  await api(`/api/v1/tasks/${cancel.task_id}/cancel`, { method: 'POST' });
  const canceled = await waitTask(cancel.task_id, ['CANCELED']);
  console.log(`取消任务完成: ${canceled.task_id}`);

  const critical = await createTask('危险 critical 管理操作，用于失败和恢复流程验证');
  const failed = await waitTask(critical.task_id, ['FAILED']);
  await api(`/api/v1/tasks/${failed.task_id}/resume`, { method: 'POST' });
  const failedAgain = await waitTask(failed.task_id, ['FAILED']);
  const checkpoints = await api(`/api/v1/tasks/${failed.task_id}/checkpoints`);
  if (checkpoints.items.length === 0) {
    throw new Error('失败恢复任务未产生 checkpoint');
  }
  console.log(`失败恢复验证完成: ${failedAgain.task_id}, checkpoints=${checkpoints.items.length}`);
}

await main();
