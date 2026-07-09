import { useMemo } from 'react';
import { useAuthStore } from '@/store/authStore';
import * as perm from '@/utils/permissions';
import type { Role } from '@/types/common';

// usePermission 提供当前登录角色的权限判断集合，供路由守卫和菜单过滤、按钮显隐统一使用。
export function usePermission() {
  const user = useAuthStore((s) => s.user);
  const role: Role = user?.role ?? 'user';

  return useMemo(
    () => ({
      role,
      user,
      canWrite: perm.canWrite(role),
      canCreateTask: perm.canCreateTask(role),
      canAccessApprovals: perm.canAccessApprovals(),
      canAccessToolsAdmin: perm.canAccessToolsAdmin(role),
      canWriteToolsAdmin: perm.canWriteToolsAdmin(role),
      canAccessObservability: perm.canAccessObservability(),
      canAccessArtifacts: perm.canAccessArtifacts(),
      canAccessToolCalls: perm.canAccessToolCalls(),
      canAccessTenants: perm.canAccessTenants(role),
      canWriteTenants: perm.canWriteTenants(role),
      canAccessUsers: perm.canAccessUsers(role),
      canWriteUsers: perm.canWriteUsers(role),
      canAccessSettings: perm.canAccessSettings(role),
      canWriteSettings: perm.canWriteSettings(role),
      canAccessAuditLogs: perm.canAccessAuditLogs(role),
      canBatchCancel: perm.canBatchCancel(role),
      canExportTasks: perm.canExportTasks(role),
      isPlatformAdmin: perm.isPlatformAdmin(role),
      isTenantAdmin: perm.isTenantAdmin(role),
      isAuditor: perm.isAuditor(role),
    }),
    [role, user]
  );
}
