// file: web/src/App.tsx
// version: 1.17.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f
// Trigger CI E2E test run

import { useState, useEffect, useCallback, lazy, Suspense } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import {
  Box,
  Backdrop,
  CircularProgress,
  Typography,
  Stack,
} from '@mui/material';
import { MainLayout } from './components/layout/MainLayout';
import { WelcomeWizard } from './components/wizard/WelcomeWizard';
import { KeyboardShortcutsDialog } from './components/KeyboardShortcutsDialog';
import { Dashboard } from './pages/Dashboard';
import { Login } from './pages/Login';
import { eventSourceManager } from './services/eventSourceManager';
import * as api from './services/api';
import { useAuth } from './contexts/AuthContext';
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts';

// Lazy-loaded pages (code-split for smaller initial bundle)
const Library = lazy(() =>
  import('./pages/Library').then((m) => ({ default: m.Library }))
);
const BookDetail = lazy(() =>
  import('./pages/BookDetail').then((m) => ({ default: m.BookDetail }))
);
const Works = lazy(() =>
  import('./pages/Works').then((m) => ({ default: m.Works }))
);
const System = lazy(() =>
  import('./pages/System').then((m) => ({ default: m.System }))
);
const Settings = lazy(() =>
  import('./pages/Settings').then((m) => ({ default: m.Settings }))
);
const FileBrowser = lazy(() =>
  import('./pages/FileBrowser').then((m) => ({ default: m.FileBrowser }))
);
// Operations page removed — redirects to /activity
const Maintenance = lazy(() =>
  import('./pages/Maintenance').then((m) => ({ default: m.Maintenance }))
);
const BookDedup = lazy(() =>
  import('./pages/BookDedup').then((m) => ({ default: m.BookDedup }))
);
const Series = lazy(() =>
  import('./pages/Series').then((m) => ({ default: m.Series }))
);
const Authors = lazy(() =>
  import('./pages/Authors').then((m) => ({ default: m.Authors }))
);
const Diagnostics = lazy(() =>
  import('./pages/Diagnostics').then((m) => ({ default: m.Diagnostics }))
);
const ActivityLog = lazy(() => import('./pages/ActivityLog'));
const Playlists = lazy(() => import('./pages/Playlists'));
const Setup = lazy(() => import('./pages/Setup'));

function App() {
  const auth = useAuth();
  const [showWizard, setShowWizard] = useState(false);
  const [wizardCheckComplete, setWizardCheckComplete] = useState(false);
  const [serverShutdown, setServerShutdown] = useState(false);
  const [reconnectAttempts, setReconnectAttempts] = useState(0);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);

  const handleShowShortcuts = useCallback(() => setShortcutsOpen(true), []);
  useKeyboardShortcuts({ onShowHelp: handleShowShortcuts });

  useEffect(() => {
    if (!auth.initialized) {
      return;
    }
    if (auth.requiresAuth && !auth.isAuthenticated) {
      setShowWizard(false);
      setWizardCheckComplete(true);
      return;
    }

    let cancelled = false;
    const checkWizardStatus = async () => {
      try {
        const config = await api.getConfig();
        const setupComplete =
          Boolean(config.root_dir && config.root_dir.trim()) ||
          Boolean(config.setup_complete);
        if (!cancelled) {
          setShowWizard(!setupComplete);
          if (setupComplete) {
            localStorage.setItem('welcome_wizard_completed', 'true');
          }
        }
      } catch (_error) {
        const wizardCompleted = localStorage.getItem(
          'welcome_wizard_completed'
        );
        if (!cancelled) {
          setShowWizard(!wizardCompleted);
        }
      } finally {
        if (!cancelled) {
          setWizardCheckComplete(true);
        }
      }
    };

    checkWizardStatus();

    return () => {
      cancelled = true;
    };
  }, [auth.initialized, auth.isAuthenticated, auth.requiresAuth]);

  // Listen for server shutdown events and handle reconnection
  useEffect(() => {
    const unsubscribe = eventSourceManager.subscribe((event) => {
      if (event.type === 'system.shutdown') {
        setServerShutdown(true);
        eventSourceManager.close();
      }
    });

    return () => unsubscribe();
  }, []);

  // Reconnect attempts when server is down
  useEffect(() => {
    if (!serverShutdown) return;

    const interval = setInterval(async () => {
      try {
        const response = await fetch('/api/v1/health');
        if (response.ok) {
          // Server is back, reload the page
          window.location.reload();
        }
      } catch (_e) {
        // Server still down, increment attempts
        setReconnectAttempts((prev) => prev + 1);
      }
    }, 5000); // Try every 5 seconds

    return () => clearInterval(interval);
  }, [serverShutdown]);

  const handleWizardComplete = () => {
    setShowWizard(false);
    // Force a full reload so dashboard fetches fresh data after setup
    window.location.reload();
  };

  if (!auth.initialized || !wizardCheckComplete) {
    return null;
  }

  const requiresLogin = auth.requiresAuth && !auth.isAuthenticated;

  return (
    <Box sx={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      {/* Shutdown/Restart Overlay */}
      <Backdrop
        open={serverShutdown}
        sx={{ color: '#fff', zIndex: (theme) => theme.zIndex.drawer + 9999 }}
      >
        <Stack spacing={3} alignItems="center">
          <CircularProgress color="inherit" size={60} />
          <Typography variant="h5">Server Shutting Down</Typography>
          <Typography variant="body1" sx={{ opacity: 0.8 }}>
            Attempting to reconnect...
          </Typography>
          {reconnectAttempts > 0 && (
            <Typography variant="caption" sx={{ opacity: 0.6 }}>
              Attempt {reconnectAttempts}
            </Typography>
          )}
        </Stack>
      </Backdrop>

      {!requiresLogin && (
        <WelcomeWizard open={showWizard} onComplete={handleWizardComplete} />
      )}

      {requiresLogin ? (
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      ) : (
        <MainLayout>
          <Suspense fallback={<CircularProgress sx={{ m: 4 }} />}>
            <Routes>
              <Route path="/" element={<Navigate to="/dashboard" replace />} />
              <Route path="/dashboard" element={<Dashboard />} />
              <Route path="/library" element={<Library />} />
              <Route path="/library/:id" element={<BookDetail />} />
              <Route path="/works" element={<Works />} />
              <Route path="/system" element={<System />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="/login" element={<Navigate to="/dashboard" replace />} />
              <Route path="/files" element={<FileBrowser />} />
              <Route path="/operations" element={<Navigate to="/activity" replace />} />
              <Route path="/maintenance" element={<Maintenance />} />
              <Route path="/authors/dedup" element={<Navigate to="/dedup" replace />} />
              <Route path="/books/dedup" element={<Navigate to="/dedup" replace />} />
              <Route path="/dedup" element={<BookDedup />} />
              <Route path="/series" element={<Series />} />
              <Route path="/authors" element={<Authors />} />
              <Route path="/diagnostics" element={<Diagnostics />} />
              <Route path="/activity" element={<ActivityLog />} />
              <Route path="/playlists" element={<Playlists />} />
              <Route path="/setup" element={<Setup />} />
            </Routes>
          </Suspense>
        </MainLayout>
      )}
      <KeyboardShortcutsDialog
        open={shortcutsOpen}
        onClose={() => setShortcutsOpen(false)}
      />
    </Box>
  );
}

export default App;
