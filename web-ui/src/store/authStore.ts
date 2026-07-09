import { create } from 'zustand';
import type { AuthUser, JwtPayload } from '@/types/auth';

const TOKEN_STORAGE_KEY = 'stableagent_jwt';

// decodeJwtPayload 仅在前端 base64url 解码 JWT payload 用于展示和权限判断，
// 不做签名校验——真正的校验永远在后端 JWTMiddleware 完成。
function decodeJwtPayload(token: string): JwtPayload | null {
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), '=');
    const json = decodeURIComponent(
      atob(padded)
        .split('')
        .map((c) => '%' + c.charCodeAt(0).toString(16).padStart(2, '0'))
        .join('')
    );
    return JSON.parse(json) as JwtPayload;
  } catch {
    return null;
  }
}

function toAuthUser(token: string): AuthUser | null {
  const payload = decodeJwtPayload(token);
  if (!payload || !payload.tenant_id) return null;
  const userId = payload.user_id || payload.sub;
  if (!userId) return null;
  const rawRole = (payload.role || 'user').toLowerCase();
  // 历史遗留 member 等价于 user。
  const role = rawRole === 'member' ? 'user' : rawRole;
  return {
    userId,
    tenantId: payload.tenant_id,
    role: role as AuthUser['role'],
    exp: payload.exp,
  };
}

export function isTokenExpired(user: AuthUser | null): boolean {
  if (!user || !user.exp) return false;
  return Date.now() >= user.exp * 1000;
}

interface AuthState {
  token: string | null;
  user: AuthUser | null;
  setToken: (token: string) => boolean;
  logout: () => void;
}

const initialToken = localStorage.getItem(TOKEN_STORAGE_KEY);
const initialUser = initialToken ? toAuthUser(initialToken) : null;

export const useAuthStore = create<AuthState>((set) => ({
  token: initialUser ? initialToken : null,
  user: initialUser,
  setToken: (token: string) => {
    const user = toAuthUser(token);
    if (!user) return false;
    localStorage.setItem(TOKEN_STORAGE_KEY, token);
    set({ token, user });
    return true;
  },
  logout: () => {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    set({ token: null, user: null });
  },
}));

export function getStoredToken(): string | null {
  return localStorage.getItem(TOKEN_STORAGE_KEY);
}
