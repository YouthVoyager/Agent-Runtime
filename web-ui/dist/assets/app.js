import React, { useEffect, useRef, useState } from 'https://esm.sh/react@19.2.3';
import { createRoot } from 'https://esm.sh/react-dom@19.2.3/client';

const h = React.createElement;

const DEFAULT_BUDGET = {
  max_steps: 50,
  max_tokens: 200000,
  max_tool_calls: 40,
  max_cost_usd: 10,
};

const EVENT_META = {
  TASK_CREATED: { label: '任务已创建', icon: '●', tone: 'blue' },
  STEP_STARTED: { label: '步骤开始', icon: '▶', tone: 'amber' },
  STEP_COMPLETED: { label: '步骤完成', icon: '✓', tone: 'green' },
  TASK_COMPLETED: { label: '任务完成', icon: '✓', tone: 'green' },
  TASK_FAILED: { label: '任务失败', icon: '!', tone: 'red' },
};

// App 渲染任务创建和 timeline 实时查看工作台。
function App() {
  const [token, setToken] = useState(() => localStorage.getItem('agent_runtime_jwt') || '');
  const [taskId, setTaskId] = useState('');
  const [goal, setGoal] = useState('分析需求文档并生成生产级 Go 技术方案');
  const [events, setEvents] = useState([]);
  const [connection, setConnection] = useState('idle');
  const [error, setError] = useState('');
  const [isCreating, setIsCreating] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const abortRef = useRef(null);
  const reconnectRef = useRef(null);
  const lastEventIdRef = useRef(0);

  useEffect(() => {
    localStorage.setItem('agent_runtime_jwt', token);
  }, [token]);

  useEffect(() => {
    let cancelled = false;
    stopStream();
    lastEventIdRef.current = 0;
    setEvents([]);
    if (!token || !taskId) {
      setConnection('idle');
      return () => {
        cancelled = true;
        stopStream();
      };
    }
    loadTimeline(cancelled).then(() => {
      if (!cancelled) {
        connectStream();
      }
    });
    return () => {
      cancelled = true;
      stopStream();
    };
  }, [token, taskId]);

  // createTask 调用后端创建任务接口，并切换到新任务 timeline。
  async function createTask(event) {
    event.preventDefault();
    if (!token || !goal.trim()) {
      setError('JWT 和目标不能为空');
      return;
    }
    setIsCreating(true);
    setError('');
    try {
      const response = await fetch('/api/v1/tasks', {
        method: 'POST',
        headers: {
          ...authHeaders(token),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          goal: goal.trim(),
          budget: DEFAULT_BUDGET,
          constraints: {},
        }),
      });
      const body = await readAPIResponse(response);
      setTaskId(body.task_id);
    } catch (err) {
      setError(err.message);
    } finally {
      setIsCreating(false);
    }
  }

  // loadTimeline 从数据库按 event_id 顺序拉取完整初始 timeline。
  async function loadTimeline(cancelled) {
    setIsLoading(true);
    setError('');
    try {
      let afterEventId = 0;
      let hasMore = true;
      while (hasMore && !cancelled) {
        const response = await fetch(`/api/v1/tasks/${encodeURIComponent(taskId)}/events?after_event_id=${afterEventId}&limit=100`, {
          headers: authHeaders(token),
        });
        const page = await readAPIResponse(response);
        setEvents((current) => mergeEvents(current, page.items || []));
        afterEventId = page.next_after_event_id || afterEventId;
        lastEventIdRef.current = Math.max(lastEventIdRef.current, afterEventId);
        hasMore = Boolean(page.has_more);
      }
    } catch (err) {
      if (!cancelled) {
        setError(err.message);
      }
    } finally {
      if (!cancelled) {
        setIsLoading(false);
      }
    }
  }

  // connectStream 使用 fetch 建立可携带 Authorization header 的 SSE 连接。
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
      await readSSE(response, handleStreamEvent);
      if (!controller.signal.aborted) {
        scheduleReconnect();
      }
    } catch (err) {
      if (controller.signal.aborted) {
        return;
      }
      setError(err.message);
      scheduleReconnect();
    }
  }

  // handleStreamEvent 合并实时事件，并维护下一次重连游标。
  function handleStreamEvent(event) {
    lastEventIdRef.current = Math.max(lastEventIdRef.current, Number(event.event_id || 0));
    setEvents((current) => mergeEvents(current, [event]));
  }

  // scheduleReconnect 在 SSE 断线后延迟重连。
  function scheduleReconnect() {
    setConnection('reconnecting');
    clearTimeout(reconnectRef.current);
    reconnectRef.current = setTimeout(() => {
      connectStream();
    }, 1500);
  }

  // stopStream 关闭当前 SSE 请求和待执行重连定时器。
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
    h(
      'aside',
      { className: 'sidebar' },
      h(
        'div',
        { className: 'brand-row' },
        h(Icon, { name: '◉' }),
        h('div', null, h('h1', null, 'Agent Runtime'), h('span', null, 'Event Timeline')),
      ),
      h(
        'form',
        { className: 'control-panel', onSubmit: createTask },
        h(
          'label',
          null,
          h('span', null, h(Icon, { name: '◆' }), 'JWT'),
          h('textarea', {
            value: token,
            onChange: (event) => setToken(event.target.value),
            rows: 5,
          }),
        ),
        h(
          'label',
          null,
          h('span', null, h(Icon, { name: '#' }), '任务 ID'),
          h('input', {
            value: taskId,
            onChange: (event) => setTaskId(event.target.value.trim()),
          }),
        ),
        h(
          'label',
          null,
          h('span', null, h(Icon, { name: '↗' }), '目标'),
          h('textarea', {
            value: goal,
            onChange: (event) => setGoal(event.target.value),
            rows: 4,
          }),
        ),
        h(
          'button',
          { type: 'submit', disabled: isCreating || !token },
          h(Icon, { name: isCreating ? '○' : '+' , spin: isCreating }),
          '创建任务',
        ),
      ),
    ),
    h(
      'section',
      { className: 'timeline-surface' },
      h(
        'header',
        { className: 'timeline-header' },
        h('div', null, h('p', null, 'Timeline'), h('h2', null, taskId || '未选择任务')),
        h(
          'div',
          { className: `connection ${connection}` },
          h(Icon, { name: connection === 'connected' ? '◉' : '↻', spin: connection === 'connecting' || connection === 'reconnecting' }),
          connectionLabel(connection),
        ),
      ),
      error
        ? h('div', { className: 'error-strip' }, h(Icon, { name: '!' }), h('span', null, error))
        : null,
      h(
        'div',
        { className: 'timeline-list' },
        isLoading ? h('div', { className: 'empty-state' }, h(Icon, { name: '○', spin: true }), h('span', null, '加载中')) : null,
        !isLoading && events.length === 0
          ? h('div', { className: 'empty-state' }, h(Icon, { name: '●' }), h('span', null, '暂无事件'))
          : null,
        events.map((event) => h(TimelineEvent, { key: event.event_id, event })),
      ),
    ),
  );
}

