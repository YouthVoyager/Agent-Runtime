import type { Role } from '@/types/common';

// 权限矩阵严格对照 docs/admin-console-design.md 第三节。
// 角色统一按小写比较；member 在 authStore 解码阶段已归一化为 user。

export function isAuditor(role: Role): boolean {
  return role === 'auditor';
}

export function isPlatformAdmin(role: Role): boolean {
  return role === 'platform_admin';
}

export function isTenantAdmin(role: Role): boolean {
  return role === 'tenant_admin';
}

export function isAdminLike(role: Role): boolean {
  return isTenantAdmin(role) || isPlatformAdmin(role);
}

// canWrite 是全局写权限总闸：auditor 任何时候都看不到写按钮。
export function canWrite(role: Role): boolean {
  return !isAuditor(role);
}

export function canCreateTask(role: Role): boolean {
  // 设计稿:USER 是、APPROVER 可选、AUDITOR 否、TENANT_ADMIN/PLATFORM_ADMIN 是。
  return role !== 'auditor';
}

export function canAccessApprovals(): boolean {
  return true; // 所有角色都能进入审批中心，范围由后端按角色过滤。
}

export function canAccessToolsAdmin(role: Role): boolean {
  // Tool Registry / Tool Policies：管理员可写，auditor 只读，其余角色不可见。
  return isAdminLike(role) || isAuditor(role);
}

export function canWriteToolsAdmin(role: Role): boolean {
  return isAdminLike(role);
}

export function canAccessObservability(): boolean {
  return true;
}

export function canAccessArtifacts(): boolean {
  return true;
}

export function canAccessToolCalls(): boolean {
  return true;
}

export function canAccessTenants(role: Role): boolean {
  return isAdminLike(role);
}

export function canWriteTenants(role: Role): boolean {
  // 契约:创建/修改租户仅 platform_admin；tenant_admin 只能查看本租户详情。
  return isPlatformAdmin(role);
}

export function canAccessUsers(role: Role): boolean {
  return isAdminLike(role);
}

export function canWriteUsers(role: Role): boolean {
  return isAdminLike(role);
}

export function canAccessSettings(role: Role): boolean {
  return isAdminLike(role);
}

export function canWriteSettings(role: Role): boolean {
  // 契约:PUT /settings/{section} 仅 platform_admin；tenant_admin 只读。
  return isPlatformAdmin(role);
}

export function canAccessAuditLogs(role: Role): boolean {
  return isAuditor(role) || isAdminLike(role);
}

export function canBatchCancel(role: Role): boolean {
  return isAdminLike(role);
}

export function canExportTasks(role: Role): boolean {
  return isAdminLike(role) || isAuditor(role);
}
