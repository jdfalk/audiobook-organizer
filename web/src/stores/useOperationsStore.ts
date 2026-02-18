// file: web/src/stores/useOperationsStore.ts
// version: 1.0.0
// guid: 2a3b4c5d-6e7f-8a9b-0c1d-2e3f4a5b6c7d

import { create } from 'zustand';
import * as api from '../services/api';

export interface ActiveOperation {
  id: string;
  type: string;
  status: string;
  progress: number;
  total: number;
  message: string;
}

interface OperationsState {
  activeOperations: ActiveOperation[];
  polling: boolean;

  startPolling: (operationId: string, type: string) => void;
  removeOperation: (operationId: string) => void;
  updateOperation: (op: ActiveOperation) => void;
}

export const useOperationsStore = create<OperationsState>()((set, get) => ({
  activeOperations: [],
  polling: false,

  startPolling: (operationId: string, type: string) => {
    const op: ActiveOperation = {
      id: operationId,
      type,
      status: 'queued',
      progress: 0,
      total: 0,
      message: 'Starting...',
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
        };

        get().updateOperation(updated);

        if (['completed', 'failed', 'canceled'].includes(status.status)) {
          // Keep completed ops visible for 10 seconds, then remove
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
