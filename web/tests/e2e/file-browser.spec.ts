// file: web/tests/e2e/file-browser.spec.ts
// version: 1.1.0
// guid: bbd8bdb0-5dc1-448f-a520-def03ae76825
// last-edited: 2026-02-04

import { test, expect, type Page } from '@playwright/test';
import {
  mockEventSource,
  setupMockApi,
  setupPhase1ApiDriven,
} from './utils/test-helpers';

const filesystem = {
  '/': {
    path: '/',
    items: [
      { name: 'home', path: '/home', is_dir: true, excluded: false },
      { name: 'library', path: '/library', is_dir: true, excluded: false },
      {
        name: 'readme.txt',
        path: '/readme.txt',
        is_dir: false,
        size: 1024,
        excluded: false,
      },
    ],
    disk_info: {
      exists: true,
      readable: true,
      writable: true,
      total_bytes: 500 * 1024 * 1024 * 1024,
      free_bytes: 350 * 1024 * 1024 * 1024,
      library_bytes: 45 * 1024 * 1024 * 1024,
    },
  },
  '/home': {
    path: '/home',
    items: [
      { name: 'user', path: '/home/user', is_dir: true, excluded: false },
    ],
  },
  '/home/user': {
    path: '/home/user',
    items: [
      {
        name: 'audiobooks',
        path: '/home/user/audiobooks',
        is_dir: true,
        excluded: false,
      },
    ],
  },
  '/home/user/audiobooks': {
    path: '/home/user/audiobooks',
    items: [
      {
        name: 'temp',
        path: '/home/user/audiobooks/temp',
        is_dir: true,
        excluded: false,
      },
      {
        name: 'book1.m4b',
        path: '/home/user/audiobooks/book1.m4b',
        is_dir: false,
        size: 1024 * 1024,
        excluded: false,
      },
      {
        name: 'song.mp3',
        path: '/home/user/audiobooks/song.mp3',
        is_dir: false,
        size: 2048,
        excluded: false,
      },
      {
        name: 'notes.txt',
        path: '/home/user/audiobooks/notes.txt',
        is_dir: false,
        size: 512,
        excluded: false,
      },
    ],
  },
  '/home/user/audiobooks/temp': {
    path: '/home/user/audiobooks/temp',
    items: [],
  },
};

const openImportFileBrowser = async (page: Page) => {
  await setupMockApi(page, { filesystem, books: [] });
  await page.goto('/library');
  await page.waitForLoadState('networkidle');
  await page.getByRole('button', { name: 'Import Files' }).click();
  await expect(
    page.getByRole('dialog', { name: 'Import Audiobook File' })
  ).toBeVisible();
};

test.describe('File Browser', () => {
  test.beforeEach(async ({ page }) => {
    // Phase 1 setup: Reset and skip welcome wizard
    await setupPhase1ApiDriven(page);
    // Mock EventSource to prevent SSE connections
    await mockEventSource(page);
  });

  test('browses root filesystem', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);

    // Act + Assert
    await expect(page.getByRole('button', { name: 'home' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'library' })).toBeVisible();
    await expect(page.getByText('readme.txt')).toBeVisible();
  });

  test('navigates into subdirectory', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);

    // Act
    await page.getByRole('button', { name: 'home' }).click();
    await page.getByRole('button', { name: 'user' }).click();
    await page.getByRole('button', { name: 'audiobooks' }).click();

    // Assert
    await expect(page.getByText('book1.m4b')).toBeVisible();
    await expect(page.getByText('song.mp3')).toBeVisible();
  });

  test('navigates up directory hierarchy', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'home' }).click();
    await page.getByRole('button', { name: 'user' }).click();
    await page.getByRole('button', { name: 'audiobooks' }).click();

    // Act
    await page.getByRole('button', { name: 'user' }).first().click();

    // Assert
    await expect(
      page.getByRole('button', { name: 'audiobooks' })
    ).toBeVisible();
  });

  test('creates excluded folder marker', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'home' }).click();
    await page.getByRole('button', { name: 'user' }).click();
    await page.getByRole('button', { name: 'audiobooks' }).click();

    // Act
    await page.getByRole('button', { name: 'temp' }).click({ button: 'right' });
    await page
      .getByRole('menuitem', { name: 'Exclude from scan' })
      .click();

    // Assert
    await expect(page.getByText('Folder excluded from scan.')).toBeVisible();
    await expect(page.getByText('Excluded')).toBeVisible();
  });

  test('removes excluded folder marker', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'home' }).click();
    await page.getByRole('button', { name: 'user' }).click();
    await page.getByRole('button', { name: 'audiobooks' }).click();

    await page.getByRole('button', { name: 'temp' }).click({ button: 'right' });
    await page
      .getByRole('menuitem', { name: 'Exclude from scan' })
      .click();

    // Act
    await page.getByRole('button', { name: 'temp' }).click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Include in scan' }).click();

    // Assert
    await expect(page.getByText('Folder included in scan.')).toBeVisible();
    await expect(page.getByText('Excluded')).not.toBeVisible();
  });

  test('shows disk space information', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);

    // Act + Assert
    await expect(page.getByText(/Available/i)).toBeVisible();
    await expect(page.getByText(/Library/i)).toBeVisible();
  });

  test('filters files by extension', async ({ page }) => {
    // Arrange
    await openImportFileBrowser(page);
    await page.getByRole('button', { name: 'home' }).click();
    await page.getByRole('button', { name: 'user' }).click();
    await page.getByRole('button', { name: 'audiobooks' }).click();

    // Act
    await page.getByLabel('Filter extension').fill('.m4b');

    // Assert
    await expect(page.getByText('book1.m4b')).toBeVisible();
    await expect(page.getByText('song.mp3')).not.toBeVisible();
    await expect(page.getByText('notes.txt')).not.toBeVisible();
  });

  test('selects folder as import path', async ({ page }) => {
    // Arrange
    await setupMockApi(page, { filesystem, importPaths: [] });
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    // Act
    await page.getByRole('button', { name: 'Add Import Path' }).click();
    await page
      .getByRole('button', { name: 'Browse Server Filesystem' })
      .click();
    await page.getByRole('button', { name: 'home' }).click();
    await page.getByRole('button', { name: 'user' }).click();
    await page.getByRole('button', { name: 'audiobooks' }).click();
    await page.getByRole('button', { name: 'Add Path' }).click();

    // Assert
    await expect(page.getByText('/home/user/audiobooks')).toBeVisible();
  });
});
