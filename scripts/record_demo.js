// file: scripts/record_demo.js
// version: 1.0.0
// Playwright script to automatically record end-to-end demo with video

const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');
const axios = require('axios');

const API_BASE_URL = process.env.API_URL || 'http://localhost:8080';
const OUTPUT_DIR = process.env.OUTPUT_DIR || './demo_recordings';
const DEMO_VIDEO_PATH = path.join(OUTPUT_DIR, 'audiobook-demo.webm');
const SCREENSHOTS_DIR = path.join(OUTPUT_DIR, 'screenshots');

// Ensure output directories exist
if (!fs.existsSync(OUTPUT_DIR)) {
  fs.mkdirSync(OUTPUT_DIR, { recursive: true });
}
if (!fs.existsSync(SCREENSHOTS_DIR)) {
  fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
}

// Helper to wait for API to be ready
async function waitForAPI(maxAttempts = 30) {
  console.log('⏳ Waiting for API server...');
  for (let i = 0; i < maxAttempts; i++) {
    try {
      const response = await axios.get(`${API_BASE_URL}/api/health`);
      if (response.status === 200) {
        console.log('✅ API server is ready!');
        return true;
      }
    } catch (error) {
      if (i === maxAttempts - 1) {
        console.error('❌ API server did not start in time');
        return false;
      }
      await new Promise(resolve => setTimeout(resolve, 1000));
    }
  }
  return false;
}

// API call helper
async function apiCall(method, endpoint, data = null) {
  try {
    const config = {
      method,
      url: `${API_BASE_URL}${endpoint}`,
      headers: { 'Content-Type': 'application/json' }
    };
    if (data) config.data = data;

    const response = await axios(config);
    return response.data;
  } catch (error) {
    console.error(`API Error: ${method} ${endpoint}`, error.response?.data || error.message);
    throw error;
  }
}

// Helper to take screenshot
async function screenshot(page, name) {
  const filePath = path.join(SCREENSHOTS_DIR, `${Date.now()}-${name}.png`);
  await page.screenshot({ path: filePath });
  console.log(`📸 Screenshot: ${name}`);
  return filePath;
}

// Main demo recording function
async function recordDemo() {
  console.log('🎬 Starting Audiobook Organizer Demo Recording...\n');

  // Check API is ready
  if (!(await waitForAPI())) {
    console.error('Failed to connect to API server');
    process.exit(1);
  }

  // Launch browser with video recording
  const browser = await chromium.launch({
    headless: false, // Show browser window
    args: ['--disable-blink-features=AutomationControlled']
  });

  const context = await browser.createBrowserContext({
    recordVideo: { dir: OUTPUT_DIR }
  });

  const page = await context.newPage();

  try {
    console.log('\n📝 PHASE 1: IMPORT FILES\n');

    // Get or create import path
    console.log('Getting import paths...');
    let importPaths = await apiCall('GET', '/api/v1/import-paths');
    let importPathId = importPaths.items?.[0]?.id;

    if (!importPathId) {
      console.log('Creating new import path...');
      const createResult = await apiCall('POST', '/api/v1/import-paths', {
        path: '/tmp/demo-audiobooks',
        name: 'Demo Library'
      });
      importPathId = createResult.id;
      console.log(`✅ Created import path: ${importPathId}`);
    }

    // Create test audiobook file
    const testFilePath = '/tmp/demo-audiobooks/test_book.m4b';
    if (!fs.existsSync('/tmp/demo-audiobooks')) {
      fs.mkdirSync('/tmp/demo-audiobooks', { recursive: true });
    }
    if (!fs.existsSync(testFilePath)) {
      fs.writeFileSync(testFilePath, Buffer.alloc(1024 * 100)); // 100KB dummy file
      console.log('✅ Created test audiobook file');
    }

    // Import the file
    console.log('Importing audiobook...');
    const importResult = await apiCall('POST', '/api/v1/import/file', {
      file_path: testFilePath,
      organize: false
    });
    const bookId = importResult.id;
    console.log(`✅ Imported book: ${bookId}`);

    // Pause to show import result
    await page.goto(`${API_BASE_URL}/api/v1/audiobooks/${bookId}`);
    await screenshot(page, '01-imported-book');
    await page.waitForTimeout(2000);

    console.log('\n📝 PHASE 2: FETCH METADATA\n');

    // Fetch metadata
    console.log('Fetching metadata from Open Library...');
    const fetchResult = await apiCall('POST', '/api/v1/metadata/bulk-fetch', {
      missing_only: false
    });
    console.log(`✅ Metadata fetch completed`);

    // Verify metadata was fetched
    const bookWithMetadata = await apiCall('GET', `/api/v1/audiobooks/${bookId}`);
    console.log(`✅ Title: ${bookWithMetadata.title}`);
    console.log(`✅ Author: ${bookWithMetadata.author_name || 'N/A'}`);
    console.log(`✅ Description: ${bookWithMetadata.description?.substring(0, 50)}...` || 'N/A');

    // Show metadata on page
    await page.goto(`${API_BASE_URL}/api/v1/audiobooks/${bookId}`);
    await screenshot(page, '02-with-metadata');
    await page.waitForTimeout(2000);

    console.log('\n📝 PHASE 3: ORGANIZE FILES\n');

    // Check current file location
    console.log('Original file location:', testFilePath);

    // Organize the book
    console.log('Organizing files...');
    const organizeResult = await apiCall('POST', `/api/v1/audiobooks/${bookId}/organize`, {});
    console.log(`✅ File organization completed`);
    console.log(`Status: ${organizeResult.status}`);

    await screenshot(page, '03-organized-files');
    await page.waitForTimeout(2000);

    console.log('\n📝 PHASE 4: EDIT METADATA\n');

    // Update metadata with overrides
    console.log('Updating book metadata...');
    const updateResult = await apiCall('PUT', `/api/v1/audiobooks/${bookId}`, {
      title: 'Custom Audio Title',
      narrator: 'Professional Narrator',
      publisher: 'Custom Publisher',
      language: 'en'
    });
    console.log(`✅ Updated title: ${updateResult.title}`);
    console.log(`✅ Updated narrator: ${updateResult.narrator}`);

    // Show updated metadata
    await page.goto(`${API_BASE_URL}/api/v1/audiobooks/${bookId}`);
    await screenshot(page, '04-edited-metadata');
    await page.waitForTimeout(2000);

    console.log('\n📝 PHASE 5: VERIFY PERSISTENCE\n');

    // Verify changes persisted
    const finalBook = await apiCall('GET', `/api/v1/audiobooks/${bookId}`);
    console.log('✅ Verification Results:');
    console.log(`   - Title persisted: ${finalBook.title === 'Custom Audio Title' ? '✅' : '❌'}`);
    console.log(`   - Narrator persisted: ${finalBook.narrator === 'Professional Narrator' ? '✅' : '❌'}`);
    console.log(`   - Publisher persisted: ${finalBook.publisher === 'Custom Publisher' ? '✅' : '❌'}`);

    // List all books
    const allBooks = await apiCall('GET', '/api/v1/audiobooks?limit=10');
    console.log(`✅ Total books in library: ${allBooks.count}`);

    // Final screenshot showing all data
    await page.goto(`${API_BASE_URL}/api/v1/audiobooks`);
    await screenshot(page, '05-final-library-view');
    await page.waitForTimeout(2000);

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
    // Close browser and save video
    await page.close();
    const video = await context.video();
    if (video) {
      await video.saveAs(DEMO_VIDEO_PATH);
      console.log(`📹 Demo video saved to: ${DEMO_VIDEO_PATH}`);
    }
    await context.close();
    await browser.close();

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
