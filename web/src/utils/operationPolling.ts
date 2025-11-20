// file: web/src/utils/operationPolling.ts
// version: 1.0.0
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
 */
export function pollOperation(
  operationId: string,
  { intervalMs = 2000, timeoutMs = 10 * 60 * 1000 }: PollOptions = {},
  onUpdate?: OperationUpdateCallback,
  onComplete?: OperationCompleteCallback,
  onError?: OperationErrorCallback
) {
  const start = Date.now();

  const tick = async () => {
    try {
      const op = await api.getOperationStatus(operationId);
      onUpdate?.(op);
      if (isTerminal(op.status)) {
        onComplete?.(op);
        return; // stop polling
      }
      if (Date.now() - start < timeoutMs) {
        setTimeout(tick, intervalMs);
      } else {
        onError?.(new Error('operation polling timed out'));
      }
    } catch (e) {
      onError?.(e);
      if (Date.now() - start < timeoutMs) {
        setTimeout(tick, intervalMs);
      }
    }
  };

  setTimeout(tick, intervalMs);
}

function isTerminal(status: string): boolean {
  return ['completed', 'failed', 'canceled'].includes(status);
}
