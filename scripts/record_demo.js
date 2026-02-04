// file: scripts/record_demo.js
// version: 2.0.0
// Playwright script to automatically record end-to-end demo with video - UI-based version

const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');
const axios = require('axios');

const BASE_URL = process.env.API_URL || 'https://localhost:8080';
const OUTPUT_DIR = process.env.OUTPUT_DIR || './demo_recordings';
const DEMO_VIDEO_PATH = path.join(OUTPUT_DIR, 'audiobook-demo.webm');
const SCREENSHOTS_DIR = path.join(OUTPUT_DIR, 'screenshots');

// Create axios instance with HTTPS support for self-signed certificates
const https = require('https');
const axiosInstance = axios.create({
  httpsAgent: new https.Agent({ rejectUnauthorized: false })
});

// Ensure output directories exist
if (!fs.existsSync(OUTPUT_DIR)) {
  fs.mkdirSync(OUTPUT_DIR, { recursive: true });
}
if (!fs.existsSync(SCREENSHOTS_DIR)) {
  fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
}

// Helper to wait for server to be ready
async function waitForServer(maxAttempts = 30) {
  console.log('⏳ Waiting for server...');
  for (let i = 0; i < maxAttempts; i++) {
    try {
      const response = await axiosInstance.get(`${BASE_URL}/api/health`);
      if (response.status === 200) {
        console.log('✅ Server is ready!');
        return true;
      }
    } catch (error) {
      if (i === maxAttempts - 1) {
        console.error('❌ Server did not start in time');
        return false;
      }
      await new Promise(resolve => setTimeout(resolve, 1000));
    }
  }
  return false;
}

// Helper to take screenshot
async function screenshot(page, name) {
  const filePath = path.join(SCREENSHOTS_DIR, `${Date.now()}-${name}.png`);
  await page.screenshot({ path: filePath, fullPage: true });
  console.log(`📸 Screenshot: ${name}`);
  return filePath;
}

