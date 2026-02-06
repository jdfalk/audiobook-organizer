// file: web/tests/e2e/utils/demo-helpers.ts
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-e1f2a3b4c5d6

import { Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

/**
 * Create a temporary demo directory structure for testing
 * Returns paths for library and import directories
 */
export async function setupDemoDirectories(): Promise<{
  tempDir: string;
  libraryPath: string;
  importPath: string;
}> {
  const tempDir = `/tmp/audiobook-demo-${Date.now()}`;
  const libraryPath = path.join(tempDir, 'library');
  const importPath = path.join(tempDir, 'import');

  // Create directory structure
  fs.mkdirSync(libraryPath, { recursive: true });
  fs.mkdirSync(importPath, { recursive: true });

  // Copy test audiobooks to import directory
  // We'll use 2-3 representative audiobooks from testdata
  const testDataSources = [
    'testdata/audio/librivox/odyssey_butler_librivox',
    'testdata/audio/librivox/moby_dick_librivox',
  ];

  for (const source of testDataSources) {
    if (fs.existsSync(source)) {
      const bookDir = path.basename(source);
      const destDir = path.join(importPath, bookDir);
      copyDirRecursive(source, destDir);
    }
  }

  return { tempDir, libraryPath, importPath };
}

/**
 * Recursively copy directory contents
 */
function copyDirRecursive(src: string, dest: string): void {
  fs.mkdirSync(dest, { recursive: true });
  const files = fs.readdirSync(src);

  for (const file of files) {
    const srcFile = path.join(src, file);
    const destFile = path.join(dest, file);
    const stat = fs.statSync(srcFile);

    if (stat.isDirectory()) {
      copyDirRecursive(srcFile, destFile);
    } else {
      fs.copyFileSync(srcFile, destFile);
    }
  }
}

/**
 * Clean up temporary demo directory
 */
export async function cleanupDemoDirectories(tempDir: string): Promise<void> {
  if (fs.existsSync(tempDir)) {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
}

/**
 * Human-like mouse movement over multiple steps
 */
export async function humanMove(
  page: Page,
  x: number,
  y: number,
  steps = 15
): Promise<void> {
  const startX = 640;
  const startY = 360;

  for (let i = 0; i <= steps; i++) {
    const progress = i / steps;
    const currentX = startX + (x - startX) * progress;
    const currentY = startY + (y - startY) * progress;
    await page.mouse.move(currentX, currentY);
    await page.waitForTimeout(5 + Math.random() * 10);
  }
}

/**
 * Type text character by character at human speed
 */
export async function humanType(page: Page, text: string): Promise<void> {
  for (const char of text) {
    await page.keyboard.type(char);
    await page.waitForTimeout(25 + Math.random() * 30);
  }
}

/**
 * Take a screenshot and log the step
 */
export async function demoScreenshot(
  page: Page,
  stepNum: number,
  description: string,
  artifactDir: string
): Promise<void> {
  const filename = `demo_${String(stepNum).padStart(2, '0')}_${description
    .toLowerCase()
    .replace(/\s+/g, '_')}.png`;
  const filepath = path.join(artifactDir, filename);

  await page.screenshot({ path: filepath, fullPage: true });
  console.log(`✓ Step ${stepNum}: ${description}`);
}
