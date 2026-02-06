import { test, expect } from '@playwright/test';

test('Debug: navigate to Settings', async ({ page }) => {
  // Skip wizard
  await page.addInitScript(() => {
    localStorage.setItem('welcome_wizard_completed', 'true');
  });
  
  // Navigate to app
  console.log('🔍 Navigating to /', new Date().toISOString());
  await page.goto('/', { waitUntil: 'domcontentloaded' });
  console.log('✅ Navigated to /', new Date().toISOString());
  
  // Check what's on the page
  const navbar = page.locator('nav');
  console.log('🔍 Navbar exists:', await navbar.isVisible().catch(() => false));
  
  // Try to find Settings text
  console.log('🔍 Looking for Settings text');
  const settingsLinks = await page.getByText('Settings', { exact: true }).all();
  console.log('✅ Found', settingsLinks.length, 'Settings elements');
  
  for (let i = 0; i < settingsLinks.length; i++) {
    const el = settingsLinks[i];
    const visible = await el.isVisible().catch(() => false);
    console.log(`  Element ${i}: visible=${visible}`);
  }
  
  if (settingsLinks.length > 0) {
    console.log('🔍 Clicking first Settings element');
    await settingsLinks[0].click();
    console.log('✅ Clicked');
    
    console.log('🔍 Waiting for URL to change to /settings');
    await page.waitForURL(/.*\/settings/, { timeout: 5000 });
    console.log('✅ URL changed to /settings');
  } else {
    console.error('❌ No Settings elements found');
    // List all clickable elements
    const allText = await page.locator('*').all();
    for (const el of allText.slice(0, 20)) {
      const text = await el.textContent();
      const visible = await el.isVisible().catch(() => false);
      if (visible && text?.toLowerCase().includes('settings')) {
        console.log(`Found potential Settings element: "${text}"`);
      }
    }
  }
});
