// file: web/src/stores/useOperationsStore.ts
// version: 2.1.0
// guid: 2a3b4c5d-6e7f-8a9b-0c1d-2e3f4a5b6c7d

import { create } from 'zustand';
import * as api from '../services/api';
import { type OperationSSEEventName } from '../services/api';
import { useAppStore } from './useAppStore';

export interface ActiveOperation {
  id: string;
  type: string;
  status: string;
  progress: number;
  total: number;
  message: string;
  startedAt?: number; // timestamp ms
  resumed?: boolean;
  // Runtime field added when operation comes from v2 source
  _source?: 'v1' | 'v2';
  // V2 fields (optional, populated when coming from v2)
  parent_id?: string | null;
  current_phase?: string | null;
  current_item?: string | null;
}

interface OperationsState {
  operations: Record<string, ActiveOperation>; // Keyed by id
  activeOperations: ActiveOperation[]; // Derived from operations
  polling: boolean;
  // SSE EventSource instance — kept here so it can be closed on unmount.
  _sseSource: EventSource | null;

  startPolling: (operationId: string, type: string, resumed?: boolean) => void;
  removeOperation: (operationId: string) => void;
  updateOperation: (op: ActiveOperation) => void;
  loadFromServer: () => Promise<void>;
  // openSSE opens the SSE connection and subscribes to op.* events.
  // Calling it again while a connection is already open is a no-op.
  openSSE: () => void;
  // closeSSE tears down the SSE connection.
  closeSSE: () => void;
}

// Converts v1 operation (ActiveOperationSummary) to unified ActiveOperation
function fromV1(op: api.ActiveOperationSummary): ActiveOperation {
  return {
    id: op.id,
    type: op.type,
    status: op.status,
    progress: op.progress,
    total: op.total,
    message: op.message,
    _source: 'v1',
  };
}

// Converts v2 operation to unified ActiveOperation
function fromV2(op: api.OperationV2): ActiveOperation {
  return {
    id: op.id,
    type: op.plugin, // In v2, the operation type is stored in 'plugin'
    status: op.status,
    progress: op.progress_current ?? 0,
    total: op.progress_total ?? 0,
    message: op.progress_message ?? op.display_name ?? '',
    _source: 'v2',
    parent_id: op.parent_id,
    current_phase: op.current_phase,
    current_item: op.current_item,
  };
}

