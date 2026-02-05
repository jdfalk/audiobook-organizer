// file: web/tests/e2e/demo-full-workflow.spec.ts
// version: 3.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-e1f2a3b4c5d6

import { test } from '@playwright/test';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { resetToFactoryDefaults } from './utils/setup-modes';

// Consistent demo artifacts directory
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const DEMO_ARTIFACTS_DIR = join(__dirname, '../../..', 'demo_artifacts');

// Realistic delay helper for human-like interactions
async function humanMove(page: any, x: number, y: number, steps = 15) {
  const startX = 640;
  const startY = 360;
  for (let i = 0; i <= steps; i++) {
    const progress = i / steps;
    const currentX = startX + (x - startX) * progress;
    const currentY = startY + (y - startY) * progress;
    await page.mouse.move(currentX, currentY);
    await page.waitForTimeout(5 + Math.random() * 10); // Variable speed like a human
  }
}

test.describe('Full End-to-End Demo Workflow', () => {
  test('Complete audiobook workflow: from empty library to organized books', async ({ page }) => {
    // Extended timeout for realistic human-paced demo
    test.setTimeout(360 * 1000); // 6 minutes for full realistic demo

    // Reset to factory defaults (keeps welcome wizard, no skipping)
    await resetToFactoryDefaults(page, 'http://127.0.0.1:8080');

    // ==============================================
    // STEP 1: Welcome Screen - First-Time User Experience
    // ==============================================
    console.log('=== STEP 1: Welcome Screen ===');
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(3000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_01_welcome_screen.png`, fullPage: true });
    console.log('✓ Welcome screen displayed');
    await page.waitForTimeout(2500);

    // ==============================================
    // STEP 2: Get Started Button - Visible Mouse Movement
    // ==============================================
    console.log('=== STEP 2: Click Get Started Button ===');

    const getStartedButton = page.locator('button').filter({ hasText: /get started|start/i }).first();
    if (await getStartedButton.isVisible().catch(() => false)) {
      // Slowly move mouse to the button
      await humanMove(page, 640, 500, 30);
      await page.waitForTimeout(500);
      await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_02_cursor_on_button.png`, fullPage: true });
      console.log('✓ Mouse hovering over Get Started');

      await page.waitForTimeout(800);
      await getStartedButton.click();
      console.log('✓ Clicked Get Started');
      await page.waitForTimeout(2000);
    }

    // ==============================================
    // STEP 3: Welcome Wizard - Step 1: Library Path
    // ==============================================
    console.log('=== STEP 3: Wizard Step 1 - Library Path ===');
    await page.waitForTimeout(1000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_03_wizard_step1_form.png`, fullPage: true });
    console.log('✓ Wizard Step 1 displayed');
    await page.waitForTimeout(1500);

    // Find the library path input field and clear it
    const libraryPathInput = page.locator('input[placeholder*="path"], input[value*="/"]').first();
    if (await libraryPathInput.isVisible().catch(() => false)) {
      // Move to input field
      await humanMove(page, 640, 400, 25);
      await page.waitForTimeout(500);

      // Click to focus
      await libraryPathInput.click();
      await page.waitForTimeout(300);

      // Select all and delete
      await page.keyboard.press('Control+A');
      await page.waitForTimeout(100);
      await page.keyboard.press('Delete');
      await page.waitForTimeout(300);

      // Type new path slowly so it's visible
      const newPath = '/Users/jdfalk/audiobooks';
      await page.waitForTimeout(300);
      for (const char of newPath) {
        await page.keyboard.type(char);
        await page.waitForTimeout(30 + Math.random() * 40); // Realistic typing speed
      }
      console.log('✓ Entered library path');
      await page.waitForTimeout(1500);

      await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_04_library_path_entered.png`, fullPage: true });
      console.log('✓ Library path configured');
    }

    await page.waitForTimeout(1500);

    // Click NEXT button
    const nextButton1 = page.locator('button').filter({ hasText: /next/i }).first();
    if (await nextButton1.isVisible().catch(() => false)) {
      await humanMove(page, 1047, 537, 25);
      await page.waitForTimeout(600);
      await nextButton1.click();
      console.log('✓ Clicked NEXT button');
      await page.waitForTimeout(2500);
    }

    // ==============================================
    // STEP 4: Welcome Wizard - Step 2: AI Setup
    // ==============================================
    console.log('=== STEP 4: Wizard Step 2 - AI Setup (Optional) ===');

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_05_wizard_step2_ai.png`, fullPage: true });
    console.log('✓ AI Setup step shown');
    await page.waitForTimeout(2000);

    // Click SKIP or NEXT
    const nextButton2 = page.locator('button').filter({ hasText: /next|skip/i }).first();
    if (await nextButton2.isVisible().catch(() => false)) {
      await humanMove(page, 1047, 537, 25);
      await page.waitForTimeout(600);
      await nextButton2.click();
      console.log('✓ Skipped AI Setup');
      await page.waitForTimeout(2500);
    }

    // ==============================================
    // STEP 5: Welcome Wizard - Step 3: Import Folders
    // ==============================================
    console.log('=== STEP 5: Wizard Step 3 - Import Folders ===');

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_06_wizard_step3_imports.png`, fullPage: true });
    console.log('✓ Import Folders step shown');
    await page.waitForTimeout(1500);

    // Look for add button and click it (optional - may not show dialog in test environment)
    const addButton = page.locator('button').filter({ hasText: /add|create|\+/i }).first();
    if (await addButton.isVisible().catch(() => false)) {
      try {
        await humanMove(page, 400, 300, 25);
        await page.waitForTimeout(1000);
        await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_07_cursor_on_add.png`, fullPage: true });
        console.log('✓ Cursor over Add button');
        await page.waitForTimeout(800);
        await addButton.click();
        console.log('✓ Clicked Add Import Path');
        await page.waitForTimeout(1500);

        // Fill in import path if dialog appears
        const importInput = page.locator('input').filter({ placeholder: /path|folder|directory/i }).first();
        if (await importInput.isVisible({ timeout: 1000 }).catch(() => false)) {
          await humanMove(page, 640, 400, 20);
          await page.waitForTimeout(400);
          await importInput.click();
          await page.waitForTimeout(300);

          // Type import path
          const importPath = '/Users/jdfalk/audiobooks';
          for (const char of importPath) {
            await page.keyboard.type(char);
            await page.waitForTimeout(25 + Math.random() * 30);
          }
          console.log('✓ Entered import path');
          await page.waitForTimeout(1000);

          await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_08_import_path_entered.png`, fullPage: true });

          // Click confirm/add button in dialog
          const confirmButton = page.locator('button').filter({ hasText: /add|confirm|save|ok/i }).last();
          if (await confirmButton.isVisible({ timeout: 1000 }).catch(() => false)) {
            await humanMove(page, 640, 500, 20);
            await page.waitForTimeout(600);
            await confirmButton.click();
            console.log('✓ Confirmed import path');
            await page.waitForTimeout(1000);
          }
        } else {
          console.log('⚠ Import dialog not found, continuing');
        }
      } catch (e) {
        console.log('⚠ Import path setup skipped, continuing to finish');
      }
    }

    await page.waitForTimeout(1500);

    // Close any open dialogs (like file browser) by pressing Escape
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);

    // Click FINISH button
    const finishButton = page.locator('button').filter({ hasText: /finish|complete|done|start/i }).first();
    if (await finishButton.isVisible().catch(() => false)) {
      await humanMove(page, 1047, 537, 20);
      await page.waitForTimeout(600);
      await finishButton.click();
      console.log('✓ Clicked FINISH - Wizard complete');
      await page.waitForTimeout(3000);
    }

    // ==============================================
    // STEP 6: Application Ready - First Look
    // ==============================================
    console.log('=== STEP 6: Application Ready ===');

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_09_app_ready.png`, fullPage: true });
    console.log('✓ Application loaded successfully');
    await page.waitForTimeout(2000);

    // ==============================================
    // STEP 7: Explore Library
    // ==============================================
    console.log('=== STEP 7: Navigate to Library ===');

    const libraryLink = page.locator('a, button').filter({ hasText: /library|books/i }).first();
    if (await libraryLink.isVisible().catch(() => false)) {
      await humanMove(page, 97, 144, 30); // Sidebar Library button
      await page.waitForTimeout(600);
      await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_10_cursor_library.png`, fullPage: true });
      await page.waitForTimeout(800);
      await libraryLink.click();
      console.log('✓ Clicked Library');
      await page.waitForTimeout(3000);
    }

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_11_library_view.png`, fullPage: true });
    console.log('✓ Library page displayed');
    await page.waitForTimeout(2000);

    // Scroll through library
    await humanMove(page, 640, 400, 15);
    await page.waitForTimeout(500);
    await page.evaluate(() => window.scrollBy(0, 300));
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_12_library_scrolled.png`, fullPage: true });
    console.log('✓ Library scrolled to show content');
    await page.waitForTimeout(1500);

    // ==============================================
    // STEP 8: Return to Dashboard
    // ==============================================
    console.log('=== STEP 8: Back to Dashboard ===');

    const dashboardLink = page.locator('a, button').filter({ hasText: /dashboard|overview/i }).first();
    if (await dashboardLink.isVisible().catch(() => false)) {
      await humanMove(page, 111, 96, 30); // Sidebar Dashboard button
      await page.waitForTimeout(600);
      await dashboardLink.click();
      console.log('✓ Clicked Dashboard');
      await page.waitForTimeout(3000);
    }

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_13_dashboard_final.png`, fullPage: true });
    console.log('✓ Dashboard final view');
    await page.waitForTimeout(1500);

    // Slow scroll through dashboard
    await humanMove(page, 640, 400, 15);
    await page.waitForTimeout(500);
    await page.evaluate(() => window.scrollBy(0, 500));
    await page.waitForTimeout(2500);

    await page.screenshot({ path: `${DEMO_ARTIFACTS_DIR}/demo_14_dashboard_scrolled.png`, fullPage: true });
    console.log('✓ Dashboard full overview');

    console.log('✅ Complete realistic demo finished successfully!');
  });
});
