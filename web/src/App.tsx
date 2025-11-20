// file: web/src/App.tsx
// version: 1.3.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f

import { useState, useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { Box } from '@mui/material';
import { MainLayout } from './components/layout/MainLayout';
import { Dashboard } from './pages/Dashboard';
import { Library } from './pages/Library';
import { Works } from './pages/Works';
import { System } from './pages/System';
import { Settings } from './pages/Settings';
import { WelcomeWizard } from './components/wizard/WelcomeWizard';

function App() {
  const [showWizard, setShowWizard] = useState(false);
  const [wizardCheckComplete, setWizardCheckComplete] = useState(false);

  useEffect(() => {
    // Check if user has completed the welcome wizard
    const wizardCompleted = localStorage.getItem('welcome_wizard_completed');
    if (!wizardCompleted) {
      setShowWizard(true);
    }
    setWizardCheckComplete(true);
  }, []);

  const handleWizardComplete = () => {
    setShowWizard(false);
  };

  // Don't render anything until we've checked wizard status
  if (!wizardCheckComplete) {
    return null;
  }

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh' }}>
      <WelcomeWizard open={showWizard} onComplete={handleWizardComplete} />

      <MainLayout>
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/library" element={<Library />} />
          <Route path="/works" element={<Works />} />
          <Route path="/system" element={<System />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </MainLayout>
    </Box>
  );
}

export default App;
