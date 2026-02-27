// file: web/src/App.tsx
// version: 1.11.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f
// Trigger CI E2E test run

import { useState, useEffect, useCallback } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import {
  Box,
  Backdrop,
  CircularProgress,
  Typography,
  Stack,
} from '@mui/material';
import { MainLayout } from './components/layout/MainLayout';
import { Dashboard } from './pages/Dashboard';
import { Library } from './pages/Library';
import { BookDetail } from './pages/BookDetail';
import { Works } from './pages/Works';
import { System } from './pages/System';
import { Settings } from './pages/Settings';
import { Login } from './pages/Login';
import { FileBrowser } from './pages/FileBrowser';
import { Operations } from './pages/Operations';
import { WelcomeWizard } from './components/wizard/WelcomeWizard';
import { eventSourceManager } from './services/eventSourceManager';
import * as api from './services/api';
import { useAuth } from './contexts/AuthContext';
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts';
import { KeyboardShortcutsDialog } from './components/KeyboardShortcutsDialog';

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
      } catch (error) {
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
      } catch (e) {
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
            <Route path="/operations" element={<Operations />} />
          </Routes>
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
