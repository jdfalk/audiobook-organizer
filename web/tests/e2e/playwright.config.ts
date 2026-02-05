// file: tests/e2e/playwright.config.ts
// version: 1.3.0
// guid: 7c8d9e0f-1a2b-3c4d-5e6f-7a8b9c0d1e2f

import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

// Centralized demo artifacts directory for all recordings and screenshots
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const DEMO_ARTIFACTS_DIR = join(__dirname, '../../..', 'demo_artifacts');

export default defineConfig({
  testDir: '.',
  timeout: 30 * 1000,
  fullyParallel: true,
  retries: 0,
  workers: 2,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    headless: true,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'chromium-record',
      testMatch: ['**/interactive-*.spec.ts', '**/demo-*.spec.ts'],
      outputDir: DEMO_ARTIFACTS_DIR,
      use: {
        ...devices['Desktop Chrome'],
        screenshot: 'on',
        video: 'on',
      },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
      // We accept WebKit failures for now; main gate stays on Chromium.
      expect: {
        toMatchSnapshot: { maxDiffPixelRatio: 0.05 },
      },
    },
  ],
  webServer: [
    {
      // Start Go backend (required for both Phase 1 and Phase 2)
      command: `bash -c "cd ${__dirname}/../../.. && ./audiobook-organizer serve"`,
      url: 'http://127.0.0.1:8080/api/v1/system/status',
      timeout: 60000,
      reuseExistingServer: !process.env.CI,
    },
    {
      // Start Vite frontend dev server
      command: 'npm run dev -- --host --port 4173',
      url: 'http://127.0.0.1:4173',
      reuseExistingServer: !process.env.CI,
    },
  ],
});