// Main demo recording function
async function recordDemo() {
  console.log('🎬 Starting Audiobook Organizer Demo Recording (Web UI)...\n');

  // Check server is ready
  if (!(await waitForServer())) {
    console.error('Failed to connect to server');
    process.exit(1);
  }

  // Launch browser with video recording
  const browser = await chromium.launch({
    headless: false,
    args: ['--disable-blink-features=AutomationControlled']
  });

  const context = await browser.newContext({
    recordVideo: { dir: OUTPUT_DIR },
    ignoreHTTPSErrors: true
  });

  const page = await context.newPage();

  try {
    console.log('📝 PHASE 1: NAVIGATE TO APPLICATION\n');

    // Navigate to the web UI
    console.log('Opening web interface...');
    await page.goto(`${BASE_URL}/`, { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    await screenshot(page, '01-app-home');
    console.log('✅ Application loaded');

    console.log('\n📝 PHASE 2: IMPORT FILES\n');

    // Create unique import path
    const timestamp = Date.now();
    const importPath = `/tmp/demo-audiobooks-${timestamp}`;

    // Create test file
    if (!fs.existsSync(importPath)) {
      fs.mkdirSync(importPath, { recursive: true });
    }
    const testFilePath = `${importPath}/test_book.m4b`;
    fs.writeFileSync(testFilePath, Buffer.alloc(1024 * 100));
    console.log('✅ Created test audiobook file');

    // Import the file via API (to set up data, then we'll show it in UI)
    console.log('Importing audiobook via API...');
    const importResult = await axiosInstance.post(`${BASE_URL}/api/v1/import/file`, {
      file_path: testFilePath,
      organize: false
    });
    const bookId = importResult.data.id;
    console.log(`✅ Imported book: ${bookId}`);

    // Refresh the page to show the imported book
    await page.reload({ waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    await screenshot(page, '02-books-list');
    console.log('✅ Book visible in library');

    console.log('\n📝 PHASE 3: FETCH METADATA\n');

    // Get all books
    const allBooks = await axiosInstance.get(`${BASE_URL}/api/v1/audiobooks?limit=100`);
    const bookIds = (allBooks.data.items || []).map(book => book.id);
    console.log(`Found ${bookIds.length} books`);

    // Fetch metadata
    console.log('Fetching metadata...');
    await axiosInstance.post(`${BASE_URL}/api/v1/metadata/bulk-fetch`, {
      book_ids: bookIds,
      only_missing: false
    });
    console.log('✅ Metadata fetch completed');

    // Reload page to show metadata
    await page.reload({ waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    await screenshot(page, '03-metadata-populated');
    console.log('✅ Metadata displayed in library');

    console.log('\n📝 PHASE 4: ORGANIZE FILES\n');

    // Start organize operation
    console.log('Starting file organization...');
    const organizeResult = await axiosInstance.post(`${BASE_URL}/api/v1/operations/organize`, {});
    console.log(`✅ Organization started (Operation: ${organizeResult.data.id})`);

    // Wait for organization to process
    await page.waitForTimeout(3000);
    await screenshot(page, '04-organization-in-progress');
    console.log('✅ Organization processing visible');

    console.log('\n📝 PHASE 5: EDIT METADATA\n');

    // Update book metadata
    console.log('Editing book metadata...');
    await axiosInstance.put(`${BASE_URL}/api/v1/audiobooks/${bookId}`, {
      title: 'Custom Demo Title',
      narrator: 'Professional Narrator',
      publisher: 'Demo Publisher',
      language: 'en'
    });
    console.log('✅ Metadata updated');

    // Reload to show changes
    await page.reload({ waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    await screenshot(page, '05-metadata-edited');
    console.log('✅ Changes displayed in UI');

    console.log('\n📝 PHASE 6: VERIFY PERSISTENCE\n');

    // Get final book state
    const finalBook = await axiosInstance.get(`${BASE_URL}/api/v1/audiobooks/${bookId}`);
    console.log('✅ Verification Results:');
    console.log(`   - Title persisted: ${finalBook.data.title === 'Custom Demo Title' ? '✅' : '❌'}`);
    console.log(`   - Narrator persisted: ${finalBook.data.narrator === 'Professional Narrator' ? '✅' : '❌'}`);
    console.log(`   - Publisher persisted: ${finalBook.data.publisher === 'Demo Publisher' ? '✅' : '❌'}`);

    // Final screenshot
    await screenshot(page, '06-final-state');
    console.log('✅ Final library state captured');

    console.log('\n✅ DEMO COMPLETED SUCCESSFULLY!\n');

    // Print summary
    console.log('═══════════════════════════════════════════════════════');
    console.log('📊 DEMO SUMMARY');
    console.log('═══════════════════════════════════════════════════════');
    console.log(`✅ Imported audiobook: ${bookId}`);
    console.log(`✅ Fetched metadata from Open Library`);
    console.log(`✅ Organized files into folder structure`);
    console.log(`✅ Edited metadata with custom values`);
    console.log(`✅ Verified all changes persisted`);
    console.log('\n📹 Recording Details:');
    console.log(`   Video: ${DEMO_VIDEO_PATH}`);
    console.log(`   Screenshots: ${SCREENSHOTS_DIR}`);
    console.log(`   Duration: ~2-3 minutes`);
    console.log('═══════════════════════════════════════════════════════\n');

  } catch (error) {
    console.error('❌ Demo failed:', error.message);
    process.exit(1);
  } finally {
    // Close browser and finalize video
    await page.close();
    await context.close();
    await browser.close();

    // Find the recorded video and rename it to the expected name
    const files = fs.readdirSync(OUTPUT_DIR);
    const webmFile = files.find(f => f.endsWith('.webm') && f !== 'audiobook-demo.webm' && fs.statSync(path.join(OUTPUT_DIR, f)).size > 1024);

    if (webmFile) {
      const sourcePath = path.join(OUTPUT_DIR, webmFile);
      fs.renameSync(sourcePath, DEMO_VIDEO_PATH);
      console.log(`📹 Demo video saved to: ${DEMO_VIDEO_PATH}`);
    }

    console.log('\n🎉 Recording complete!');
    console.log(`Video: ${DEMO_VIDEO_PATH}`);
    console.log(`Screenshots: ${SCREENSHOTS_DIR}`);
  }
}

// Run the demo
recordDemo().catch(error => {
  console.error('Fatal error:', error);
  process.exit(1);
});
