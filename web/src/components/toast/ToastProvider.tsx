// file: web/src/components/toast/ToastProvider.tsx
// version: 1.0.0
// guid: 21680277-8dde-49a7-b06e-e4d2de977e04

import { createContext, useCallback, useContext, type ReactNode } from 'react';
import { Alert, Snackbar, Stack } from '@mui/material';
import { useAppStore } from '../../stores/useAppStore';

type ToastSeverity = 'success' | 'error' | 'warning' | 'info';

interface ToastContextType {
  toast: (message: string, severity?: ToastSeverity) => void;
}

const ToastContext = createContext<ToastContextType>({
  toast: () => {},
});

/** Returns the toast API for displaying notifications. */
export function useToast(): ToastContextType {
  return useContext(ToastContext);
}

interface ToastProviderProps {
  children: ReactNode;
}

/** Provides toast notifications to descendant components. */
export function ToastProvider({ children }: ToastProviderProps): JSX.Element {
  const notifications = useAppStore((state) => state.notifications);
  const addNotification = useAppStore((state) => state.addNotification);
  const removeNotification = useAppStore((state) => state.removeNotification);

  const toast = useCallback(
    (message: string, severity: ToastSeverity = 'info') => {
      addNotification(message, severity);
    },
    [addNotification]
  );

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <Stack
        spacing={1}
        sx={{ position: 'fixed', bottom: 16, left: 16, zIndex: 1400 }}
      >
        {notifications.map((notification) => (
          <Snackbar
            key={notification.id}
            open
            autoHideDuration={4500}
            onClose={() => removeNotification(notification.id)}
            sx={{ position: 'relative', bottom: 'auto', left: 'auto' }}
          >
            <Alert
              severity={notification.severity}
              onClose={() => removeNotification(notification.id)}
              variant="filled"
              sx={{ width: '100%', minWidth: 280, maxWidth: 420 }}
            >
              {notification.message}
            </Alert>
          </Snackbar>
        ))}
      </Stack>
    </ToastContext.Provider>
  );
}
