import dayjs from 'dayjs';

export function formatDateTime(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : '-';
}

export function formatNumber(value?: number | null): string {
  if (value === undefined || value === null) return '-';
  return value.toLocaleString('zh-CN');
}

export function formatCost(value?: number | null): string {
  if (value === undefined || value === null) return '-';
  return `$${value.toFixed(4)}`;
}

export function formatPercent(value?: number | null): string {
  if (value === undefined || value === null) return '-';
  return `${(value * 100).toFixed(1)}%`;
}

export function formatBytes(bytes?: number | null): string {
  if (bytes === undefined || bytes === null) return '-';
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let value = bytes / 1024;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i += 1;
  }
  return `${value.toFixed(1)} ${units[i]}`;
}

export function formatDurationMs(ms?: number | null): string {
  if (ms === undefined || ms === null) return '-';
  if (ms < 1000) return `${ms}ms`;
  const seconds = ms / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = Math.round(seconds % 60);
  return `${minutes}m${rest}s`;
}

export function truncate(text: string, max = 200): string {
  if (text.length <= max) return text;
  return `${text.slice(0, max)}...`;
}

export function safeJsonStringify(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function summarize(value: unknown, max = 120): string {
  if (value === null || value === undefined) return '-';
  const text = typeof value === 'string' ? value : safeJsonStringify(value);
  return truncate(text.replace(/\s+/g, ' '), max);
}
