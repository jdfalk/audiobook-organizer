// file: web/tests/unit/MetadataHistory.test.tsx
// version: 1.0.0
// guid: 6f7a8b9c-0d1e-2f3a-4b5c-6d7e8f9a0b1c

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MetadataHistory } from '../../src/components/MetadataHistory';
import * as api from '../../src/services/api';
import type { MetadataChangeRecord } from '../../src/services/api';

vi.mock('../../src/services/api', async () => {
  const actual = await vi.importActual<typeof import('../../src/services/api')>(
    '../../src/services/api'
  );
  return {
    ...actual,
    getBookMetadataHistory: vi.fn(),
    undoMetadataChange: vi.fn(),
  };
});

const mockGetHistory = vi.mocked(api.getBookMetadataHistory);
const mockUndo = vi.mocked(api.undoMetadataChange);

const baseRecord: MetadataChangeRecord = {
  id: 1,
  book_id: 'book-123',
  field: 'title',
  previous_value: '"Old Title"',
  new_value: '"New Title"',
  change_type: 'override',
  source: 'manual',
  changed_at: '2026-01-15T10:00:00Z',
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe('MetadataHistory', () => {
  it('renders empty state when no history', async () => {
    mockGetHistory.mockResolvedValue([]);

    render(
      <MetadataHistory
        bookId="book-123"
        open={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('No metadata changes recorded yet.')).toBeInTheDocument();
    });
  });

  it('renders change records with correct field labels', async () => {
    mockGetHistory.mockResolvedValue([
      { ...baseRecord, id: 1, field: 'title' },
      { ...baseRecord, id: 2, field: 'author_name', change_type: 'fetched' },
    ]);

    render(
      <MetadataHistory
        bookId="book-123"
        open={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Title')).toBeInTheDocument();
      expect(screen.getByText('Author')).toBeInTheDocument();
    });

    // Verify change type chips are present
    expect(screen.getByText('override')).toBeInTheDocument();
    expect(screen.getByText('fetched')).toBeInTheDocument();
  });

  it('undo button calls API and refreshes', async () => {
    mockGetHistory.mockResolvedValue([
      { ...baseRecord, id: 1, field: 'title', change_type: 'override' },
    ]);
    mockUndo.mockResolvedValue({ message: 'undo applied' });

    const onUndoComplete = vi.fn();

    render(
      <MetadataHistory
        bookId="book-123"
        open={true}
        onClose={() => {}}
        onUndoComplete={onUndoComplete}
      />
    );

    // Wait for history to load
    await waitFor(() => {
      expect(screen.getByText('Title')).toBeInTheDocument();
    });

    // Click undo button (inside the table, find the icon button)
    const undoButtons = screen.getAllByTestId ?
      screen.getAllByRole('button').filter(btn => btn.querySelector('svg[data-testid="UndoIcon"]')) :
      [];
    // Fallback: find all icon buttons that are not disabled
    const allButtons = screen.getAllByRole('button');
    const undoButton = allButtons.find(btn => btn.innerHTML.includes('UndoIcon') || btn.querySelector('svg'));
    expect(undoButton).toBeTruthy();
    await userEvent.click(undoButton!);

    await waitFor(() => {
      expect(mockUndo).toHaveBeenCalledWith('book-123', 'title');
    });

    // Should refresh history and call onUndoComplete
    await waitFor(() => {
      expect(mockGetHistory).toHaveBeenCalledTimes(2); // initial load + refresh
      expect(onUndoComplete).toHaveBeenCalled();
    });
  });

  it('only shows undo on most recent change per field', async () => {
    mockGetHistory.mockResolvedValue([
      { ...baseRecord, id: 3, field: 'title', change_type: 'override' },
      { ...baseRecord, id: 2, field: 'author_name', change_type: 'fetched' },
      { ...baseRecord, id: 1, field: 'title', change_type: 'fetched' },
    ]);

    render(
      <MetadataHistory
        bookId="book-123"
        open={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getAllByText('Title').length).toBeGreaterThanOrEqual(1);
    });

    // Should have exactly 2 undo icon buttons: one for title (id=3), one for author_name (id=2)
    // The older title record (id=1) should not have an undo button
    // Plus the Close button = 3 total buttons, but only 2 should have svg icons (undo buttons)
    const allButtons = screen.getAllByRole('button');
    const iconButtons = allButtons.filter(btn => btn.querySelector('svg'));
    expect(iconButtons).toHaveLength(2);
  });

  it('does not show undo button on undo-type changes', async () => {
    mockGetHistory.mockResolvedValue([
      { ...baseRecord, id: 1, field: 'title', change_type: 'undo' },
    ]);

    render(
      <MetadataHistory
        bookId="book-123"
        open={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Title')).toBeInTheDocument();
    });

    // Only the Close button should exist, no undo icon buttons
    const allButtons = screen.getAllByRole('button');
    const iconButtons = allButtons.filter(btn => btn.querySelector('svg'));
    expect(iconButtons).toHaveLength(0);
  });
});
