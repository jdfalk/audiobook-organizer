// file: web/src/stores/useAppStore.ts
// version: 1.2.0
// guid: 1e2f3a4b-5c6d-7e8f-9a0b-1c2d3e4f5a6b

import { create } from 'zustand';
import { devtools } from 'zustand/middleware';

type ThemeMode = 'dark' | 'light';

const THEME_MODE_STORAGE_KEY = 'app-theme-mode';

function readStoredThemeMode(): ThemeMode {
  if (typeof window === 'undefined') {
    return 'dark';
  }
  const stored = window.localStorage.getItem(THEME_MODE_STORAGE_KEY);
  return stored === 'light' || stored === 'dark' ? stored : 'dark';
}

function persistThemeMode(mode: ThemeMode): void {
  if (typeof window === 'undefined') {
    return;
  }
  window.localStorage.setItem(THEME_MODE_STORAGE_KEY, mode);
}

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

  // Theme preferences
  themeMode: ThemeMode;
  setThemeMode: (mode: ThemeMode) => void;
  toggleThemeMode: () => void;

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

      // Theme preferences
      themeMode: readStoredThemeMode(),
      setThemeMode: (mode) => {
        persistThemeMode(mode);
        set({ themeMode: mode });
      },
      toggleThemeMode: () =>
        set((state) => {
          const nextMode = state.themeMode === 'dark' ? 'light' : 'dark';
          persistThemeMode(nextMode);
          return { themeMode: nextMode };
        }),

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
        // Auto-remove success/info after 5 seconds; error/warning persist
        if (severity === 'success' || severity === 'info') {
          setTimeout(() => {
            set((state) => ({
              notifications: state.notifications.filter((n) => n.id !== id),
            }));
          }, 5000);
        }
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
