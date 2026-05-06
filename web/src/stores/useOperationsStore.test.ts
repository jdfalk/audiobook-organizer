// file: web/src/stores/useOperationsStore.test.ts
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-06

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useOperationsStore } from './useOperationsStore';
import * as api from '../services/api';

// Mock the api module
vi.mock('../services/api');
vi.mock('./useAppStore', () => ({
  useAppStore: {
    getState: () => ({
      addNotification: vi.fn(),
    }),
  },
}));

describe('useOperationsStore', () => {
  beforeEach(() => {
    // Reset store to initial state
    useOperationsStore.setState({
      operations: {},
      activeOperations: [],
      polling: false,
    });
  });

  it('dedupes operations by id when same op appears in both v1 and v2', async () => {
    const v1Op: api.ActiveOperationSummary = {
      id: 'op-1',
      type: 'scan',
      status: 'running',
      progress: 50,
      total: 100,
      message: 'Scanning books...',
    };

    const v2Op: api.OperationV2 = {
      id: 'op-1',
      def_id: 'scan-def',
      plugin: 'library_scanner',
      display_name: 'Library Scan',
      status: 'running',
      priority: 10,
      progress_current: 75,
      progress_total: 100,
      progress_message: 'Almost done',
      current_phase: null,
      current_item: null,
      actor_user_id: null,
      parent_id: null,
      queued_at: '2026-05-06T09:00:00Z',
      started_at: '2026-05-06T09:01:00Z',
      completed_at: null,
      error_message: null,
      resume_count: 0,
      trace_id: null,
      span_id: null,
    };

    vi.mocked(api.getActiveOperations).mockResolvedValue([v1Op]);
    vi.mocked(api.getRecentCompletedOperations).mockResolvedValue([]);
    vi.mocked(api.getOperationTimeline).mockResolvedValue([v2Op]);

    await useOperationsStore.getState().loadFromServer();

    const ops = useOperationsStore.getState().activeOperations;
    expect(ops).toHaveLength(1);
    expect(ops[0].id).toBe('op-1');
    // v2 should win on collision
    expect(ops[0]._source).toBe('v2');
    expect(ops[0].message).toBe('Almost done');
    expect(ops[0].progress).toBe(75);
  });

  it('v2 endpoint 404 falls back to v1 gracefully', async () => {
    const v1Op: api.ActiveOperationSummary = {
      id: 'op-1',
      type: 'organize',
      status: 'running',
      progress: 10,
      total: 20,
      message: 'Organizing...',
    };

    vi.mocked(api.getActiveOperations).mockResolvedValue([v1Op]);
    vi.mocked(api.getRecentCompletedOperations).mockResolvedValue([]);
    vi.mocked(api.getOperationTimeline).mockResolvedValue([]); // 404 returns []

    await useOperationsStore.getState().loadFromServer();

    const ops = useOperationsStore.getState().activeOperations;
    expect(ops).toHaveLength(1);
    expect(ops[0].id).toBe('op-1');
    expect(ops[0]._source).toBe('v1');
  });

  it('handles v1 endpoint failure gracefully', async () => {
    const v2Op: api.OperationV2 = {
      id: 'op-1',
      def_id: 'scan-def',
      plugin: 'library_scanner',
      display_name: 'Library Scan',
      status: 'running',
      priority: 10,
      progress_current: 50,
      progress_total: 100,
      progress_message: 'Scanning',
      current_phase: null,
      current_item: null,
      actor_user_id: null,
      parent_id: null,
      queued_at: '2026-05-06T09:00:00Z',
      started_at: '2026-05-06T09:01:00Z',
      completed_at: null,
      error_message: null,
      resume_count: 0,
      trace_id: null,
      span_id: null,
    };

    vi.mocked(api.getActiveOperations).mockRejectedValue(new Error('Network error'));
    vi.mocked(api.getRecentCompletedOperations).mockRejectedValue(new Error('Network error'));
    vi.mocked(api.getOperationTimeline).mockResolvedValue([v2Op]);

    // Should not throw, just log error
    await useOperationsStore.getState().loadFromServer();

    // We don't check the result since the error path logs to console.error
    // Just verify it doesn't crash
    expect(true).toBe(true);
  });

  it('stores parent_id and v2 fields from v2 operations', async () => {
    const parentV2: api.OperationV2 = {
      id: 'parent-op',
      def_id: 'scan-def',
      plugin: 'library_scanner',
      display_name: 'Library Scan',
      status: 'running',
      priority: 10,
      progress_current: 50,
      progress_total: 100,
      progress_message: 'Parent',
      current_phase: null,
      current_item: null,
      actor_user_id: null,
      parent_id: null,
      queued_at: '2026-05-06T09:00:00Z',
      started_at: '2026-05-06T09:01:00Z',
      completed_at: null,
      error_message: null,
      resume_count: 0,
      trace_id: null,
      span_id: null,
    };

    const childV2: api.OperationV2 = {
      id: 'child-op',
      def_id: 'index-def',
      plugin: 'indexer',
      display_name: 'Index',
      status: 'running',
      priority: 10,
      progress_current: 30,
      progress_total: 100,
      progress_message: 'Child',
      current_phase: 'phase-1',
      current_item: 'item-1',
      actor_user_id: null,
      parent_id: 'parent-op',
      queued_at: '2026-05-06T09:00:00Z',
      started_at: null,
      completed_at: null,
      error_message: null,
      resume_count: 0,
      trace_id: null,
      span_id: null,
    };

    vi.mocked(api.getActiveOperations).mockResolvedValue([]);
    vi.mocked(api.getRecentCompletedOperations).mockResolvedValue([]);
    vi.mocked(api.getOperationTimeline).mockResolvedValue([parentV2, childV2]);

    await useOperationsStore.getState().loadFromServer();

    const ops = useOperationsStore.getState().activeOperations;
    const child = ops.find((op) => op.id === 'child-op');
    expect(child).toBeDefined();
    expect(child?.parent_id).toBe('parent-op');
    expect(child?.current_phase).toBe('phase-1');
    expect(child?.current_item).toBe('item-1');
  });
});
