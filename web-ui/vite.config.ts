import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// StableAgent 管理台前端构建配置。
// 产物输出到 web-ui/dist，由 api-service 托管（已配置 SPA fallback）。
// 开发模式下把 /api 代理到本地 api-service（:8080），避免浏览器端跨域和手动拼接 base url。
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
  },
});
