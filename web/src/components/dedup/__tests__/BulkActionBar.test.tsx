// file: web/src/components/dedup/__tests__/BulkActionBar.test.tsx
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-333456789012
// last-edited: 2026-06-10

import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { BulkActionBar } from '../BulkActionBar';

describe('BulkActionBar', () => {
  it('renders nothing when selectedCount is 0', () => {
    const { container } = render(
      <BulkActionBar
        selectedCount={0}
        total={10}
        bandFilter={null}
        isBusy={false}
        onMergeSelected={vi.fn()}
        onDismissSelected={vi.fn()}
        onMergeAllFiltered={vi.fn()}
        onClearSelection={vi.fn()}
      />
    );
    expect(container.querySelector('[data-testid="bulk-action-bar"]')).toBeNull();
  });

  it('renders bulk action bar when selectedCount > 0', () => {
    render(
      <BulkActionBar
        selectedCount={3}
        total={10}
        bandFilter={null}
        isBusy={false}
        onMergeSelected={vi.fn()}
        onDismissSelected={vi.fn()}
        onMergeAllFiltered={vi.fn()}
        onClearSelection={vi.fn()}
      />
    );
    expect(screen.getByTestId('bulk-action-bar')).toBeInTheDocument();
    expect(screen.getByText('3 selected')).toBeInTheDocument();
    expect(screen.getByTestId('bulk-merge-selected-btn')).toBeInTheDocument();
    expect(screen.getByTestId('bulk-dismiss-selected-btn')).toBeInTheDocument();
  });

  it('calls onMergeSelected when merge selected button clicked', () => {
    const onMergeSelected = vi.fn();
    render(
      <BulkActionBar
        selectedCount={2}
        total={10}
        bandFilter={null}
        isBusy={false}
        onMergeSelected={onMergeSelected}
        onDismissSelected={vi.fn()}
        onMergeAllFiltered={vi.fn()}
        onClearSelection={vi.fn()}
      />
    );
    fireEvent.click(screen.getByTestId('bulk-merge-selected-btn'));
    expect(onMergeSelected).toHaveBeenCalledTimes(1);
  });

  it('shows confirm dialog for CERTAIN band merge-all', () => {
    render(
      <BulkActionBar
        selectedCount={2}
        total={10}
        bandFilter="CERTAIN"
        isBusy={false}
        onMergeSelected={vi.fn()}
        onDismissSelected={vi.fn()}
        onMergeAllFiltered={vi.fn()}
        onClearSelection={vi.fn()}
      />
    );
    fireEvent.click(screen.getByTestId('bulk-merge-all-btn'));
    // Dialog should appear.
    expect(screen.getByText(/Auto-merge all CERTAIN candidates/i)).toBeInTheDocument();
  });

  it('calls onMergeAllFiltered after confirming CERTAIN merge', () => {
    const onMergeAllFiltered = vi.fn();
    render(
      <BulkActionBar
        selectedCount={2}
        total={10}
        bandFilter="CERTAIN"
        isBusy={false}
        onMergeSelected={vi.fn()}
        onDismissSelected={vi.fn()}
        onMergeAllFiltered={onMergeAllFiltered}
        onClearSelection={vi.fn()}
      />
    );
    fireEvent.click(screen.getByTestId('bulk-merge-all-btn'));
    fireEvent.click(screen.getByTestId('confirm-merge-all-btn'));
    expect(onMergeAllFiltered).toHaveBeenCalledTimes(1);
  });
});
