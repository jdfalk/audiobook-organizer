// file: web/src/components/dedup/__tests__/UnifiedDedupTab.test.tsx
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-defa-444567890123
// last-edited: 2026-06-10

import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { UnifiedDedupTab } from '../UnifiedDedupTab';
import * as api from '../../../services/api';

// Mock the full api module.
vi.mock('../../../services/api', () => ({
  getDedupCandidates: vi.fn(),
  getDedupStats: vi.fn(),
  mergeDedupCandidate: vi.fn(),
  dismissDedupCandidate: vi.fn(),
  bulkMergeDedupCandidates: vi.fn(),
  rescoreDedupCandidates: vi.fn(),
  triggerDedupScan: vi.fn(),
}));

// Mock the operations store used inside trackOp.
vi.mock('../../../stores/useOperationsStore', () => ({
  useOperationsStore: {
    getState: () => ({ startPolling: vi.fn() }),
  },
}));

const mockCandidate = {
  id: 1,
  entity_type: 'book' as const,
  entity_a_id: '01ABCDEFGHIJKLMNOPQRSTUV01',
  entity_b_id: '01ABCDEFGHIJKLMNOPQRSTUV02',
  layer: 'embedding' as const,
  similarity: 0.95,
  status: 'pending' as const,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  band: 'HIGH' as const,
  score: 92.5,
};

function renderInRouter() {
  return render(
    <MemoryRouter>
      <UnifiedDedupTab />
    </MemoryRouter>
  );
}

describe('UnifiedDedupTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: fetch returns an empty candidate list.
    vi.mocked(api.getDedupCandidates).mockResolvedValue({
      candidates: [],
      total: 0,
    });
    vi.mocked(api.getDedupStats).mockResolvedValue({ stats: [] });

    // Mock the global fetch used inside the component for AbortController-aware calls.
    globalThis.fetch = vi.fn().mockImplementation((url: string) => {
      if (url.includes('/dedup/stats')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ data: { stats: [] } }),
        });
      }
      if (url.includes('/dedup/candidates')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ data: { candidates: [], total: 0 } }),
        });
      }
      return Promise.resolve({ ok: false, status: 404, json: () => Promise.resolve({}) });
    }) as typeof fetch;
  });

  it('renders the band filter bar', async () => {
    renderInRouter();
    expect(screen.getByTestId('band-filter-bar')).toBeInTheDocument();
  });

  it('renders empty state when no candidates', async () => {
    renderInRouter();
    await waitFor(() => {
      expect(
        screen.getByText(/No candidates found for the current filter/i)
      ).toBeInTheDocument();
    });
  });

  it('renders candidates in the table', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation((url: string) => {
      if (url.includes('/dedup/stats')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ data: { stats: [] } }),
        });
      }
      if (url.includes('/dedup/candidates')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              data: { candidates: [mockCandidate], total: 1 },
            }),
        });
      }
      return Promise.resolve({ ok: false, json: () => Promise.resolve({}) });
    });

    renderInRouter();
    await waitFor(() => {
      expect(
        screen.getByText(mockCandidate.entity_a_id)
      ).toBeInTheDocument();
    });
  });

  it('shows bulk action bar when a candidate is selected', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation((url: string) => {
      if (url.includes('/dedup/stats')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ data: { stats: [] } }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({ data: { candidates: [mockCandidate], total: 1 } }),
      });
    });

    renderInRouter();

    // Wait for table to render.
    await waitFor(() => {
      expect(screen.getByText(mockCandidate.entity_a_id)).toBeInTheDocument();
    });

    // Click the row's checkbox.
    const checkboxes = screen.getAllByRole('checkbox');
    // index 0 is the select-all, index 1 is the first row.
    fireEvent.click(checkboxes[1]);

    await waitFor(() => {
      expect(screen.getByTestId('bulk-action-bar')).toBeInTheDocument();
    });
  });

  it('shows rescore dialog when rescore button clicked', async () => {
    renderInRouter();
    fireEvent.click(screen.getByTestId('rescore-btn'));
    expect(screen.getByText(/Rescore dedup candidates/i)).toBeInTheDocument();
  });
});
