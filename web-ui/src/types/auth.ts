import type { Role } from './common';

// 前端本地解码的 JWT payload（不做签名校验，签名校验由后端负责）。
export interface JwtPayload {
  tenant_id: string;
  user_id?: string;
  sub?: string;
  role?: Role;
  exp?: number;
  iat?: number;
  nbf?: number;
}

export interface AuthUser {
  userId: string;
  tenantId: string;
  role: Role;
  exp?: number;
}
