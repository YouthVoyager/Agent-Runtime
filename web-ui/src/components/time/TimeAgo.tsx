import { useEffect, useState } from 'react';
import { Tooltip } from 'antd';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import { formatDateTime } from '@/utils/format';

dayjs.extend(relativeTime);

interface TimeAgoProps {
  value?: string | null;
}

// TimeAgo 显示相对时间（如"3 分钟前"），hover 显示完整时间戳；每分钟自动刷新一次。
export function TimeAgo({ value }: TimeAgoProps) {
  const [, forceTick] = useState(0);

  useEffect(() => {
    const timer = setInterval(() => forceTick((n) => n + 1), 60_000);
    return () => clearInterval(timer);
  }, []);

  if (!value) return <span>-</span>;
  const d = dayjs(value);
  if (!d.isValid()) return <span>-</span>;

  return (
    <Tooltip title={formatDateTime(value)}>
      <span>{d.fromNow()}</span>
    </Tooltip>
  );
}
