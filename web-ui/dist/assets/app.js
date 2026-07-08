import React, { useEffect, useRef, useState } from 'https://esm.sh/react@19.2.3';
import { createRoot } from 'https://esm.sh/react-dom@19.2.3/client';

const h = React.createElement;
const TOKEN_STORAGE_KEY = 'stableagent_jwt';

const DEFAULT_BUDGET = {
  max_steps: 50,
  max_tokens: 200000,
  max_tool_calls: 40,
  max_cost_usd: 10,
};

const EVENT_META = {
  TASK_CREATED: { label: '任务已创建', icon: '+', tone: 'blue' },
  TASK_STARTED: { label: '任务开始', icon: '▶', tone: 'amber' },
  STEP_STARTED: { label: '步骤开始', icon: '▶', tone: 'amber' },
  LLM_CALL_COMPLETED: { label: '模型完成', icon: '#', tone: 'blue' },
  TOOL_APPROVAL_REQUIRED: { label: '等待审批', icon: '!', tone: 'amber' },
  TOOL_APPROVAL_DECIDED: { label: '审批完成', icon: '✓', tone: 'green' },
  TOOL_CALL_COMPLETED: { label: '工具完成', icon: '✓', tone: 'green' },
  CHECKPOINT_CREATED: { label: 'Checkpoint', icon: '◇', tone: 'blue' },
  ARTIFACT_SAVED: { label: '产物保存', icon: '◆', tone: 'green' },
  TASK_COMPLETED: { label: '任务完成', icon: '✓', tone: 'green' },
  TASK_FAILED: { label: '任务失败', icon: '!', tone: 'red' },
  TASK_CANCELLED: { label: '任务取消', icon: '×', tone: 'red' },
};

