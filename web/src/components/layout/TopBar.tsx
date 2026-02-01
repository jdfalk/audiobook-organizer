// file: web/src/components/layout/TopBar.tsx
// version: 1.2.0
// guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b

import { useEffect, useRef, useState } from 'react';
import {
  AppBar,
  Toolbar,
  IconButton,
  Typography,
  Chip,
  Tooltip,
} from '@mui/material';
import MenuIcon from '@mui/icons-material/Menu.js';
import Brightness4Icon from '@mui/icons-material/Brightness4.js';
import Brightness7Icon from '@mui/icons-material/Brightness7.js';
import { eventSourceManager } from '../../services/eventSourceManager';
import { useAppStore } from '../../stores/useAppStore';

interface TopBarProps {
  onMenuClick: () => void;
  drawerWidth: number;
}

export function TopBar({ onMenuClick, drawerWidth }: TopBarProps) {
  const [connectionState, setConnectionState] = useState<
    'open' | 'reconnecting' | 'closed' | 'error'
  >('open');
  const [connectionMessage, setConnectionMessage] = useState<string | null>(
    null
  );
  const lastStateRef = useRef(connectionState);
  const themeMode = useAppStore((state) => state.themeMode);
  const toggleThemeMode = useAppStore((state) => state.toggleThemeMode);

  useEffect(() => {
    const unsubscribe = eventSourceManager.subscribe(
      () => undefined,
      (status) => {
        const previous = lastStateRef.current;
        lastStateRef.current = status.state;
        setConnectionState(status.state);

        if (status.state === 'open') {
          if (previous !== 'open') {
            setConnectionMessage('Connection restored');
            window.setTimeout(() => setConnectionMessage(null), 3000);
          }
        } else if (
          status.state === 'reconnecting' ||
          status.state === 'error'
        ) {
          setConnectionMessage('Connection lost');
        }
      }
    );

    return () => unsubscribe();
  }, []);

  return (
    <AppBar
      position="fixed"
      sx={{
        width: { sm: `calc(100% - ${drawerWidth}px)` },
        ml: { sm: `${drawerWidth}px` },
      }}
    >
      <Toolbar>
        <IconButton
          color="inherit"
          aria-label="open drawer"
          edge="start"
          onClick={onMenuClick}
          sx={{ mr: 2, display: { sm: 'none' } }}
        >
          <MenuIcon />
        </IconButton>
        <Typography variant="h6" noWrap component="div" sx={{ flexGrow: 1 }}>
          Audiobook Organizer
        </Typography>
        <Tooltip title="Toggle color mode">
          <IconButton
            color="inherit"
            aria-label="toggle color mode"
            onClick={toggleThemeMode}
            sx={{ mr: 1 }}
          >
            {themeMode === 'dark' ? (
              <Brightness7Icon />
            ) : (
              <Brightness4Icon />
            )}
          </IconButton>
        </Tooltip>
        {connectionMessage && (
          <Chip
            label={connectionMessage}
            color={connectionState === 'open' ? 'success' : 'warning'}
            size="small"
            variant="outlined"
          />
        )}
      </Toolbar>
    </AppBar>
  );
}
