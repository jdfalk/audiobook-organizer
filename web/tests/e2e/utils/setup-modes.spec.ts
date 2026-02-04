// file: web/tests/e2e/utils/setup-modes.spec.ts
// version: 1.0.0
// guid: e2d3c4b5-a6f7-8901-2345-6789abcdef01

import { test, expect } from '@playwright/test';
import {
  mockEventSource,
  setupPhase1ApiDriven,
  setupPhase2Interactive,
  resetToFactoryDefaults,
} from './test-helpers';

test.describe('Setup Modes', () => {
  test('resetToFactoryDefaults handles missing endpoint gracefully', async ({
    page,
    baseURL,
  }) => {
    // Arrange - mock an endpoint that doesn't exist
    const result = await resetToFactoryDefaults(page, baseURL || 'http://localhost:5173');

    // Assert - should return false for endpoint that doesn't exist, but not throw
    expect(typeof result).toBe('boolean');
  });

  test('setupPhase1ApiDriven skips welcome wizard', async ({
    page,
    baseURL,
  }) => {
    // Arrange
    await mockEventSource(page);

    // Act
    await setupPhase1ApiDriven(page, baseURL || 'http://localhost:5173');
    await page.goto(baseURL || 'http://localhost:5173');

    // Assert - verify welcome_wizard_completed is set in localStorage
    const wizardCompleted = await page.evaluate(() => {
      return localStorage.getItem('welcome_wizard_completed');
    });
    expect(wizardCompleted).toBe('true');
  });

  test('setupPhase2Interactive sets up mock API and skips wizard', async ({
    page,
    baseURL,
  }) => {
    // Arrange
    await mockEventSource(page);

    // Act
    await setupPhase2Interactive(page, baseURL || 'http://localhost:5173', {
      books: [],
    });
    await page.goto(baseURL || 'http://localhost:5173');

    // Assert - verify welcome_wizard_completed is set
    const wizardCompleted = await page.evaluate(() => {
      return localStorage.getItem('welcome_wizard_completed');
    });
    expect(wizardCompleted).toBe('true');

    // Verify mock API is set up
    const apiMock = await page.evaluate(() => {
      return (window as unknown as { __apiMock: unknown }).__apiMock !== undefined;
    });
    expect(apiMock).toBe(true);
  });

  test('setupPhase2Interactive accepts custom mock options', async ({
    page,
    baseURL,
  }) => {
    // Arrange
    await mockEventSource(page);
    const mockBooks = [
      {
        id: 'test-1',
        title: 'Test Book',
        author_name: 'Test Author',
        series_name: null,
        series_position: null,
        library_state: 'organized',
        marked_for_deletion: false,
        language: 'en',
        file_path: '/library/test.m4b',
        file_hash: 'testhash',
        original_file_hash: 'testhash',
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ];

    // Act
    await setupPhase2Interactive(page, baseURL || 'http://localhost:5173', {
      books: mockBooks,
    });

    // Assert - verify we can navigate and access the API
    await page.goto(baseURL || 'http://localhost:5173');
    const response = await page.evaluate(async () => {
      const res = await fetch('/api/v1/audiobooks/count');
      return res.json();
    });
    expect(response).toHaveProperty('count');
  });
});
