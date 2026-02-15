// file: web/src/components/layout/TopBar.tsx
// version: 1.3.0
// guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b

import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  AppBar,
  Box,
  InputBase,
  Toolbar,
  IconButton,
  Typography,
  Chip,
  Tooltip,
} from '@mui/material';
import MenuIcon from '@mui/icons-material/Menu.js';
import Brightness4Icon from '@mui/icons-material/Brightness4.js';
import Brightness7Icon from '@mui/icons-material/Brightness7.js';
import SearchIcon from '@mui/icons-material/Search.js';
import LogoutIcon from '@mui/icons-material/Logout.js';
import { eventSourceManager } from '../../services/eventSourceManager';
import { useAppStore } from '../../stores/useAppStore';
import { useAuth } from '../../contexts/AuthContext';

interface TopBarProps {
  onMenuClick: () => void;
  drawerWidth: number;
}

export function TopBar({ onMenuClick, drawerWidth }: TopBarProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const auth = useAuth();
  const [connectionState, setConnectionState] = useState<
    'open' | 'reconnecting' | 'closed' | 'error'
  >('open');
  const [connectionMessage, setConnectionMessage] = useState<string | null>(
    null
  );
  const [searchQuery, setSearchQuery] = useState('');
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

  useEffect(() => {
    if (!location.pathname.startsWith('/library')) {
      return;
    }
    const params = new URLSearchParams(location.search);
    setSearchQuery(params.get('search') || '');
  }, [location.pathname, location.search]);

  const submitSearch = () => {
    const params = new URLSearchParams();
    if (searchQuery.trim()) {
      params.set('search', searchQuery.trim());
    }
    navigate({
      pathname: '/library',
      search: params.toString() ? `?${params.toString()}` : '',
    });
  };

  const handleLogout = async () => {
    try {
      await auth.logout();
    } finally {
      navigate('/login');
    }
  };

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
        <Typography variant="h6" noWrap component="div" sx={{ mr: 2 }}>
          Audiobook Organizer
        </Typography>
        <Box
          sx={{
            display: { xs: 'none', md: 'flex' },
            alignItems: 'center',
            px: 1.5,
            py: 0.5,
            borderRadius: 2,
            bgcolor: 'rgba(255,255,255,0.15)',
            flexGrow: 1,
            maxWidth: 520,
            mr: 2,
          }}
        >
          <SearchIcon fontSize="small" sx={{ mr: 1, opacity: 0.8 }} />
          <InputBase
            value={searchQuery}
            onChange={(event) => setSearchQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault();
                submitSearch();
              }
            }}
            placeholder="Search library..."
            inputProps={{ 'aria-label': 'Search library' }}
            sx={{ color: 'inherit', width: '100%' }}
          />
        </Box>
        <Box sx={{ flexGrow: 1, display: { xs: 'block', md: 'none' } }} />
        <Tooltip title="Search library">
          <IconButton
            color="inherit"
            aria-label="search library"
            onClick={submitSearch}
            sx={{ mr: 1, display: { xs: 'inline-flex', md: 'none' } }}
          >
            <SearchIcon />
          </IconButton>
        </Tooltip>
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
        {auth.requiresAuth && (
          <Tooltip title="Logout">
            <IconButton
              color="inherit"
              aria-label="logout"
              onClick={() => {
                void handleLogout();
              }}
              sx={{ mr: 1 }}
            >
              <LogoutIcon />
            </IconButton>
          </Tooltip>
        )}
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
