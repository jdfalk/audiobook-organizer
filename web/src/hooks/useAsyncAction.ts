// file: web/src/hooks/useAsyncAction.ts
// version: 1.0.0
// guid: h8i9j0k1-l2m3-4567-nopq-890123456789
// last-edited: 2026-05-01

import { useState, useCallback } from 'react';

interface AsyncActionState {
  loading: boolean;
  error: string | null;
}

interface UseAsyncActionReturn<T> extends AsyncActionState {
  run: (...args: unknown[]) => Promise<T | undefined>;
  clearError: () => void;
}

/**
 * useAsyncAction wraps an async function with loading and error state.
 * Eliminates repeated useState(false) / try-finally loading boilerplate.
 *
 * @example
 * const { run: handleSave, loading, error } = useAsyncAction(async () => {
 *   await api.saveBook(book);
 * });
 */
export function useAsyncAction<T = void>(
  fn: (...args: unknown[]) => Promise<T>
): UseAsyncActionReturn<T> {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(
    async (...args: unknown[]): Promise<T | undefined> => {
      setLoading(true);
      setError(null);
      try {
        return await fn(...args);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        return undefined;
      } finally {
        setLoading(false);
      }
    },
    [fn]
  );

  const clearError = useCallback(() => setError(null), []);

  return { loading, error, run, clearError };
}

export type { AsyncActionState, UseAsyncActionReturn };
