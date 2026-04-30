import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

// VITE_ALLOWED_HOSTS is a comma-separated list of hostnames the dev server will
// accept. Set this when running Vite behind a reverse proxy on a custom domain
// (e.g. `VITE_ALLOWED_HOSTS=dev.example.com,staging.example.com`). Empty /
// unset means "allow localhost only" — Vite's safe default.
const allowedHosts = (process.env.VITE_ALLOWED_HOSTS ?? '')
  .split(',')
  .map((h) => h.trim())
  .filter(Boolean);
const devPort = Number(process.env.VITE_DEV_PORT ?? '5173');
const apiProxyTarget = process.env.VITE_API_PROXY_TARGET ?? 'http://localhost:8080';
const wsProxyTarget = apiProxyTarget.replace(/^http/, 'ws');

export default defineConfig({
  root: import.meta.dirname,
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    port: devPort,
    host: '0.0.0.0',
    ...(allowedHosts.length > 0 && { allowedHosts }),
    proxy: {
      '/api': apiProxyTarget,
      '/ws': {
        target: wsProxyTarget,
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