function formatOpLabel(type: string): string {
  const labels: Record<string, string> = {
    itunes_import: 'iTunes Import',
    itunes_sync: 'iTunes Sync',
    scan: 'Library Scan',
    organize: 'Organize',
    metadata_fetch: 'Metadata Fetch',
    metadata_candidate_fetch: 'Metadata Fetch (Batch)',
    bulk_write_back: 'Tag Write-back',
    composer_tag_scan: 'Composer Tag Scan',
    isbn_enrichment: 'ISBN Enrichment',
    metadata_refresh: 'Metadata Refresh',
    reconcile_scan: 'Reconcile Scan',
    itunes_path_repair: 'iTunes Path Repair',
    series_normalize: 'Series Normalize',
    transcode: 'Transcode',
    ol_dump_import: 'Open Library Import',
    'dedup-scan': 'Dedup Scan',
    'dedup-llm-review': 'Dedup AI Review',
    'dedup-acoustid-scan': 'AcoustID Scan',
    'dedup-book-signature-scan': 'Book Signature Scan',
    'embed-scan': 'Embedding Rescan',
    'fingerprint-rescan': 'Fingerprint Rescan',
  };
  return labels[type] ?? type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

// Helper to derive activeOperations array from operations map
function deriveActiveOperations(operations: Record<string, ActiveOperation>): ActiveOperation[] {
  return Object.values(operations);
}

export const useOperationsStore = create<OperationsState>()((set, get) => ({
  operations: {},
  activeOperations: [],
  polling: false,
  _sseSource: null,

  loadFromServer: async () => {
    try {
      // Load from both v1 and v2 in parallel
      const [v1Active, v1Recent, v2Ops] = await Promise.all([
        api.getActiveOperations(),
        api.getRecentCompletedOperations(),
        api.getOperationTimeline(15),
      ]);

      set(() => {
        // Start with empty operations map
        const merged: Record<string, ActiveOperation> = {};

        // Add v1 active operations
        for (const op of v1Active) {
          merged[op.id] = fromV1(op);
        }

        // Add v1 recent (completed) operations
        for (const op of v1Recent) {
          merged[op.id] = {
            id: op.id,
            type: op.type,
            status: op.status,
            progress: op.progress,
            total: op.total,
            message: op.message,
            _source: 'v1',
          };
        }

        // Merge v2 operations (v2 wins on id collision)
        for (const op of v2Ops) {
          merged[op.id] = fromV2(op);
        }

        return {
          operations: merged,
          activeOperations: deriveActiveOperations(merged),
        };
      });
    } catch (err) {
      console.error('Failed to load operations from server', err);
    }
  },

  startPolling: (operationId: string, type: string, resumed = false) => {
    const label = formatOpLabel(type);
    const notify = useAppStore.getState().addNotification;

    notify(resumed ? `${label} resumed` : `${label} started`, 'info');

    const op: ActiveOperation = {
      id: operationId,
      type,
      status: 'queued',
      progress: 0,
      total: 0,
      message: resumed ? 'Resuming...' : 'Starting...',
      startedAt: Date.now(),
      resumed,
    };

    set((state) => {
      const operations = { ...state.operations, [operationId]: op };
      return {
        operations,
        activeOperations: deriveActiveOperations(operations),
        polling: true,
      };
    });

    const poll = async () => {
      try {
        const status = await api.getOperationStatus(operationId);
        const updated: ActiveOperation = {
          id: operationId,
          type,
          status: status.status,
          progress: status.progress,
          total: status.total,
          message: status.message,
          resumed,
        };

        get().updateOperation(updated);

        if (['completed', 'failed', 'canceled'].includes(status.status)) {
          const n = useAppStore.getState().addNotification;
          if (status.status === 'completed') {
            n(`${label} completed`, 'success');
          } else if (status.status === 'failed') {
            n(`${label} failed`, 'error');
          } else {
            n(`${label} canceled`, 'info');
          }
          setTimeout(() => get().removeOperation(operationId), 10000);
          return;
        }

        setTimeout(poll, 2000);
      } catch {
        setTimeout(poll, 5000);
      }
    };

    setTimeout(poll, 1000);
  },

  removeOperation: (operationId: string) => {
    set((state) => {
      const { [operationId]: _, ...remaining } = state.operations;
      return {
        operations: remaining,
        activeOperations: deriveActiveOperations(remaining),
        polling: Object.keys(remaining).length > 0,
      };
    });
  },

  updateOperation: (updated: ActiveOperation) => {
    set((state) => {
      const operations = {
        ...state.operations,
        [updated.id]: updated,
      };
      return {
        operations,
        activeOperations: deriveActiveOperations(operations),
      };
    });
  },

  openSSE: () => {
    // Guard: don't open a second connection if one is already active.
    if (get()._sseSource !== null) return;

    const es = api.openOperationsSSE({
      onEvent: (name: OperationSSEEventName, payload: unknown) => {
        const p = payload as Record<string, unknown>;
        const opId = (p?.op_id ?? '') as string;

        if (name === 'op.created') {
          // A new v2 op appeared — re-fetch the full timeline to pick it up.
          get().loadFromServer();
        } else if (name === 'op.updated' && opId) {
          // Partial progress update: merge into existing operation if present.
          set((state) => {
            const existing = state.operations[opId];
            if (!existing) return state;
            const updated: ActiveOperation = {
              ...existing,
              progress: (p.progress_current as number | undefined) ?? existing.progress,
              total: (p.progress_total as number | undefined) ?? existing.total,
              message: (p.message as string | undefined) ?? existing.message,
            };
            const operations = { ...state.operations, [opId]: updated };
            return { operations, activeOperations: deriveActiveOperations(operations) };
          });
        } else if (name === 'op.terminal' && opId) {
          // Operation reached a terminal state — refresh from server.
          get().loadFromServer();
        }
        // op.log is informational; no store update needed (logs are fetched on-demand).
      },
      onError: () => {
        // On error, clear the source so the next openSSE() call reconnects.
        // The browser EventSource already retries automatically, but if the
        // connection is truly closed we want the next call to re-open it.
        set({ _sseSource: null });
      },
    });

    set({ _sseSource: es });
  },

  closeSSE: () => {
    const es = get()._sseSource;
    if (es) {
      es.close();
      set({ _sseSource: null });
    }
  },
}));
