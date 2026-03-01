// file: web/src/pages/BookDetail.unlock.test.tsx
// version: 1.2.0
// guid: 2b197bb0-4a61-49ef-8b75-1f9c6c23c84e

import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import { BookDetail } from './BookDetail';
import * as api from '../services/api';

vi.mock('../services/api', () => ({
  getBook: vi.fn().mockResolvedValue({
    id: 'book-1',
    title: 'Test Title',
    file_path: '/tmp/book.m4b',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }),
  getBookVersions: vi.fn().mockResolvedValue([]),
  getBookSegments: vi.fn().mockResolvedValue([]),
  getBookTags: vi.fn().mockResolvedValue({
    tags: {
      title: {
        file_value: 'File Title',
        fetched_value: 'Fetched Title',
        stored_value: 'Stored Title',
        override_value: 'Override Title',
        override_locked: true,
        effective_value: 'Override Title',
        effective_source: 'override',
      },
    },
  }),
  updateBook: vi.fn().mockResolvedValue({
    id: 'book-1',
    title: 'Test Title',
    file_path: '/tmp/book.m4b',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }),
}));

describe('BookDetail override unlock', () => {
  it('unlocks an override via update payload', async () => {
    render(
      <MemoryRouter initialEntries={['/library/book-1']}>
        <Routes>
          <Route path="/library/:id" element={<BookDetail />} />
        </Routes>
      </MemoryRouter>
    );

    const tagsTab = await screen.findByRole('tab', { name: /tags/i });
    fireEvent.click(tagsTab);

    const unlockButton = await screen.findByRole('button', { name: 'Unlock' });
    fireEvent.click(unlockButton);

    const updateBookMock = vi.mocked(api.updateBook);
    await waitFor(() => {
      expect(updateBookMock).toHaveBeenCalledWith('book-1', {
        overrides: { title: { locked: false } },
      });
    });
  });
});
