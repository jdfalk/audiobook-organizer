// file: web/src/hooks/__tests__/useColumnConfig.test.ts
// version: 1.0.0
// guid: d4e5f6a7-b8c9-40d1-e2f3-a4b5c6d7e8f9

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useColumnConfig } from '../useColumnConfig';
import * as api from '../../services/api';
import { ALL_COLUMNS } from '../../config/columnDefinitions';

vi.mock('../../services/api', async () => {
  const actual = await vi.importActual('../../services/api');
  return {
    ...actual,
    getUserColumnConfig: vi.fn(),
    saveUserColumnConfig: vi.fn(),
    deleteUserColumnConfig: vi.fn(),
  };
});

const mockGetConfig = vi.mocked(api.getUserColumnConfig);
const mockSaveConfig = vi.mocked(api.saveUserColumnConfig);
const mockDeleteConfig = vi.mocked(api.deleteUserColumnConfig);

const defaultVisibleIds = ALL_COLUMNS.filter((c) => c.defaultVisible).map(
  (c) => c.id
);

describe('useColumnConfig', () => {
  beforeEach(() => {
    mockGetConfig.mockResolvedValue(null);
    mockSaveConfig.mockResolvedValue(undefined);
    mockDeleteConfig.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('uses default visible columns when no saved config', async () => {
    const { result } = renderHook(() => useColumnConfig());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.visibleColumnIds).toEqual(defaultVisibleIds);
    expect(result.current.columns.length).toBe(defaultVisibleIds.length);
    expect(result.current.columns.map((c) => c.id)).toEqual(
      defaultVisibleIds
    );
  });

  it('loads saved config from API', async () => {
    const savedConfig: api.ColumnConfig = {
      visibleColumns: ['title', 'author'],
      columnOrder: ['author', 'title'],
      columnWidths: { title: 300 },
    };
    mockGetConfig.mockResolvedValue(savedConfig);

    const { result } = renderHook(() => useColumnConfig());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.visibleColumnIds).toEqual(['title', 'author']);
    // Order: author first, then title
    expect(result.current.columns.map((c) => c.id)).toEqual([
      'author',
      'title',
    ]);
    // Title width overridden
    const titleCol = result.current.columns.find((c) => c.id === 'title');
    expect(titleCol?.defaultWidth).toBe(300);
  });

  it('toggleColumn hides a visible column', async () => {
    const { result } = renderHook(() => useColumnConfig());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    act(() => {
      result.current.toggleColumn('title');
    });

    expect(result.current.visibleColumnIds).not.toContain('title');
    expect(
      result.current.columns.find((c) => c.id === 'title')
    ).toBeUndefined();
  });

  it('toggleColumn shows a hidden column at the end', async () => {
    const { result } = renderHook(() => useColumnConfig());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // series_number is not defaultVisible
    act(() => {
      result.current.toggleColumn('series_number');
    });

    expect(result.current.visibleColumnIds).toContain('series_number');
    // Should be last
    const ids = result.current.columns.map((c) => c.id);
    expect(ids[ids.length - 1]).toBe('series_number');
  });

  it('resizeColumn persists width and debounces save', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });

    const { result } = renderHook(() => useColumnConfig());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    act(() => {
      result.current.resizeColumn('title', 400);
    });

    expect(result.current.columnWidths['title']).toBe(400);
    const titleCol = result.current.columns.find((c) => c.id === 'title');
    expect(titleCol?.defaultWidth).toBe(400);

    // Save not called yet (debounced)
    expect(mockSaveConfig).not.toHaveBeenCalled();

    // Advance past debounce
    await act(async () => {
      vi.advanceTimersByTime(600);
    });

    expect(mockSaveConfig).toHaveBeenCalledTimes(1);
    expect(mockSaveConfig).toHaveBeenCalledWith(
      expect.objectContaining({
        columnWidths: expect.objectContaining({ title: 400 }),
      })
    );

    vi.useRealTimers();
  });

  it('reorderColumns updates display order', async () => {
    const { result } = renderHook(() => useColumnConfig());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    const reversed = [...defaultVisibleIds].reverse();

    act(() => {
      result.current.reorderColumns(reversed);
    });

    expect(result.current.columns.map((c) => c.id)).toEqual(reversed);
  });

  it('resetToDefaults clears config and calls delete', async () => {
    mockGetConfig.mockResolvedValue({
      visibleColumns: ['title'],
      columnOrder: ['title'],
      columnWidths: { title: 500 },
    });

    const { result } = renderHook(() => useColumnConfig());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.visibleColumnIds).toEqual(['title']);

    act(() => {
      result.current.resetToDefaults();
    });

    expect(result.current.visibleColumnIds).toEqual(defaultVisibleIds);
    expect(result.current.columnWidths).toEqual({});
    expect(mockDeleteConfig).toHaveBeenCalledTimes(1);
  });

  it('ignores unknown column IDs from saved config', async () => {
    mockGetConfig.mockResolvedValue({
      visibleColumns: ['title', 'nonexistent_column'],
      columnOrder: ['nonexistent_column', 'title'],
      columnWidths: {},
    });

    const { result } = renderHook(() => useColumnConfig());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.visibleColumnIds).toEqual(['title']);
    expect(result.current.columns.map((c) => c.id)).toEqual(['title']);
  });
});
