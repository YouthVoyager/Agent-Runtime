import crypto from 'node:crypto';

// base64url 将 Buffer 转换为 JWT 使用的 base64url 字符串。
function base64url(input) {
  return Buffer.from(input).toString('base64url');
}

// sign 生成 HS256 JWT 签名。
function sign(value, secret) {
  return crypto.createHmac('sha256', secret).update(value).digest('base64url');
}

// readArg 读取命令行参数，缺失时返回默认值。
function readArg(name, fallback) {
  const index = process.argv.indexOf(`--${name}`);
  if (index === -1 || index + 1 >= process.argv.length) {
    return fallback;
  }
  return process.argv[index + 1];
}

const secret = readArg('secret', process.env.JWT_SECRET || 'local-dev-secret');
const tenantID = readArg('tenant', 'tenant_local');
const userID = readArg('user', 'admin_local');
const role = readArg('role', 'admin');
const now = Math.floor(Date.now() / 1000);

const header = base64url(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
const payload = base64url(JSON.stringify({
  tenant_id: tenantID,
  user_id: userID,
  role,
  iat: now,
  exp: now + 24 * 60 * 60,
}));
const signingInput = `${header}.${payload}`;

process.stdout.write(`${signingInput}.${sign(signingInput, secret)}\n`);
