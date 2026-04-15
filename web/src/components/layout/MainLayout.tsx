// file: web/src/components/layout/MainLayout.tsx
// version: 1.1.0
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a

import { useState, ReactNode } from 'react';
import { Box } from '@mui/material';
import { Sidebar } from './Sidebar';
import { TopBar } from './TopBar';
import { usePendingFileOps } from '../../hooks/usePendingFileOps';

interface MainLayoutProps {
  children: ReactNode;
}

const DRAWER_WIDTH = 240;

export function MainLayout({ children }: MainLayoutProps) {
  const [mobileOpen, setMobileOpen] = useState(false);

  // Global poller — fires the "All tag writes complete" toast on every page
  // even if the user is not on Activity Log.
  usePendingFileOps({ fireToasts: true });

  const handleDrawerToggle = () => {
    setMobileOpen(!mobileOpen);
  };

  return (
    <Box sx={{ display: 'flex', width: '100%' }}>
      <TopBar onMenuClick={handleDrawerToggle} drawerWidth={DRAWER_WIDTH} />
      <Sidebar
        open={mobileOpen}
        onClose={handleDrawerToggle}
        drawerWidth={DRAWER_WIDTH}
      />
      <Box
        component="main"
        sx={{
          flexGrow: 1,
          p: 3,
          width: { sm: `calc(100% - ${DRAWER_WIDTH}px)` },
          mt: 8,
          height: '100vh',
          overflow: 'auto',
        }}
      >
        {children}
      </Box>
    </Box>
  );
}
