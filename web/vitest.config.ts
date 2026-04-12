// file: vitest.config.ts
// version: 1.1.0
// guid: 9c0d1e2f-3a4b-5c6d-7e8f-9a0b1c2d3e4f

import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    globals: true,
    exclude: ['tests/e2e/**', 'node_modules/**', 'dist/**'],
    // CI runners are slower than local dev machines. The default
    // 5000ms timeout was causing false failures on tests that
    // render complex components (Library, BookDetail) with
    // multiple async operations. 30s is generous enough for CI
    // while still catching genuinely hung tests.
    testTimeout: 30000,
  },
});
