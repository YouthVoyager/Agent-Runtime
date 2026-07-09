import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchEventSource } from '@microsoft/fetch-event-source';
import { getStoredToken } from '@/store/authStore';
import { tasksApi } from '@/api/tasks';
import type { AgentEvent } from '@/types/event';

const MAX_BACKOFF_MS = 30_000;
const BASE_BACKOFF_MS = 1_000;

interface UseTaskEventsResult {
  events: AgentEvent[];
  connected: boolean;
  error: string | null;
}

// useTaskEvents 负责任务详情页 Timeline 的实时数据：
// 1) 先拉一遍历史事件兜底（防止 SSE 建联前的窗口丢事件）；
// 2) 用 fetch-event-source 建立 SSE 长连接（带 Authorization 头，原生 EventSource 做不到）；
// 3) 断线后指数退避重连，`fetchEventSource` 库会自动把收到的最后一个 id 当作 Last-Event-ID 带上，
//    服务端据此只补发缺失事件，天然支持"断线重连 + 不丢不重"；
// 4) 按 event_id 去重后按序追加，避免历史补发和实时推送重叠。
// 5) 组件卸载时通过 AbortController 中止连接，避免内存泄漏和僵尸请求。
export function useTaskEvents(taskId: string | undefined): UseTaskEventsResult {
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const seenIds = useRef<Set<number>>(new Set());
  const retryCountRef = useRef(0);

  const appendEvent = useCallback((event: AgentEvent) => {
    if (seenIds.current.has(event.event_id)) return;
    seenIds.current.add(event.event_id);
    setEvents((prev) => {
      const next = [...prev, event];
      next.sort((a, b) => a.event_id - b.event_id);
      return next;
    });
  }, []);

  useEffect(() => {
    if (!taskId) return;
    seenIds.current = new Set();
    retryCountRef.current = 0;
    setEvents([]);
    setError(null);
    const controller = new AbortController();
    let disposed = false;

    async function bootstrapAndConnect() {
      // 先分页拉全部历史事件，保证首屏可见，即使 SSE 暂时连不上也能看到 timeline。
      try {
        let afterEventId = 0;
        for (;;) {
          const page = await tasksApi.events(taskId as string, { after_event_id: afterEventId, limit: 200 });
          page.items.forEach(appendEvent);
          if (!page.has_more || page.items.length === 0) break;
          afterEventId = page.next_after_event_id;
        }
      } catch {
        // 历史拉取失败不阻断 SSE 连接尝试，交给 SSE 的 backfill 兜底。
      }
      if (disposed) return;

      const token = getStoredToken();
      await fetchEventSource(`/api/v1/tasks/${taskId}/events/stream`, {
        signal: controller.signal,
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        openWhenHidden: true,
        async onopen(response) {
          if (response.ok) {
            retryCountRef.current = 0;
            setConnected(true);
            setError(null);
            return;
          }
          throw new Error(`SSE 连接失败: HTTP ${response.status}`);
        },
        onmessage(msg) {
          if (!msg.data) return;
          try {
            const event = JSON.parse(msg.data) as AgentEvent;
            appendEvent(event);
          } catch {
            // 忽略无法解析的心跳或异常帧。
          }
        },
        onclose() {
          setConnected(false);
          // 库在 onclose 后会按 onerror 返回值重连；这里主动抛出触发统一的退避逻辑。
          throw new Error('SSE 连接被服务端关闭');
        },
        onerror(err) {
          setConnected(false);
          if (disposed) {
            throw err; // 组件已卸载，停止重连。
          }
          retryCountRef.current += 1;
          const delay = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * 2 ** (retryCountRef.current - 1));
          setError(`连接中断，${Math.round(delay / 1000)}s 后重试`);
          return delay; // 返回数值 = 指数退避后重试；不抛错即视为可重试。
        },
      });
    }

    bootstrapAndConnect().catch((err) => {
      if (!disposed) {
        setError(err instanceof Error ? err.message : 'SSE 连接失败');
      }
    });

    return () => {
      disposed = true;
      controller.abort();
      setConnected(false);
    };
  }, [taskId, appendEvent]);

  return { events, connected, error };
}