// App 渲染 StableAgent 本地生产闭环工作台。
function App() {
  const [token, setToken] = useState(() => localStorage.getItem(TOKEN_STORAGE_KEY) || '');
  const [goal, setGoal] = useState('生成 StableAgent 本地生产闭环报告');
  const [taskId, setTaskId] = useState('');
  const [tasks, setTasks] = useState([]);
  const [task, setTask] = useState(null);
  const [events, setEvents] = useState([]);
  const [toolCalls, setToolCalls] = useState([]);
  const [checkpoints, setCheckpoints] = useState([]);
  const [artifacts, setArtifacts] = useState([]);
  const [activeTab, setActiveTab] = useState('timeline');
  const [connection, setConnection] = useState('idle');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const abortRef = useRef(null);
  const reconnectRef = useRef(null);
  const lastEventIdRef = useRef(0);

  useEffect(() => {
    localStorage.setItem(TOKEN_STORAGE_KEY, token);
  }, [token]);

  useEffect(() => {
    if (token) {
      loadTasks();
    }
  }, [token]);

  useEffect(() => {
    stopStream();
    lastEventIdRef.current = 0;
    setEvents([]);
    setTask(null);
    setToolCalls([]);
    setCheckpoints([]);
    setArtifacts([]);
    if (!token || !taskId) {
      setConnection('idle');
      return () => stopStream();
    }
    let cancelled = false;
    loadTaskData(cancelled).then(() => {
      if (!cancelled) {
        connectStream();
      }
    });
    return () => {
      cancelled = true;
      stopStream();
    };
  }, [token, taskId]);

  // loadTasks 拉取当前用户可见任务列表。
  async function loadTasks() {
    if (!token) {
      return;
    }
    try {
      const data = await apiFetch('/api/v1/tasks?page_size=20', { token });
      setTasks(data.items || []);
      if (!taskId && data.items?.[0]?.task_id) {
        setTaskId(data.items[0].task_id);
      }
    } catch (err) {
      setError(err.message);
    }
  }

  // createTask 创建任务并切换到新任务详情。
  async function createTask(event) {
    event.preventDefault();
    if (!token || !goal.trim()) {
      setError('JWT 和目标不能为空');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const data = await apiFetch('/api/v1/tasks', {
        token,
        method: 'POST',
        body: {
          goal: goal.trim(),
          client_request_id: `ui_${Date.now()}`,
          budget: DEFAULT_BUDGET,
          constraints: { language: 'zh-CN' },
        },
      });
      setTaskId(data.task_id);
      await loadTasks();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  // loadTaskData 拉取任务详情和关联资源。
  async function loadTaskData(cancelled) {
    setError('');
    try {
      const [detail, timeline, calls, ckpts, files] = await Promise.all([
        apiFetch(`/api/v1/tasks/${encodeURIComponent(taskId)}`, { token }),
        apiFetch(`/api/v1/tasks/${encodeURIComponent(taskId)}/events?limit=100`, { token }),
        apiFetch(`/api/v1/tasks/${encodeURIComponent(taskId)}/tool-calls`, { token }),
        apiFetch(`/api/v1/tasks/${encodeURIComponent(taskId)}/checkpoints`, { token }),
        apiFetch(`/api/v1/tasks/${encodeURIComponent(taskId)}/artifacts`, { token }),
      ]);
      if (cancelled) {
        return;
      }
      setTask(detail);
      setEvents(timeline.items || []);
      setToolCalls(calls.items || []);
      setCheckpoints(ckpts.items || []);
      setArtifacts(files.items || []);
      lastEventIdRef.current = timeline.next_after_event_id || 0;
    } catch (err) {
      if (!cancelled) {
        setError(err.message);
      }
    }
  }

  // refreshSelected 刷新当前任务的全部详情。
  async function refreshSelected() {
    if (!taskId) {
      return;
    }
    await loadTaskData(false);
    await loadTasks();
  }

  // mutateTask 调用 cancel/resume 等任务状态接口。
  async function mutateTask(action) {
    if (!taskId) {
      return;
    }
    setBusy(true);
    setError('');
    try {
      await apiFetch(`/api/v1/tasks/${encodeURIComponent(taskId)}/${action}`, { token, method: 'POST' });
      await refreshSelected();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  // decideToolCall 处理工具审批通过或拒绝。
  async function decideToolCall(callId, decision) {
    setBusy(true);
    setError('');
    try {
      await apiFetch(`/api/v1/tool-calls/${encodeURIComponent(callId)}/${decision}`, {
        token,
        method: 'POST',
        body: { comment: decision === 'approve' ? '本地验证通过' : '本地验证拒绝' },
      });
      await refreshSelected();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  // connectStream 使用 fetch 建立带 Authorization header 的 SSE 连接。
  async function connectStream() {
    if (!token || !taskId) {
      return;
    }
    const controller = new AbortController();
    abortRef.current = controller;
    setConnection('connecting');
    try {
      const headers = authHeaders(token);
      if (lastEventIdRef.current > 0) {
        headers['Last-Event-ID'] = String(lastEventIdRef.current);
      }
      const response = await fetch(`/api/v1/tasks/${encodeURIComponent(taskId)}/events/stream`, {
        headers,
        signal: controller.signal,
      });
      if (!response.ok) {
        await readAPIResponse(response);
      }
      setConnection('connected');
      await readSSE(response, (event) => {
        lastEventIdRef.current = Math.max(lastEventIdRef.current, Number(event.event_id || 0));
        setEvents((current) => mergeEvents(current, [event]));
        refreshSelected();
      });
      if (!controller.signal.aborted) {
        scheduleReconnect();
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        setError(err.message);
        scheduleReconnect();
      }
    }
  }

  // scheduleReconnect 在事件流断开后延迟重连。
  function scheduleReconnect() {
    setConnection('reconnecting');
    clearTimeout(reconnectRef.current);
    reconnectRef.current = setTimeout(() => connectStream(), 1500);
  }

  // stopStream 关闭当前事件流。
  function stopStream() {
    clearTimeout(reconnectRef.current);
    reconnectRef.current = null;
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
  }

  return h(
    'main',
    { className: 'app-shell' },
    h(Sidebar, { token, setToken, goal, setGoal, createTask, busy, tasks, taskId, setTaskId, loadTasks }),
    h(
      'section',
      { className: 'workspace' },
      h(
        'header',
        { className: 'workspace-header' },
        h('div', null, h('p', null, 'Task'), h('h2', null, taskId || '未选择任务')),
        h(
          'div',
          { className: 'actions-row' },
          h('span', { className: `status-pill ${task?.status || 'idle'}` }, task?.status || 'IDLE'),
          h('button', { type: 'button', onClick: () => mutateTask('cancel'), disabled: busy || !taskId }, h(Icon, { name: '×' }), '取消'),
          h('button', { type: 'button', onClick: () => mutateTask('resume'), disabled: busy || !taskId }, h(Icon, { name: '↻' }), '恢复'),
          h('button', { type: 'button', onClick: refreshSelected, disabled: busy || !taskId }, h(Icon, { name: '↺', spin: busy }), '刷新'),
        ),
      ),
      error ? h('div', { className: 'error-strip' }, h(Icon, { name: '!' }), h('span', null, error)) : null,
      task ? h(TaskSummary, { task, connection }) : null,
      h(Tabs, { activeTab, setActiveTab }),
      activeTab === 'timeline' ? h(Timeline, { events }) : null,
      activeTab === 'approvals' ? h(ToolCalls, { toolCalls, decideToolCall, busy }) : null,
      activeTab === 'checkpoints' ? h(ResourceList, { items: checkpoints, title: 'Checkpoints' }) : null,
      activeTab === 'artifacts' ? h(ResourceList, { items: artifacts, title: 'Artifacts' }) : null,
    ),
  );
}

// Sidebar 渲染左侧创建任务和任务列表。
function Sidebar({ token, setToken, goal, setGoal, createTask, busy, tasks, taskId, setTaskId, loadTasks }) {
  return h(
    'aside',
    { className: 'sidebar' },
    h('div', { className: 'brand-row' }, h(Icon, { name: '◉' }), h('div', null, h('h1', null, 'StableAgent'), h('span', null, '生产级通用 Agent 执行平台'))),
    h(
      'form',
      { className: 'control-panel', onSubmit: createTask },
      h('label', null, h('span', null, h(Icon, { name: '◆' }), 'JWT'), h('textarea', { value: token, onChange: (event) => setToken(event.target.value), rows: 5 })),
      h('label', null, h('span', null, h(Icon, { name: '#' }), '目标'), h('textarea', { value: goal, onChange: (event) => setGoal(event.target.value), rows: 4 })),
      h('button', { type: 'submit', disabled: busy || !token }, h(Icon, { name: busy ? '○' : '+', spin: busy }), '创建任务'),
    ),
    h(
      'div',
      { className: 'task-list-header' },
      h('strong', null, '任务列表'),
      h('button', { type: 'button', onClick: loadTasks }, h(Icon, { name: '↺' })),
    ),
    h(
      'div',
      { className: 'task-list' },
      tasks.map((item) =>
        h(
          'button',
          { key: item.task_id, type: 'button', className: item.task_id === taskId ? 'active' : '', onClick: () => setTaskId(item.task_id) },
          h('span', null, item.goal),
          h('code', null, item.status),
        ),
      ),
    ),
  );
}

// TaskSummary 渲染任务概要和连接状态。
function TaskSummary({ task, connection }) {
  return h(
    'div',
    { className: 'summary-grid' },
    h(SummaryItem, { label: '当前步骤', value: task.current_step }),
    h(SummaryItem, { label: 'Trace', value: task.trace_id || '-' }),
    h(SummaryItem, { label: '已用 Steps', value: task.budget_usage?.used_steps ?? 0 }),
    h(SummaryItem, { label: 'SSE', value: connectionLabel(connection), tone: connection }),
  );
}

// SummaryItem 渲染一个概要指标。
function SummaryItem({ label, value, tone }) {
  return h('div', { className: `summary-item ${tone || ''}` }, h('span', null, label), h('strong', null, String(value)));
}

// Tabs 渲染资源视图切换。
function Tabs({ activeTab, setActiveTab }) {
  const tabs = [
    ['timeline', 'Timeline'],
    ['approvals', 'Tool Calls'],
    ['checkpoints', 'Checkpoints'],
    ['artifacts', 'Artifacts'],
  ];
  return h('nav', { className: 'tabs' }, tabs.map(([id, label]) => h('button', { key: id, type: 'button', className: activeTab === id ? 'active' : '', onClick: () => setActiveTab(id) }, label)));
}

// Timeline 渲染事件时间线。
function Timeline({ events }) {
  return h('div', { className: 'timeline-list' }, events.length === 0 ? h('div', { className: 'empty-state' }, h(Icon, { name: '●' }), h('span', null, '暂无事件')) : events.map((event) => h(TimelineEvent, { key: event.event_id, event })));
}

// TimelineEvent 渲染单条事件。
function TimelineEvent({ event }) {
  const meta = EVENT_META[event.type] || { label: event.type, icon: '●', tone: 'neutral' };
  return h(
    'article',
    { className: 'timeline-item' },
    h('div', { className: `event-marker ${meta.tone}` }, h(Icon, { name: meta.icon })),
    h(
      'div',
      { className: 'event-body' },
      h('div', { className: 'event-title-row' }, h('div', null, h('strong', null, meta.label), h('code', null, event.type)), h('time', null, formatTime(event.created_at))),
      h('div', { className: 'event-fields' }, h('span', null, `event_id=${event.event_id}`), event.trace_id ? h('span', null, `trace=${event.trace_id}`) : null),
      h(JSONDetails, { value: { input: event.input, output: event.output, metadata: event.metadata } }),
    ),
  );
}

// ToolCalls 渲染工具调用和审批按钮。
function ToolCalls({ toolCalls, decideToolCall, busy }) {
  return h(
    'div',
    { className: 'resource-list' },
    toolCalls.length === 0 ? h('div', { className: 'empty-state' }, h(Icon, { name: '●' }), h('span', null, '暂无工具调用')) : null,
    toolCalls.map((call) =>
      h(
        'article',
        { key: call.call_id, className: 'resource-row' },
        h('div', null, h('strong', null, call.tool_name), h('code', null, `${call.status} · ${call.risk_level} · ${call.approval_status}`)),
        call.approval_status === 'PENDING'
          ? h('div', { className: 'row-actions' }, h('button', { type: 'button', disabled: busy, onClick: () => decideToolCall(call.call_id, 'approve') }, '通过'), h('button', { type: 'button', disabled: busy, onClick: () => decideToolCall(call.call_id, 'reject') }, '拒绝'))
          : null,
        h(JSONDetails, { value: call }),
      ),
    ),
  );
}

// ResourceList 渲染 checkpoint 和 artifact 通用列表。
function ResourceList({ items, title }) {
  return h('div', { className: 'resource-list' }, items.length === 0 ? h('div', { className: 'empty-state' }, h(Icon, { name: '●' }), h('span', null, `${title} 暂无数据`)) : items.map((item, index) => h('article', { key: item.checkpoint_id || item.artifact_id || index, className: 'resource-row' }, h('div', null, h('strong', null, item.reason || item.name || title), h('code', null, item.checkpoint_id || item.artifact_id || 'resource')), h(JSONDetails, { value: item }))));
}

// JSONDetails 渲染可展开 JSON。
function JSONDetails({ value }) {
  return h('details', null, h('summary', null, 'JSON'), h('pre', null, JSON.stringify(value, null, 2)));
}

// Icon 渲染轻量符号图标。
function Icon({ name, spin }) {
  return h('span', { className: `icon ${spin ? 'spin' : ''}`, 'aria-hidden': 'true' }, name);
}

// apiFetch 调用统一后端 API 并返回 data。
async function apiFetch(path, { token, method = 'GET', body } = {}) {
  const response = await fetch(path, {
    method,
    headers: {
      ...authHeaders(token),
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  return readAPIResponse(response);
}

// readAPIResponse 解析统一成功或错误响应。
async function readAPIResponse(response) {
  const text = await response.text();
  const payload = text ? JSON.parse(text) : {};
  if (!response.ok) {
    throw new Error(payload.error?.message || `HTTP ${response.status}`);
  }
  return payload.data;
}

// authHeaders 生成鉴权请求头。
function authHeaders(token) {
  return token ? { Authorization: `Bearer ${token}` } : {};
}

// readSSE 从 fetch Response 中解析 SSE 事件。
async function readSSE(response, onEvent) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      return;
    }
    buffer += decoder.decode(value, { stream: true });
    const frames = buffer.split('\n\n');
    buffer = frames.pop() || '';
    for (const frame of frames) {
      const event = parseSSEFrame(frame);
      if (event) {
        onEvent(event);
      }
    }
  }
}

// parseSSEFrame 解析单个 SSE frame。
function parseSSEFrame(frame) {
  const dataLine = frame.split('\n').find((line) => line.startsWith('data:'));
  if (!dataLine) {
    return null;
  }
  return JSON.parse(dataLine.slice(5).trim());
}

// mergeEvents 按 event_id 合并事件列表。
function mergeEvents(current, incoming) {
  const map = new Map(current.map((event) => [event.event_id, event]));
  for (const event of incoming) {
    map.set(event.event_id, event);
  }
  return Array.from(map.values()).sort((a, b) => a.event_id - b.event_id);
}

// connectionLabel 返回 SSE 连接状态文案。
function connectionLabel(connection) {
  switch (connection) {
    case 'connected':
      return '已连接';
    case 'connecting':
      return '连接中';
    case 'reconnecting':
      return '重连中';
    default:
      return '未连接';
  }
}

// formatTime 格式化时间显示。
function formatTime(value) {
  if (!value) {
    return '-';
  }
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value));
}

createRoot(document.getElementById('root')).render(h(App));
