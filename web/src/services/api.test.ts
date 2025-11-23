// file: src/services/api.test.ts
// version: 1.0.0
// guid: 0a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d

import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  getImportPaths,
  addImportPath,
  addImportPathDetailed,
  removeImportPath,
} from './api';

const mockFetch = vi.fn();

describe('api import paths', () => {
  beforeEach(() => {
    // @ts-expect-error allow override for tests
    global.fetch = mockFetch;
  });

  afterEach(() => {
    mockFetch.mockReset();
  });

  it('getImportPaths returns import paths list', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ importPaths: [{ id: 1, path: '/tmp', name: 'Tmp', enabled: true, created_at: 'now', book_count: 0 }] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    );

    const paths = await getImportPaths();
    expect(paths).toEqual([
      { id: 1, path: '/tmp', name: 'Tmp', enabled: true, created_at: 'now', book_count: 0 },
    ]);
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/import-paths');
  });

  it('addImportPath returns created import path', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ importPath: { id: 2, path: '/new', name: 'New', enabled: true, created_at: 'now', book_count: 0 } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    );

    const created = await addImportPath('/new', 'New');
    expect(created.path).toBe('/new');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/import-paths', expect.any(Object));
  });

  it('addImportPathDetailed returns detailed response', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ importPath: { id: 3, path: '/detailed', name: 'Detailed', enabled: true, created_at: 'now', book_count: 0 }, scan_operation_id: 'op-1' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    );

    const detailed = await addImportPathDetailed('/detailed', 'Detailed');
    expect(detailed.importPath.id).toBe(3);
    expect(detailed.scan_operation_id).toBe('op-1');
  });

  it('removeImportPath calls delete endpoint', async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 200 }));

    await removeImportPath(4);
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/import-paths/4', { method: 'DELETE' });
  });
});
