import { useMemo } from 'react';
import { Layout, Menu, Space, Tag, Dropdown, Typography } from 'antd';
import type { MenuProps } from 'antd';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  DashboardOutlined,
  UnorderedListOutlined,
  CheckCircleOutlined,
  ToolOutlined,
  EyeOutlined,
  FileZipOutlined,
  ApartmentOutlined,
  TeamOutlined,
  SettingOutlined,
  AuditOutlined,
  UserOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import { useAuthStore } from '@/store/authStore';
import { usePermission } from '@/hooks/usePermission';

const { Header, Sider, Content } = Layout;

// AppLayout 是登录后所有页面的外壳：左侧导航按角色过滤菜单项（对照设计稿第二/三节），
// 顶部展示当前用户 + 角色 Tag + 登出入口。
export function AppLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout } = useAuthStore();
  const perm = usePermission();

  const menuItems: MenuProps['items'] = useMemo(() => {
    const items: MenuProps['items'] = [
      { key: '/dashboard', icon: <DashboardOutlined />, label: 'Dashboard' },
      {
        key: 'tasks',
        icon: <UnorderedListOutlined />,
        label: 'Tasks',
        children: [
          { key: '/tasks', label: 'Task List' },
          ...(perm.canCreateTask ? [{ key: '/tasks/create', label: 'Task Create' }] : []),
        ],
      },
      { key: '/approvals', icon: <CheckCircleOutlined />, label: 'Approvals' },
    ];

    const toolsChildren = [
      ...(perm.canAccessToolsAdmin ? [{ key: '/tools/registry', label: 'Tool Registry' }] : []),
      ...(perm.canAccessToolsAdmin ? [{ key: '/tools/policies', label: 'Tool Policies' }] : []),
      { key: '/tools/calls', label: 'Tool Calls' },
    ];
    if (toolsChildren.length > 0) {
      items.push({ key: 'tools', icon: <ToolOutlined />, label: 'Tools', children: toolsChildren });
    }

    items.push({
      key: 'observability',
      icon: <EyeOutlined />,
      label: 'Observability',
      children: [
        { key: '/observability/timeline', label: 'Timeline Explorer' },
        { key: '/observability/trace', label: 'Trace Viewer' },
        { key: '/observability/metrics', label: 'Metrics' },
        { key: '/observability/logs', label: 'Logs' },
      ],
    });

    items.push({ key: '/artifacts', icon: <FileZipOutlined />, label: 'Artifacts' });

    if (perm.canAccessTenants) {
      items.push({ key: '/tenants', icon: <ApartmentOutlined />, label: 'Tenants' });
    }
    if (perm.canAccessUsers) {
      items.push({ key: '/users', icon: <TeamOutlined />, label: 'Users & Roles' });
    }
    if (perm.canAccessSettings) {
      items.push({ key: '/settings', icon: <SettingOutlined />, label: 'Settings' });
    }
    if (perm.canAccessAuditLogs) {
      items.push({ key: '/audit', icon: <AuditOutlined />, label: 'Audit Logs' });
    }
    return items;
  }, [perm]);

  const selectedKey = location.pathname;
  const openKeys = ['tasks', 'tools', 'observability'];

  const userMenu: MenuProps['items'] = [
    { key: 'logout', icon: <LogoutOutlined />, label: '登出' },
  ];

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider width={220} theme="dark">
        <div style={{ color: '#fff', padding: 16, fontWeight: 600, fontSize: 16 }}>StableAgent Console</div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          defaultOpenKeys={openKeys}
          items={menuItems}
          onClick={({ key }) => {
            if (key.startsWith('/')) navigate(key);
          }}
        />
      </Sider>
      <Layout>
        <Header style={{ background: '#fff', padding: '0 20px', display: 'flex', justifyContent: 'flex-end', alignItems: 'center', borderBottom: '1px solid #f0f0f0' }}>
          <Dropdown
            menu={{
              items: userMenu,
              onClick: ({ key }) => {
                if (key === 'logout') {
                  logout();
                  navigate('/login');
                }
              },
            }}
          >
            <Space style={{ cursor: 'pointer' }}>
              <UserOutlined />
              <Typography.Text>{user?.userId}</Typography.Text>
              <Tag color="blue">{user?.role}</Tag>
            </Space>
          </Dropdown>
        </Header>
        <Content style={{ margin: 20 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
