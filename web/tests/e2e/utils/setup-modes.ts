// file: web/tests/e2e/utils/setup-modes.ts
// version: 1.1.0
// guid: f1e2d3c4-b5a6-7890-cdef-a1b2c3d4e5f6
// last-edited: 2026-02-04

import { Page } from '@playwright/test';
import type { MockApiOptions } from './test-helpers';
import { skipWelcomeWizard, setupMockApi } from './test-helpers';

/**
 * Call the factory reset endpoint to reset the app to factory defaults
 *
 * This endpoint is called on the backend to clear all data and state.
 * Falls back gracefully if the endpoint doesn't exist (404) for testing
 * with backends that don't yet support the reset endpoint.
 *
 * @param page - Playwright test page object
 * @param baseURL - Base URL for API calls (defaults to http://127.0.0.1:4173)
 */
export async function resetToFactoryDefaults(
  page: Page,
  baseURL: string = 'http://127.0.0.1:4173'
): Promise<boolean> {
  try {
    const response = await page.evaluate(
      async ({ baseURL }: { baseURL: string }) => {
        try {
          const res = await fetch(`${baseURL}/api/v1/system/reset`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({}),
          });
          return { success: res.ok, status: res.status };
        } catch (error) {
          return { success: false, status: 0, error: String(error) };
        }
      },
      { baseURL }
    );

    if (!response.success) {
      // Log but don't fail - endpoint may not exist in some test environments
      console.warn(
        `Factory reset endpoint returned status ${response.status}: ${response.error || 'unknown error'}`
      );
      return false;
    }
    return true;
  } catch (error) {
    console.warn(`Failed to reset factory defaults: ${String(error)}`);
    return false;
  }
}

/**
 * Phase 1: API-Driven Setup
 *
 * Resets the app to factory defaults and sets up with real API calls.
 * Use this mode when:
 * - Testing API integration
 * - Testing end-to-end workflows with real backend
 * - Testing actual API behavior and error handling
 *
 * Skips the welcome wizard since we're using real APIs.
 *
 * @param page - Playwright test page object
 * @param baseURL - Base URL for API calls (defaults to http://127.0.0.1:4173)
 */
export async function setupPhase1ApiDriven(
  page: Page,
  baseURL: string = 'http://127.0.0.1:4173'
): Promise<void> {
  // Attempt to reset to factory defaults
  await resetToFactoryDefaults(page, baseURL);

  // Skip the welcome wizard - real API will be used
  await skipWelcomeWizard(page);
}

/**
 * Phase 2: Interactive UI Testing
 *
 * Resets the app to factory defaults and sets up with mocked APIs.
 * Use this mode when:
 * - Testing UI interactions and workflows
 * - Testing without a backend server
 * - Testing specific edge cases with mock data
 * - Running tests in CI/CD environments
 *
 * The mockOptions parameter allows customizing the mock API responses.
 *
 * @param page - Playwright test page object
 * @param baseURL - Base URL for API calls (defaults to http://127.0.0.1:4173)
 * @param mockOptions - Options for mock API responses
 */
export async function setupPhase2Interactive(
  page: Page,
  baseURL: string = 'http://127.0.0.1:4173',
  mockOptions: MockApiOptions = {}
): Promise<void> {
  // Attempt to reset to factory defaults
  await resetToFactoryDefaults(page, baseURL);

  // Set up mock APIs with provided options
  await setupMockApi(page, mockOptions);

  // Skip the welcome wizard when using mocked APIs
  await skipWelcomeWizard(page);
}
