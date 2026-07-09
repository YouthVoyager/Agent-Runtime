import type { ReactNode } from 'react';
import { Button, Select, Space, Table } from 'antd';
import type { TableProps } from 'antd';
import type { Page } from '@/types/common';

interface PageTableProps<T extends object> {
  rowKey: string;
  columns: TableProps<T>['columns'];
  data?: Page<T>;
  loading?: boolean;
  page: number;
  pageSize: number;
  onPageChange: (page: number, pageSize: number) => void;
  toolbar?: ReactNode;
  rowSelection?: TableProps<T>['rowSelection'];
  scroll?: TableProps<T>['scroll'];
}

// PageTable 封装分页 + 工具栏的通用表格。
// 后端分页响应只给 has_more（没有 total 总数),因此这里用"上一页 / 下一页" + 页码显示，
// 而不是 AntD 默认依赖 total 的页码跳转控件。
export function PageTable<T extends object>({
  rowKey,
  columns,
  data,
  loading,
  page,
  pageSize,
  onPageChange,
  toolbar,
  rowSelection,
  scroll,
}: PageTableProps<T>) {
  return (
    <div>
      {toolbar && (
        <div style={{ marginBottom: 12, display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 8 }}>
          {toolbar}
        </div>
      )}
      <Table<T>
        rowKey={rowKey}
        columns={columns}
        dataSource={data?.items ?? []}
        loading={loading}
        pagination={false}
        rowSelection={rowSelection}
        scroll={scroll}
      />
      <div style={{ marginTop: 12, display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>
        <Space>
          <Select
            size="small"
            value={pageSize}
            style={{ width: 100 }}
            onChange={(size) => onPageChange(1, size)}
            options={[10, 20, 50, 100].map((n) => ({ value: n, label: `${n} 条/页` }))}
          />
          <Button size="small" disabled={page <= 1} onClick={() => onPageChange(page - 1, pageSize)}>
            上一页
          </Button>
          <span>第 {page} 页</span>
          <Button size="small" disabled={!data?.has_more} onClick={() => onPageChange(page + 1, pageSize)}>
            下一页
          </Button>
        </Space>
      </div>
    </div>
  );
}
