// file: web/src/App.tsx
// version: 1.5.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f

import { useState, useEffect } from 'react';
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
import { WelcomeWizard } from './components/wizard/WelcomeWizard';

function App() {
  const [showWizard, setShowWizard] = useState(false);
  const [wizardCheckComplete, setWizardCheckComplete] = useState(false);
  const [serverShutdown, setServerShutdown] = useState(false);
  const [reconnectAttempts, setReconnectAttempts] = useState(0);

  useEffect(() => {
    // Check if user has completed the welcome wizard
    const wizardCompleted = localStorage.getItem('welcome_wizard_completed');
    if (!wizardCompleted) {
      setShowWizard(true);
    }
    setWizardCheckComplete(true);
  }, []);

  // Listen for server shutdown events and handle reconnection
  useEffect(() => {
    const es = new EventSource('/api/events');

    es.addEventListener('message', (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'system.shutdown') {
          setServerShutdown(true);
          es.close();
        }
      } catch (e) {
        // Ignore parse errors
      }
    });

    es.onerror = () => {
      if (serverShutdown) {
        es.close();
      }
    };

    return () => es.close();
  }, [serverShutdown]);

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
  };

  // Don't render anything until we've checked wizard status
  if (!wizardCheckComplete) {
    return null;
  }

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

      <WelcomeWizard open={showWizard} onComplete={handleWizardComplete} />

      <MainLayout>
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/library" element={<Library />} />
          <Route path="/library/:id" element={<BookDetail />} />
          <Route path="/works" element={<Works />} />
          <Route path="/system" element={<System />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </MainLayout>
    </Box>
  );
}

export default App;
