// file: web/src/pages/Library.bulkFetch.test.tsx
// version: 1.0.0
// guid: 5b7b0d6f-5c2b-4d57-9b6c-8dbb7a9e9e2c

import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import { Library } from './Library';
import * as api from '../services/api';

vi.mock('../services/api', () => ({
  getActiveOperations: vi.fn().mockResolvedValue([]),
  getOperationLogsTail: vi.fn().mockResolvedValue([]),
  getBooks: vi.fn().mockResolvedValue([
    {
      id: 'id-1',
      title: 'Test Book',
      file_path: '/tmp/book.m4b',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      author_name: 'Author',
    },
  ]),
  searchBooks: vi.fn().mockResolvedValue([]),
  getImportPaths: vi.fn().mockResolvedValue([]),
  countBooks: vi.fn().mockResolvedValue(1),
  getSystemStatus: vi.fn().mockResolvedValue({
    status: 'ok',
    library: { path: '/tmp', book_count: 1, total_size: 0 },
    import_paths: { book_count: 0, folder_count: 0, total_size: 0 },
    memory: {},
    runtime: {},
    operations: { recent: [] },
  }),
  getSoftDeletedBooks: vi.fn().mockResolvedValue({ items: [], count: 0 }),
  bulkFetchMetadata: vi.fn().mockResolvedValue({
    updated_count: 1,
    total_count: 1,
    results: [],
    source: 'Open Library',
  }),
}));

describe('Library bulk metadata fetch', () => {
  it('triggers bulk fetch when confirmed', async () => {
    render(
      <MemoryRouter>
        <Library />
      </MemoryRouter>
    );

    const openButton = await screen.findByRole('button', {
      name: /bulk fetch metadata/i,
    });
    fireEvent.click(openButton);

    const confirmButton = await screen.findByRole('button', {
      name: /^fetch metadata$/i,
    });
    fireEvent.click(confirmButton);

    const bulkFetchMock = vi.mocked(api.bulkFetchMetadata);
    await waitFor(() => {
      expect(bulkFetchMock).toHaveBeenCalledWith(['id-1'], true);
    });
  });
});
