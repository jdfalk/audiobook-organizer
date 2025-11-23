// file: web/src/test/setup.ts
// version: 1.0.3
// guid: 8f9a0b1c-2d3e-4f5a-6b7c-8d9e0f1a2b3c

import '@testing-library/jest-dom';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// Cleanup after each test case
afterEach(() => {
  cleanup();
});

// Mock window.matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => {},
  }),
});

// Mock IntersectionObserver
global.IntersectionObserver = class IntersectionObserver {
  constructor() {}
  disconnect() {}
  observe() {}
  takeRecords() {
    return [];
  }
  unobserve() {}
} as any;

// Mock localStorage for jsdom environment
const storage = new Map<string, string>();
Object.defineProperty(global, 'localStorage', {
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => {
      storage.set(key, value);
    },
    removeItem: (key: string) => {
      storage.delete(key);
    },
    clear: () => {
      storage.clear();
    },
  },
});

// Mock EventSource (SSE) for tests
class MockEventSource {
  url: string;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readyState = 1;

  constructor(url: string) {
    this.url = url;
  }

  addEventListener() {}
  removeEventListener() {}
  close() {
    this.readyState = 2;
  }
}

// @ts-ignore
global.EventSource = MockEventSource;

// Mock fetch to avoid network calls in tests
const okJson = (data: unknown) =>
  Promise.resolve(
    new Response(JSON.stringify(data), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  );

global.fetch = (input: RequestInfo | URL) => {
  const url = typeof input === 'string' ? input : input.toString();

  if (url.includes('/api/v1/system/status')) {
    return okJson({
      status: 'ok',
      library: { book_count: 0, folder_count: 1, total_size: 0 },
      import_paths: { book_count: 0, folder_count: 0, total_size: 0 },
      memory: {},
      runtime: {},
      operations: { recent: [] },
    });
  }

  if (url.includes('/api/v1/import-paths')) {
    return okJson({ importPaths: [] });
  }

  // Default empty response
  return okJson({});
};
