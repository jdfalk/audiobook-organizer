// file: web/vite.config.ts
// version: 1.2.0
// guid: 9a8b7c6d-5e4f-3a2b-1c0d-9e8f7a6b5c4d
// last-edited: 2026-06-12

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8484',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
    rollupOptions: {
      output: {
        // Function form (not the object form) because vite 8's rolldown bundler
        // only accepts manualChunks as a function — the object form throws
        // "manualChunks is not a function" at build time. Splits the same two
        // long-lived vendor chunks (react core + MUI) for better browser caching.
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined;
          if (id.includes('/@mui/')) return 'mui';
          if (
            id.includes('/react/') ||
            id.includes('/react-dom/') ||
            id.includes('/react-router/') ||
            id.includes('/react-router-dom/') ||
            id.includes('/scheduler/')
          ) {
            return 'vendor';
          }
          return undefined;
        },
      },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      thresholds: {
        statements: 15,
        branches: 10,
        functions: 15,
        lines: 15,
      },
    },
  },
});
