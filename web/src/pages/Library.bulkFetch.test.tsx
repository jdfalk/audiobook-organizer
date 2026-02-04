// file: web/src/pages/Library.bulkFetch.test.tsx
// version: 1.0.3
// guid: 5b7b0d6f-5c2b-4d57-9b6c-8dbb7a9e9e2c

import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
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
  getHomeDirectory: vi.fn().mockResolvedValue('/tmp'),
  getSoftDeletedBooks: vi.fn().mockResolvedValue({ items: [], count: 0 }),
  fetchBookMetadata: vi.fn().mockResolvedValue({
    message: 'Success',
    source: 'Open Library',
    book: {
      id: 'id-1',
      title: 'Test Book',
      file_path: '/tmp/book.m4b',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      author_name: 'Author',
    },
  }),
}));

describe('Library bulk metadata fetch', () => {
  it('triggers bulk fetch when confirmed', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <Library />
      </MemoryRouter>
    );

    const selectBox = await screen.findByRole('checkbox', {
      name: /select test book/i,
    });
    await user.click(selectBox);
    await waitFor(() => {
      expect(selectBox).toBeChecked();
    });

    const openButton = await screen.findByRole('button', {
      name: /bulk fetch metadata/i,
    });
    await waitFor(() => {
      expect(openButton).toBeEnabled();
    });
    await user.click(openButton);

    const confirmButton = await screen.findByRole('button', {
      name: /^fetch metadata$/i,
    });
    await user.click(confirmButton);

    const fetchMock = vi.mocked(api.fetchBookMetadata);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('id-1');
    });
  });
});
