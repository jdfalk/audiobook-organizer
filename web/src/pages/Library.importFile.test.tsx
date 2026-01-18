// file: web/src/pages/Library.importFile.test.tsx
// version: 1.0.0
// guid: 6f4a7b0d-9c9f-4f0b-8d85-1dd9e1ffb913

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
  browseFilesystem: vi.fn().mockResolvedValue({
    path: '/',
    items: [],
    disk_info: { total: 0, free: 0, available: 0 },
  }),
  importFile: vi.fn().mockResolvedValue({
    message: 'import started',
    book: { id: 'id-1', title: 'Test Book', file_path: '/tmp/book.m4b' },
  }),
}));

describe('Library import dialog', () => {
  it('imports a selected file path', async () => {
    render(
      <MemoryRouter>
        <Library />
      </MemoryRouter>
    );

    const openButton = await screen.findByRole('button', {
      name: /import files/i,
    });
    fireEvent.click(openButton);

    const pathField = await screen.findByLabelText(/import file path/i);
    fireEvent.change(pathField, { target: { value: '/tmp/book.m4b' } });

    const importButton = await screen.findByRole('button', { name: 'Import' });
    fireEvent.click(importButton);

    const importFileMock = vi.mocked(api.importFile);
    await waitFor(() => {
      expect(importFileMock).toHaveBeenCalledWith('/tmp/book.m4b', true);
    });
  });
});
