// file: web/src/utils/withOptimisticOperation.ts
// version: 1.0.0
//
// Wraps an async API call that creates a server-side operation so the bell
// reflects the user's intent IMMEDIATELY — before the network round-trip
// completes — rather than after the start-endpoint returns. The placeholder
// is reconciled to the real operation_id when the API responds, or removed
// silently if the API failed or reported "nothing to do".

import { useOperationsStore } from '../stores/useOperationsStore';

interface MaybeOperationResult {
  operation_id?: string | null;
  id?: string | null;
}

/**
 * Inserts a placeholder queued op into the bell BEFORE calling `fn`. When
 * `fn` resolves, the placeholder is renamed to the real operation id
 * (extracted via `pickId` or, by default, `result.operation_id ?? result.id`).
 * If `fn` rejects or `pickId` yields no id, the placeholder is removed.
 *
 * Typical use:
 *   const resp = await withOptimisticOperation('scan', () => api.startScan());
 *   if (!resp.operation_id) toast('Nothing to do', 'info');
 *
 * Callers no longer need a separate `startPolling(opId, type)` — the
 * reconcile step fires the same toast + loadFromServer side-effects.
 */
export async function withOptimisticOperation<T>(
  type: string,
  fn: () => Promise<T>,
  pickId: (result: T) => string | null | undefined = defaultPickId,
): Promise<T> {
  const store = useOperationsStore.getState();
  const tempId = store.beginOptimistic(type);
  try {
    const result = await fn();
    const realId = pickId(result);
    store.reconcileOptimistic(tempId, realId ?? null);
    return result;
  } catch (err) {
    store.reconcileOptimistic(tempId, null);
    throw err;
  }
}

function defaultPickId(result: unknown): string | null {
  if (!result || typeof result !== 'object') return null;
  const r = result as MaybeOperationResult;
  return r.operation_id ?? r.id ?? null;
}
