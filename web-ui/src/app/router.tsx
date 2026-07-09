import type { ReactNode } from 'react';
import { createBrowserRouter, Navigate } from 'react-router-dom';
import { Result } from 'antd';
import { AppLayout } from '@/components/layout/AppLayout';
import { RequireAuth, RequireRole } from '@/app/RouteGuard';
import { usePermission } from '@/hooks/usePermission';
import { LoginPage } from '@/pages/login/LoginPage';
import { DashboardPage } from '@/pages/dashboard/DashboardPage';
import { TaskListPage } from '@/pages/tasks/TaskListPage';
import { TaskCreatePage } from '@/pages/tasks/TaskCreatePage';
import { TaskDetailPage } from '@/pages/tasks/TaskDetailPage';
import { ApprovalsPage } from '@/pages/approvals/ApprovalsPage';
import { ToolRegistryPage } from '@/pages/tools/ToolRegistryPage';
import { ToolPoliciesPage } from '@/pages/tools/ToolPoliciesPage';
import { ToolCallsPage } from '@/pages/tools/ToolCallsPage';
import { TimelineExplorerPage } from '@/pages/observability/TimelineExplorerPage';
import { TraceViewerPage } from '@/pages/observability/TraceViewerPage';
import { MetricsPage } from '@/pages/observability/MetricsPage';
import { LogsPage } from '@/pages/observability/LogsPage';
import { ArtifactsPage } from '@/pages/artifacts/ArtifactsPage';
import { TenantsPage } from '@/pages/tenants/TenantsPage';
import { UsersPage } from '@/pages/users/UsersPage';
import { SettingsPage } from '@/pages/settings/SettingsPage';
import { AuditLogsPage } from '@/pages/audit/AuditLogsPage';

// PermissionRoute 将权限 hook 适配为路由元素，避免菜单可见但直达 URL 绕过前端守卫。
function PermissionRoute({ children, allow }: { children: ReactNode; allow: (permission: ReturnType<typeof usePermission>) => boolean }) {
  const permission = usePermission();
  return <RequireRole allowed={allow(permission)}>{children}</RequireRole>;
}

// NotFoundPage 渲染 SPA 内部 404。
function NotFoundPage() {
  return <Result status="404" title="页面不存在" subTitle="请从左侧导航进入管理台页面。" />;
}

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  {
    path: '/',
    element: (
      <RequireAuth>
        <AppLayout />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: 'dashboard', element: <DashboardPage /> },
      { path: 'tasks', element: <TaskListPage /> },
      {
        path: 'tasks/create',
        element: (
          <PermissionRoute allow={(p) => p.canCreateTask}>
            <TaskCreatePage />
          </PermissionRoute>
        ),
      },
      { path: 'tasks/:taskId', element: <TaskDetailPage /> },
      { path: 'approvals', element: <ApprovalsPage /> },
      {
        path: 'tools/registry',
        element: (
          <PermissionRoute allow={(p) => p.canAccessToolsAdmin}>
            <ToolRegistryPage />
          </PermissionRoute>
        ),
      },
      {
        path: 'tools/policies',
        element: (
          <PermissionRoute allow={(p) => p.canAccessToolsAdmin}>
            <ToolPoliciesPage />
          </PermissionRoute>
        ),
      },
      { path: 'tools/calls', element: <ToolCallsPage /> },
      { path: 'observability/timeline', element: <TimelineExplorerPage /> },
      { path: 'observability/trace', element: <TraceViewerPage /> },
      { path: 'observability/metrics', element: <MetricsPage /> },
      { path: 'observability/logs', element: <LogsPage /> },
      { path: 'artifacts', element: <ArtifactsPage /> },
      {
        path: 'tenants',
        element: (
          <PermissionRoute allow={(p) => p.canAccessTenants}>
            <TenantsPage />
          </PermissionRoute>
        ),
      },
      {
        path: 'users',
        element: (
          <PermissionRoute allow={(p) => p.canAccessUsers}>
            <UsersPage />
          </PermissionRoute>
        ),
      },
      {
        path: 'settings',
        element: (
          <PermissionRoute allow={(p) => p.canAccessSettings}>
            <SettingsPage />
          </PermissionRoute>
        ),
      },
      {
        path: 'audit',
        element: (
          <PermissionRoute allow={(p) => p.canAccessAuditLogs}>
            <AuditLogsPage />
          </PermissionRoute>
        ),
      },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
]);
