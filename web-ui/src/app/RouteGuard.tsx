import type { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { Result } from 'antd';
import { isTokenExpired, useAuthStore } from '@/store/authStore';

// RequireAuth 是路由守卫：未登录或 token 已过期一律跳转登录页并保留原始路径用于登录后回跳。
export function RequireAuth({ children }: { children: ReactNode }) {
  const { user } = useAuthStore();
  const location = useLocation();

  if (!user || isTokenExpired(user)) {
    const redirect = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?redirect=${redirect}`} replace />;
  }
  return <>{children}</>;
}

// RequireRole 在已登录基础上按权限矩阵二次校验，无权限时展示 403 而不是白屏或报错。
export function RequireRole({ allowed, children }: { allowed: boolean; children: ReactNode }) {
  if (!allowed) {
    return (
      <Result
        status="403"
        title="没有访问权限"
        subTitle="当前角色无权访问该页面，如有需要请联系管理员调整角色。"
      />
    );
  }
  return <>{children}</>;
}
