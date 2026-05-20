// file: web/src/utils/operationPolling.ts
// version: 1.1.0
// guid: 9d8c7b6a-5f4e-3d2c-1b0a-9e8d7c6b5a4f

import * as api from '../services/api';

export interface PollOptions {
  intervalMs?: number;
  timeoutMs?: number;
}

export type OperationUpdateCallback = (op: api.Operation) => void;
export type OperationCompleteCallback = (op: api.Operation) => void;
export type OperationErrorCallback = (error: unknown) => void;

/**
 * pollOperation polls an operation status until it reaches a terminal state.
 * Provides progress updates and completion notification.
 * Returns a cleanup function that should be called on component unmount.
 */
export function pollOperation(
  operationId: string,
  { intervalMs = 2000, timeoutMs = 10 * 60 * 1000 }: PollOptions = {},
  onUpdate?: OperationUpdateCallback,
  onComplete?: OperationCompleteCallback,
  onError?: OperationErrorCallback
): () => void {
  const start = Date.now();
  let timeoutId: ReturnType<typeof setTimeout> | null = null;

  const tick = async () => {
    try {
      const op = await api.getOperationStatus(operationId);
      if (!timeoutId) return; // cleanup already called
      onUpdate?.(op);
      if (isTerminal(op.status)) {
        timeoutId = null;
        onComplete?.(op);
        return; // stop polling
      }
      if (Date.now() - start < timeoutMs) {
        timeoutId = setTimeout(tick, intervalMs);
      } else {
        timeoutId = null;
        onError?.(new Error('operation polling timed out'));
      }
    } catch (e) {
      if (timeoutId) {
        // Only continue polling if timeoutId is still set (cleanup not called)
        onError?.(e);
        if (Date.now() - start < timeoutMs) {
          timeoutId = setTimeout(tick, intervalMs);
        } else {
          timeoutId = null;
        }
      }
    }
  };

  timeoutId = setTimeout(tick, intervalMs);

  // Return cleanup function to cancel polling
  return () => {
    if (timeoutId) {
      clearTimeout(timeoutId);
      timeoutId = null;
    }
  };
}

function isTerminal(status: string): boolean {
  return ['completed', 'failed', 'canceled'].includes(status);
}
