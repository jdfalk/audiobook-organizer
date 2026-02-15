// file: web/tests/e2e/auth-flow.spec.ts
// version: 1.0.0
// guid: 9b9cd01d-ea34-4d87-bc84-f390b6ef10cd
// last-edited: 2026-02-15

import { test, expect } from '@playwright/test';
import { setupPhase2Interactive } from './utils/test-helpers';

test.describe('Authentication Flow', () => {
  test('redirects protected routes to login when auth is required', async ({
    page,
  }) => {
    await setupPhase2Interactive(page, undefined, {
      auth: {
        has_users: true,
        requires_auth: true,
        bootstrap_ready: false,
        login_username: 'admin',
        login_password: 'secretpass123',
      },
    });

    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login$/);
    await expect(
      page.getByRole('heading', { name: 'Login' })
    ).toBeVisible();
  });

  test('supports first-run admin setup and login', async ({ page }) => {
    await setupPhase2Interactive(page, undefined, {
      auth: {
        has_users: false,
        requires_auth: true,
        bootstrap_ready: true,
      },
    });

    await page.goto('/login');
    await expect(
      page.getByRole('heading', { name: 'Create Admin Account' })
    ).toBeVisible();

    await page.getByLabel('Username').fill('first-admin');
    await page.getByLabel('Email (optional)').fill('admin@example.com');
    await page.getByLabel('Password').fill('very-strong-password');
    await page.getByRole('button', { name: 'Create And Login' }).click();

    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.getByLabel('logout')).toBeVisible();
  });

  test('shows invalid-credential error before successful login', async ({
    page,
  }) => {
    await setupPhase2Interactive(page, undefined, {
      auth: {
        has_users: true,
        requires_auth: true,
        bootstrap_ready: false,
        login_username: 'admin',
        login_password: 'secretpass123',
      },
    });

    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('wrong-password');
    await page.getByRole('button', { name: 'Login' }).click();
    await expect(page.getByText('invalid credentials')).toBeVisible();

    await page.getByLabel('Password').fill('secretpass123');
    await page.getByRole('button', { name: 'Login' }).click();
    await expect(page).toHaveURL(/\/dashboard$/);
  });
});
