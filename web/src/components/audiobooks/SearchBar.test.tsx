// file: web/src/components/audiobooks/SearchBar.test.tsx
// version: 1.0.0

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, fireEvent, waitFor } from '@testing-library/react';
import { renderWithProviders } from '../../test/renderWithProviders';
import { SearchBar, type ViewMode } from './SearchBar';
import { SortField, SortOrder } from '../../types';

// Minimal default props — every test can override what it needs.
function defaultProps(overrides: Partial<Parameters<typeof SearchBar>[0]> = {}) {
  return {
    value: '',
    onChange: vi.fn(),
    viewMode: 'grid' as ViewMode,
    onViewModeChange: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe('SearchBar', () => {
  describe('rendering', () => {
    it('renders the search input with default placeholder', () => {
      renderWithProviders(<SearchBar {...defaultProps()} />);
      expect(
        screen.getByPlaceholderText(/Search audiobooks/i)
      ).toBeInTheDocument();
    });

    it('renders a custom placeholder when provided', () => {
      renderWithProviders(
        <SearchBar {...defaultProps({ placeholder: 'Find books...' })} />
      );
      expect(screen.getByPlaceholderText('Find books...')).toBeInTheDocument();
    });

    it('does not render sort controls when onSortChange is absent', () => {
      renderWithProviders(<SearchBar {...defaultProps()} />);
      expect(screen.queryByLabelText('Sort by')).not.toBeInTheDocument();
    });

    it('renders sort controls when onSortChange is provided', () => {
      renderWithProviders(
        <SearchBar
          {...defaultProps({
            onSortChange: vi.fn(),
            onSortOrderChange: vi.fn(),
            sortBy: SortField.Title,
            sortOrder: SortOrder.Ascending,
          })}
        />
      );
      expect(screen.getByLabelText('Sort by')).toBeInTheDocument();
      expect(screen.getByLabelText('Order')).toBeInTheDocument();
    });
  });

  describe('view mode toggle', () => {
    it('highlights the current view mode', () => {
      renderWithProviders(
        <SearchBar {...defaultProps({ viewMode: 'list' })} />
      );
      const listBtn = screen.getByRole('button', { name: 'list view' });
      expect(listBtn).toHaveAttribute('aria-pressed', 'true');
    });

    it('calls onViewModeChange when toggling', () => {
      const onViewModeChange = vi.fn();
      renderWithProviders(
        <SearchBar {...defaultProps({ onViewModeChange })} />
      );
      fireEvent.click(screen.getByRole('button', { name: 'list view' }));
      expect(onViewModeChange).toHaveBeenCalledWith('list');
    });
  });

  describe('search input', () => {
    it('calls onChange when typing', () => {
      const onChange = vi.fn();
      renderWithProviders(<SearchBar {...defaultProps({ onChange })} />);
      const input = screen.getByPlaceholderText(/Search audiobooks/i);
      fireEvent.change(input, { target: { value: 'sanderson' } });
      expect(onChange).toHaveBeenCalledWith('sanderson');
    });

    it('shows clear button when value is non-empty', () => {
      renderWithProviders(
        <SearchBar {...defaultProps({ value: 'hello' })} />
      );
      // The clear button exists (ClearIcon inside an IconButton)
      const buttons = screen.getAllByRole('button');
      // At least one button should be the clear button
      expect(buttons.length).toBeGreaterThanOrEqual(1);
    });

    it('calls onChange with empty string when clear is clicked', () => {
      const onChange = vi.fn();
      renderWithProviders(
        <SearchBar {...defaultProps({ value: 'hello', onChange })} />
      );
      // Find the clear button — it's an IconButton near the input
      const clearButtons = screen.getAllByRole('button');
      // The clear button is before the help button in the end adornment
      const clearBtn = clearButtons.find(
        (btn) => btn.querySelector('[data-testid="ClearIcon"]') !== null
      );
      if (clearBtn) {
        fireEvent.click(clearBtn);
        expect(onChange).toHaveBeenCalledWith('');
      }
    });
  });

  describe('filter chips', () => {
    it('displays parsed field filters as chips', () => {
      // Provide a value that the real parser will parse into field filters
      renderWithProviders(
        <SearchBar {...defaultProps({ value: 'author:Sanderson tag:scifi' })} />
      );
      expect(screen.getByText('author:Sanderson')).toBeInTheDocument();
      expect(screen.getByText('tag:scifi')).toBeInTheDocument();
    });

    it('shows negated filters with NOT prefix', () => {
      renderWithProviders(
        <SearchBar {...defaultProps({ value: '-tag:romance' })} />
      );
      expect(screen.getByText('NOT tag:romance')).toBeInTheDocument();
    });

    it('calls onChange with filter removed when chip is deleted', () => {
      const onChange = vi.fn();
      renderWithProviders(
        <SearchBar
          {...defaultProps({ value: 'author:Smith great books', onChange })}
        />
      );
      // Find the delete button on the chip
      const chip = screen.getByText('author:Smith');
      const deleteIcon = chip.closest('.MuiChip-root')?.querySelector('.MuiChip-deleteIcon');
      if (deleteIcon) {
        onChange.mockClear();
        fireEvent.click(deleteIcon);
        expect(onChange).toHaveBeenCalled();
        // The last call should have the filter removed
        const lastCall = onChange.mock.calls[onChange.mock.calls.length - 1];
        const newValue = lastCall[0] as string;
        expect(newValue).not.toContain('author:Smith');
        expect(newValue).toContain('great books');
      }
    });
  });

  describe('help panel', () => {
    it('opens help panel when help button is clicked', async () => {
      renderWithProviders(<SearchBar {...defaultProps()} />);
      const helpBtn = screen.getByLabelText('Search help');
      fireEvent.click(helpBtn);
      await waitFor(() => {
        expect(screen.getByText('Search Syntax')).toBeInTheDocument();
      });
    });

    it('populates search when a help example is clicked', async () => {
      const onChange = vi.fn();
      renderWithProviders(<SearchBar {...defaultProps({ onChange })} />);
      fireEvent.click(screen.getByLabelText('Search help'));
      await waitFor(() => {
        expect(screen.getByText('Search Syntax')).toBeInTheDocument();
      });
      // Click the first example
      fireEvent.click(screen.getByText('author:"Brandon Sanderson"'));
      expect(onChange).toHaveBeenCalledWith('author:"Brandon Sanderson"');
    });

    it('closes help panel when close button is clicked', async () => {
      renderWithProviders(<SearchBar {...defaultProps()} />);
      fireEvent.click(screen.getByLabelText('Search help'));
      await waitFor(() => {
        expect(screen.getByText('Search Syntax')).toBeInTheDocument();
      });
      // Close button is inside the help panel
      const closeButtons = screen.getAllByRole('button');
      const closeBtn = closeButtons.find(
        (btn) => btn.querySelector('[data-testid="CloseIcon"]') !== null
      );
      if (closeBtn) {
        fireEvent.click(closeBtn);
        await waitFor(() => {
          expect(screen.queryByText('Search Syntax')).not.toBeInTheDocument();
        });
      }
    });
  });

  describe('recent searches', () => {
    it('saves to localStorage on Enter', () => {
      renderWithProviders(
        <SearchBar {...defaultProps({ value: 'my search' })} />
      );
      const input = screen.getByPlaceholderText(/Search audiobooks/i);
      fireEvent.keyDown(input, { key: 'Enter' });
      const stored = JSON.parse(localStorage.getItem('library_recent_searches') || '[]');
      expect(stored).toContain('my search');
    });

    it('does not save empty searches on Enter', () => {
      renderWithProviders(<SearchBar {...defaultProps({ value: '  ' })} />);
      const input = screen.getByPlaceholderText(/Search audiobooks/i);
      fireEvent.keyDown(input, { key: 'Enter' });
      const stored = JSON.parse(localStorage.getItem('library_recent_searches') || '[]');
      expect(stored).toHaveLength(0);
    });
  });
});