// TimelineEvent 渲染单条 AgentEvent。
function TimelineEvent({ event }) {
  const meta = EVENT_META[event.type] || { label: event.type, icon: '●', tone: 'neutral' };
  return h(
    'article',
    { className: 'timeline-item' },
    h('div', { className: `event-marker ${meta.tone}` }, h(Icon, { name: meta.icon })),
    h(
      'div',
      { className: 'event-body' },
      h(
        'div',
        { className: 'event-title-row' },
        h('div', null, h('strong', null, meta.label), h('code', null, event.type)),
        h('time', null, formatTime(event.created_at)),
      ),
      h(
        'div',
        { className: 'event-fields' },
        h('span', null, `event_id: ${event.event_id}`),
        event.trace_id ? h('span', null, `trace_id: ${event.trace_id}`) : null,
        event.span_id ? h('span', null, `span_id: ${event.span_id}`) : null,
      ),
      h(
        'details',
        null,
        h('summary', null, 'payload'),
        h('pre', null, formatJSON({ input: event.input, output: event.output, metadata: event.metadata })),
      ),
    ),
  );
}

// Icon 渲染轻量符号图标。
function Icon({ name, spin = false }) {
  return h('span', { className: spin ? 'icon spin' : 'icon', 'aria-hidden': 'true' }, name);
}

// authHeaders 生成后端 API 鉴权请求头。
function authHeaders(token) {
  return {
    Authorization: `Bearer ${token}`,
  };
}

// readAPIResponse 读取统一 API 响应，错误响应会抛出业务消息。
async function readAPIResponse(response) {
  let body = null;
  try {
    body = await response.json();
  } catch {
    body = null;
  }
  if (!response.ok) {
    const message = body?.error?.message || response.statusText || `HTTP ${response.status}`;
    throw new Error(message);
  }
  return body?.data ?? body;
}

// readSSE 解析 fetch 返回的 text/event-stream。
async function readSSE(response, onEvent) {
  if (!response.body) {
    throw new Error('SSE 响应没有可读取内容');
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { value, done } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    let splitIndex = buffer.indexOf('\n\n');
    while (splitIndex >= 0) {
      const rawBlock = buffer.slice(0, splitIndex);
      buffer = buffer.slice(splitIndex + 2);
      parseSSEBlock(rawBlock, onEvent);
      splitIndex = buffer.indexOf('\n\n');
    }
  }
}

// parseSSEBlock 解析单个 SSE block，仅处理 agent_event。
function parseSSEBlock(block, onEvent) {
  const lines = block.replaceAll('\r', '').split('\n');
  let eventName = 'message';
  const dataLines = [];
  for (const line of lines) {
    if (line.startsWith(':')) {
      continue;
    }
    if (line.startsWith('event:')) {
      eventName = line.slice(6).trim();
      continue;
    }
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart());
    }
  }
  if (eventName !== 'agent_event' || dataLines.length === 0) {
    return;
  }
  onEvent(JSON.parse(dataLines.join('\n')));
}

// mergeEvents 按 event_id 去重并保持升序。
function mergeEvents(current, incoming) {
  const byID = new Map();
  for (const event of current) {
    byID.set(Number(event.event_id), event);
  }
  for (const event of incoming) {
    byID.set(Number(event.event_id), event);
  }
  return [...byID.values()].sort((left, right) => Number(left.event_id) - Number(right.event_id));
}

// connectionLabel 返回连接状态展示文本。
function connectionLabel(connection) {
  switch (connection) {
    case 'connected':
      return '实时连接';
    case 'connecting':
      return '连接中';
    case 'reconnecting':
      return '重连中';
    default:
      return '未连接';
  }
}

// formatTime 格式化事件创建时间。
function formatTime(value) {
  if (!value) {
    return '';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date(value));
}

// formatJSON 以稳定缩进展示事件 payload。
function formatJSON(value) {
  return JSON.stringify(value, null, 2);
}

createRoot(document.getElementById('root')).render(h(React.StrictMode, null, h(App)));
