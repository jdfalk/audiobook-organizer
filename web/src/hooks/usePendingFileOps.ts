// file: web/src/hooks/usePendingFileOps.ts
// version: 1.0.0
// guid: 8f4a2b9c-5e1d-4f70-a8c3-2b6e9d1a0f45

import { useEffect, useRef, useState } from 'react';
import { fetchPendingFileOps, type PendingFileOp } from '../services/fileOpsApi';
import { useToast } from '../components/toast/ToastProvider';

const POLL_INTERVAL_MS = 4000;

export interface UsePendingFileOpsOptions {
  enabled?: boolean;
  /** Fire a toast when count drops from N to 0. Should only be true on one mount per app. */
  fireToasts?: boolean;
}

export interface UsePendingFileOpsResult {
  operations: PendingFileOp[];
  count: number;
  loading: boolean;
}

/**
 * Polls /api/v1/file-ops/pending. When fireToasts is true, fires a single
 * "All tag writes complete" toast when the count drops from N to 0 (rising
 * edge is suppressed since the user already knows they kicked off an apply).
 */
export function usePendingFileOps(options: UsePendingFileOpsOptions = {}): UsePendingFileOpsResult {
  const { enabled = true, fireToasts = false } = options;
  const [operations, setOperations] = useState<PendingFileOp[]>([]);
  const [loading, setLoading] = useState<boolean>(enabled);
  const { toast } = useToast();
  const previousCountRef = useRef<number>(0);
  const initialLoadRef = useRef<boolean>(true);

  useEffect(() => {
    if (!enabled) {
      setOperations([]);
      return;
    }

    let cancelled = false;

    const poll = async () => {
      try {
        const resp = await fetchPendingFileOps();
        if (cancelled) return;
        setOperations(resp.operations || []);
        setLoading(false);

        if (fireToasts) {
          const prev = previousCountRef.current;
          const next = resp.count;
          if (!initialLoadRef.current && prev > 0 && next === 0) {
            toast('All tag writes complete', 'success');
          }
          previousCountRef.current = next;
          initialLoadRef.current = false;
        }
      } catch {
        // Silent — transient network error shouldn't spam the user
      }
    };

    void poll();
    const interval = setInterval(poll, POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [enabled, fireToasts, toast]);

  return { operations, count: operations.length, loading };
}
