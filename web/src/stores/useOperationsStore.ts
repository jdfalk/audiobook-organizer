// file: web/src/stores/useOperationsStore.ts
// version: 1.3.0
// guid: 2a3b4c5d-6e7f-8a9b-0c1d-2e3f4a5b6c7d

import { create } from 'zustand';
import * as api from '../services/api';
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
}

interface OperationsState {
  activeOperations: ActiveOperation[];
  polling: boolean;

  startPolling: (operationId: string, type: string, resumed?: boolean) => void;
  removeOperation: (operationId: string) => void;
  updateOperation: (op: ActiveOperation) => void;
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

export const useOperationsStore = create<OperationsState>()((set, get) => ({
  activeOperations: [],
  polling: false,

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

    set((state) => ({
      activeOperations: [...state.activeOperations, op],
      polling: true,
    }));

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
      const activeOperations = state.activeOperations.filter(
        (op) => op.id !== operationId
      );
      return {
        activeOperations,
        polling: activeOperations.length > 0,
      };
    });
  },

  updateOperation: (updated: ActiveOperation) => {
    set((state) => ({
      activeOperations: state.activeOperations.map((op) =>
        op.id === updated.id ? updated : op
      ),
    }));
  },
}));
