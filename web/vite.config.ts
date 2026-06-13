// file: web/vite.config.ts
// version: 1.3.0
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
        // Pinned to vite 7 (rollup). vite 8's rolldown bundler crashed the whole
        // app with React error #130 (a CJS/ESM interop bug resolving MUI/emotion
        // to a namespace object) regardless of chunking — see the toolchain pin
        // in package.json. On rollup this object form splits the two long-lived
        // vendor chunks for browser caching.
        manualChunks: {
          vendor: ['react', 'react-dom', 'react-router-dom'],
          mui: ['@mui/material', '@mui/icons-material'],
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
