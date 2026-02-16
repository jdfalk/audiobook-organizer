// file: web/src/pages/Works.test.tsx
// version: 1.0.0
// guid: 6a1ce53a-f243-45d8-a45c-d3f9897179a4

import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { Works } from './Works';
import * as api from '../services/api';

vi.mock('../services/api', () => ({
  getWorks: vi.fn(),
}));

describe('Works page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows a table when works exist', async () => {
    vi.mocked(api.getWorks).mockResolvedValue([
      {
        id: 'work-1',
        title: 'The Hobbit',
        author_id: 1,
        series_id: 2,
        alt_titles: ['There and Back Again'],
      },
    ]);

    render(<Works />);

    const row = await screen.findByRole('row', { name: /The Hobbit/i });
    expect(within(row).getByText('work-1')).toBeVisible();
    expect(within(row).getAllByText('1')).toHaveLength(2);
    expect(within(row).getByText('2')).toBeVisible();
  });

  it('shows empty state when no works exist', async () => {
    vi.mocked(api.getWorks).mockResolvedValue([]);

    render(<Works />);

    expect(
      await screen.findByText(
        /No works found yet\. Works are created during scans and metadata imports\./i
      )
    ).toBeVisible();
  });

  it('shows error and retries load', async () => {
    const getWorksMock = vi
      .mocked(api.getWorks)
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValueOnce([
        {
          id: 'work-2',
          title: 'Dune',
          author_id: 3,
        },
      ]);

    render(<Works />);

    expect(await screen.findByText('boom')).toBeVisible();

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));

    await waitFor(() => {
      expect(getWorksMock).toHaveBeenCalledTimes(2);
    });
    expect(await screen.findByText('Dune')).toBeVisible();
  });
});
