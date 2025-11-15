// file: web/src/App.tsx
// version: 1.0.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f

import { Routes, Route, Navigate } from 'react-router-dom';
import { Box } from '@mui/material';
import { MainLayout } from './components/layout/MainLayout';
import { Dashboard } from './pages/Dashboard';
import { Library } from './pages/Library';
import { Works } from './pages/Works';
import { FileManager } from './pages/FileManager';
import { Settings } from './pages/Settings';

function App() {
  return (
    <Box sx={{ display: 'flex', minHeight: '100vh' }}>
      <MainLayout>
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/library" element={<Library />} />
          <Route path="/works" element={<Works />} />
          <Route path="/files" element={<FileManager />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </MainLayout>
    </Box>
  );
}

export default App;
