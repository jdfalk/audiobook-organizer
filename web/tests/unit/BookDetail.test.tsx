// file: web/tests/unit/BookDetail.test.tsx
// version: 1.2.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

/**
 * Unit tests for BookDetail component
 * Tests loading states, button behavior, and state management
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { BookDetail } from '../../src/pages/BookDetail';
import * as api from '../../src/services/api';

// Mock the API module
vi.mock('../../src/services/api', async () => {
  const actual = await vi.importActual<typeof import('../../src/services/api')>(
    '../../src/services/api'
  );
  return {
    ...actual,
    getBook: vi.fn(),
    getBookVersions: vi.fn(),
    getBookTags: vi.fn(),
    getBookSegments: vi.fn(),
    fetchBookMetadata: vi.fn(),
    parseAudiobookWithAI: vi.fn(),
    updateBook: vi.fn(),
    deleteBook: vi.fn(),
    restoreSoftDeletedBook: vi.fn(),
  };
});

const navigateMock = vi.fn();

// Mock react-router-dom hooks
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useParams: () => ({ id: 'test-book-id' }),
    useNavigate: () => navigateMock,
  };
});

const mockBook = {
  id: 'test-book-id',
  title: 'The Odyssey',
  author: 'Homer',
  file_path: '/library/test.m4b',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockTags = {
  media_info: {
    codec: 'aac',
    bitrate: 128,
    sample_rate: 44100,
    channels: 2,
  },
  tags: {
    title: {
      file_value: 'The Odyssey',
      fetched_value: 'The Odyssey: Homer',
      stored_value: 'The Odyssey',
      override_value: null,
      override_locked: false,
      effective_value: 'The Odyssey',
      effective_source: 'stored',
    },
    author_name: {
      file_value: null,
      fetched_value: 'Homer',
      stored_value: null,
      override_value: null,
      override_locked: false,
      effective_value: null,
      effective_source: null,
    },
    audiobook_release_year: {
      file_value: null,
      fetched_value: 2020,
      stored_value: null,
      override_value: null,
      override_locked: false,
      effective_value: null,
      effective_source: null,
    },
  },
};

describe('BookDetail Component', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    navigateMock.mockReset();
    vi.mocked(api.getBook).mockResolvedValue(mockBook);
    vi.mocked(api.getBookVersions).mockResolvedValue([]);
    vi.mocked(api.getBookTags).mockResolvedValue(mockTags);
    vi.mocked(api.getBookSegments).mockResolvedValue([]);
  });

  it('renders book details correctly', async () => {
    render(
      <BrowserRouter>
        <BookDetail />
      </BrowserRouter>
    );

    const titleHeading = await screen.findByRole('heading', {
      name: 'The Odyssey',
    });
    expect(titleHeading).toBeInTheDocument();
  });

  it('Fetch Metadata button shows loading state', async () => {
    const user = userEvent.setup();

    vi.mocked(api.fetchBookMetadata).mockImplementation(
      () =>
        new Promise((resolve) => {
          setTimeout(
            () =>
              resolve({
                message: 'Success',
                source: 'Open Library',
                book: mockBook,
              }),
            100
          );
        })
    );

    render(
      <BrowserRouter>
        <BookDetail />
      </BrowserRouter>
    );

    await screen.findByRole('heading', { name: 'The Odyssey' });

    const fetchButton = screen.getByRole('button', { name: /fetch metadata/i });
    expect(fetchButton).toHaveTextContent('Fetch Metadata');

    await user.click(fetchButton);

    // Should show loading state
    expect(fetchButton).toHaveTextContent('Fetching...');
    expect(fetchButton).toBeDisabled();

    // Wait for completion - after fetch, loadBook() is called which may briefly show loading
    await waitFor(
      () => {
        const btn = screen.getByRole('button', { name: /fetch metadata/i });
        expect(btn).toHaveTextContent('Fetch Metadata');
        expect(btn).not.toBeDisabled();
      },
      { timeout: 3000 }
    );
  });

  it('Parse with AI button shows loading state', async () => {
    const user = userEvent.setup();

    vi.mocked(api.parseAudiobookWithAI).mockImplementation(
      () =>
        new Promise((resolve) => {
          setTimeout(
            () =>
              resolve({
                message: 'Success',
                book: mockBook,
              }),
            100
          );
        })
    );

    render(
      <BrowserRouter>
        <BookDetail />
      </BrowserRouter>
    );

    await screen.findByRole('heading', { name: 'The Odyssey' });

    const parseButton = screen.getByRole('button', { name: /parse with ai/i });
    expect(parseButton).toHaveTextContent('Parse with AI');

    await user.click(parseButton);

    // Should show loading state
    expect(parseButton).toHaveTextContent('Parsing...');
    expect(parseButton).toBeDisabled();

    await waitFor(
      () => {
        expect(parseButton).toHaveTextContent('Parse with AI');
        expect(parseButton).not.toBeDisabled();
      },
      { timeout: 2000 }
    );
  });

  it('Use Fetched button shows loading state and updates optimistically', async () => {
    const user = userEvent.setup();

    vi.mocked(api.updateBook).mockImplementation(
      () =>
        new Promise((resolve) => {
          setTimeout(() => resolve(mockBook), 100);
        })
    );

    render(
      <BrowserRouter>
        <BookDetail />
      </BrowserRouter>
    );

    await screen.findByRole('heading', { name: 'The Odyssey' });

    // Navigate to Compare tab
    const compareTab = await screen.findByRole('tab', { name: /tags/i });
    await user.click(compareTab);

    const compareTable = await screen.findByRole('table');
    expect(
      within(compareTable).getByText(/audiobook release year/i)
    ).toBeInTheDocument();

    // Find Use Fetched button
    const useFetchedButtons = within(compareTable).getAllByRole('button', {
      name: /use fetched/i,
    });
    const useFetchedButton = useFetchedButtons[0];

    await user.click(useFetchedButton);

    // Should show loading state
    expect(useFetchedButton).toHaveTextContent('Applying...');
    expect(useFetchedButton).toBeDisabled();

    await waitFor(
      () => {
        expect(useFetchedButton).not.toBeDisabled();
      },
      { timeout: 2000 }
    );
  });

  it('does not switch tabs after fetching metadata', async () => {
    const user = userEvent.setup();

    vi.mocked(api.fetchBookMetadata).mockResolvedValue({
      message: 'Success',
      source: 'Open Library',
      book: mockBook,
    });

    render(
      <BrowserRouter>
        <BookDetail />
      </BrowserRouter>
    );

    await screen.findByRole('heading', { name: 'The Odyssey' });

    // Should start on Info tab
    const infoTab = screen.getByRole('tab', { name: /^info$/i });
    expect(infoTab).toHaveAttribute('aria-selected', 'true');

    const fetchButton = screen.getByRole('button', { name: /fetch metadata/i });
    await user.click(fetchButton);

    await waitFor(() => {
      expect(api.fetchBookMetadata).toHaveBeenCalled();
    });

    // Should still be on Info tab
    expect(infoTab).toHaveAttribute('aria-selected', 'true');
  });

  it('allows other buttons to be clicked while one is loading', async () => {
    const user = userEvent.setup();

    // Make one API call slow
    vi.mocked(api.updateBook).mockImplementation(
      () =>
        new Promise((resolve) => {
          setTimeout(() => resolve(mockBook), 1000);
        })
    );

    render(
      <BrowserRouter>
        <BookDetail />
      </BrowserRouter>
    );

    await screen.findByRole('heading', { name: 'The Odyssey' });

    const compareTab = screen.getByRole('tab', { name: /tags/i });
    await user.click(compareTab);

    const compareTable = await screen.findByRole('table');
    expect(
      within(compareTable).getByText(/audiobook release year/i)
    ).toBeInTheDocument();

    const useFetchedButtons = within(compareTable).getAllByRole('button', {
      name: /use fetched/i,
    });
    const enabledButtons = useFetchedButtons.filter((button) =>
      button instanceof HTMLButtonElement ? !button.disabled : false
    );
    expect(enabledButtons.length).toBeGreaterThanOrEqual(2);

    // Click first button
    await user.click(enabledButtons[0]);
    expect(enabledButtons[0]).toBeDisabled();

    // Second button (for different field) should still be enabled
    // This tests per-field loading states
    expect(enabledButtons[1]).not.toBeDisabled();
  });
});
