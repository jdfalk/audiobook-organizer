// file: web/src/test/factories.ts
// version: 1.1.0

import type { Book, Author, Series } from '../types/index';
import type { UserPlaylist } from '../services/playlistApi';
import type { UserBookState } from '../services/readingApi';

let idCounter = 0;
function nextId() {
  return `test-id-${++idCounter}`;
}

export function buildBook(overrides: Partial<Book> = {}): Book {
  const id = nextId();
  return {
    id,
    title: `Test Book ${id}`,
    author: 'Test Author',
    file_path: `/library/${id}/book.m4b`,
    library_state: 'organized',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

export function buildAuthor(overrides: Partial<Author> = {}): Author {
  return {
    id: nextId(),
    name: 'Test Author',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

export function buildSeries(overrides: Partial<Series> = {}): Series {
  return {
    id: nextId(),
    title: 'Test Series',
    total_books: 3,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

export function buildPlaylist(overrides: Partial<UserPlaylist> = {}): UserPlaylist {
  return {
    id: nextId(),
    name: 'Test Playlist',
    type: 'static',
    book_ids: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    dirty: false,
    version: 1,
    ...overrides,
  };
}

export function buildBookState(overrides: Partial<UserBookState> = {}): UserBookState {
  return {
    user_id: 'user-1',
    book_id: nextId(),
    status: 'unstarted',
    status_manual: false,
    last_activity_at: '2026-01-01T00:00:00Z',
    progress_pct: 0,
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}
