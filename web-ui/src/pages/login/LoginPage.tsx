import { useState } from 'react';
import { Alert, Button, Card, Form, Input, Space, Typography, message } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuthStore } from '@/store/authStore';

const { Title, Paragraph, Text } = Typography;

// LoginPage 是开发态登录方式：粘贴一枚 JWT，前端在本地解码 payload 拿 sub/tenant_id/role/exp，
// 真正的签名校验交给后端 JWTMiddleware。生产环境应替换为正式的 OIDC/账号密码登录流程。
export function LoginPage() {
  const [token, setToken] = useState('');
  const [error, setError] = useState<string | null>(null);
  const setStoredToken = useAuthStore((s) => s.setToken);
  const navigate = useNavigate();
  const [params] = useSearchParams();

  const handleSubmit = () => {
    const trimmed = token.trim();
    if (!trimmed) {
      setError('请输入 JWT');
      return;
    }
    const ok = setStoredToken(trimmed);
    if (!ok) {
      setError('无法解析该 JWT，请确认 payload 包含 tenant_id 和 sub/user_id');
      return;
    }
    message.success('登录成功');
    const redirect = params.get('redirect');
    navigate(redirect ? decodeURIComponent(redirect) : '/dashboard', { replace: true });
  };

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#f5f5f5' }}>
      <Card style={{ width: 480 }}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Title level={3} style={{ marginBottom: 0 }}>
            StableAgent Console
          </Title>
          <Paragraph type="secondary">
            开发环境登录：粘贴 JWT Bearer token。可在仓库根目录执行 <Text code>make jwt</Text> 生成本地开发用 token。
          </Paragraph>
          {error && <Alert type="error" message={error} showIcon closable onClose={() => setError(null)} />}
          <Form layout="vertical" onFinish={handleSubmit}>
            <Form.Item label="JWT Token">
              <Input.TextArea
                rows={5}
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
              />
            </Form.Item>
            <Button type="primary" htmlType="submit" block>
              登录
            </Button>
          </Form>
        </Space>
      </Card>
    </div>
  );
}
