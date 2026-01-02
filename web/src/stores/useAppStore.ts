// file: web/src/stores/useAppStore.ts
// version: 1.0.0
// guid: 1e2f3a4b-5c6d-7e8f-9a0b-1c2d3e4f5a6b

import { create } from 'zustand';
import { devtools } from 'zustand/middleware';

interface Notification {
  id: string;
  message: string;
  severity: 'success' | 'error' | 'warning' | 'info';
  timestamp: number;
}

interface AppState {
  // Loading states
  isLoading: boolean;
  setLoading: (loading: boolean) => void;

  // Notifications
  notifications: Notification[];
  addNotification: (
    message: string,
    severity: Notification['severity']
  ) => void;
  removeNotification: (id: string) => void;
  clearNotifications: () => void;

  // Error handling
  error: string | null;
  setError: (error: string | null) => void;
  clearError: () => void;
}

export const useAppStore = create<AppState>()(
  devtools(
    (set) => ({
      // Loading states
      isLoading: false,
      setLoading: (loading) => set({ isLoading: loading }),

      // Notifications
      notifications: [],
      addNotification: (message, severity) => {
        const id = `${Date.now()}-${Math.random()}`;
        set((state) => ({
          notifications: [
            ...state.notifications,
            { id, message, severity, timestamp: Date.now() },
          ],
        }));
        // Auto-remove after 5 seconds
        setTimeout(() => {
          set((state) => ({
            notifications: state.notifications.filter((n) => n.id !== id),
          }));
        }, 5000);
      },
      removeNotification: (id) =>
        set((state) => ({
          notifications: state.notifications.filter((n) => n.id !== id),
        })),
      clearNotifications: () => set({ notifications: [] }),

      // Error handling
      error: null,
      setError: (error) => set({ error }),
      clearError: () => set({ error: null }),
    }),
    { name: 'AppStore' }
  )
);
