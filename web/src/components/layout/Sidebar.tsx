// file: web/src/components/layout/Sidebar.tsx
// version: 1.8.0
// guid: 6f7a8b9c-0d1e-2f3a-4b5c-6d7e8f9a0b1c

import { useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import {
  Box,
  Collapse,
  Drawer,
  IconButton,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Toolbar,
} from '@mui/material';
import ExpandLessIcon from '@mui/icons-material/ExpandLess.js';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore.js';
import DashboardIcon from '@mui/icons-material/Dashboard.js';
import LibraryBooksIcon from '@mui/icons-material/LibraryBooks.js';
import MenuBookIcon from '@mui/icons-material/MenuBook.js';
import MonitorIcon from '@mui/icons-material/Monitor.js';
import SettingsIcon from '@mui/icons-material/Settings.js';
import FolderOpenIcon from '@mui/icons-material/FolderOpen.js';
import ListAltIcon from '@mui/icons-material/ListAlt.js';
import MergeTypeIcon from '@mui/icons-material/MergeType.js';
import BuildIcon from '@mui/icons-material/Build.js';
import BugReportIcon from '@mui/icons-material/BugReport.js';
import CollectionsBookmarkIcon from '@mui/icons-material/CollectionsBookmark.js';
import PeopleIcon from '@mui/icons-material/People.js';

interface SidebarProps {
  open: boolean;
  onClose: () => void;
  drawerWidth: number;
}

const librarySubItems = [
  { text: 'All Books', icon: <LibraryBooksIcon />, path: '/library' },
  { text: 'Series', icon: <CollectionsBookmarkIcon />, path: '/series' },
  { text: 'Authors', icon: <PeopleIcon />, path: '/authors' },
];

const menuItems = [
  { text: 'Dashboard', icon: <DashboardIcon />, path: '/dashboard' },
  { text: 'File Browser', icon: <FolderOpenIcon />, path: '/files' },
  { text: 'Works', icon: <MenuBookIcon />, path: '/works' },
  { text: 'Operations', icon: <ListAltIcon />, path: '/operations' },
  { text: 'Maintenance', icon: <BuildIcon />, path: '/maintenance' },
  { text: 'Dedup', icon: <MergeTypeIcon />, path: '/dedup' },
  { text: 'Diagnostics', icon: <BugReportIcon />, path: '/diagnostics' },
  { text: 'System', icon: <MonitorIcon />, path: '/system' },
  { text: 'Settings', icon: <SettingsIcon />, path: '/settings' },
];

export function Sidebar({ open, onClose, drawerWidth }: SidebarProps) {
  const navigate = useNavigate();
  const location = useLocation();

  const [libraryOpen, setLibraryOpen] = useState(true);

  const handleNavigation = (path: string) => {
    navigate(path);
    onClose();
  };

  const drawer = (
    <Box>
      <Toolbar />
      <List>
        {/* Dashboard */}
        <ListItem disablePadding>
          <ListItemButton
            selected={location.pathname === '/dashboard'}
            onClick={() => handleNavigation('/dashboard')}
          >
            <ListItemIcon><DashboardIcon /></ListItemIcon>
            <ListItemText primary="Dashboard" />
          </ListItemButton>
        </ListItem>

        {/* Library group */}
        <ListItem disablePadding secondaryAction={
          <IconButton edge="end" size="small" onClick={() => setLibraryOpen(!libraryOpen)}>
            {libraryOpen ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </IconButton>
        }>
          <ListItemButton
            selected={location.pathname === '/library'}
            onClick={() => handleNavigation('/library')}
            sx={{ pr: 5 }}
          >
            <ListItemIcon><LibraryBooksIcon /></ListItemIcon>
            <ListItemText primary="Library" />
          </ListItemButton>
        </ListItem>
        <Collapse in={libraryOpen} timeout="auto" unmountOnExit>
          <List component="div" disablePadding>
            {librarySubItems.map((item) => (
              <ListItem key={item.text} disablePadding>
                <ListItemButton
                  sx={{ pl: 4 }}
                  selected={location.pathname === item.path}
                  onClick={() => handleNavigation(item.path)}
                >
                  <ListItemIcon>{item.icon}</ListItemIcon>
                  <ListItemText primary={item.text} />
                </ListItemButton>
              </ListItem>
            ))}
          </List>
        </Collapse>

        {/* Other menu items */}
        {menuItems.slice(1).map((item) => (
          <ListItem key={item.text} disablePadding>
            <ListItemButton
              selected={location.pathname === item.path}
              onClick={() => handleNavigation(item.path)}
            >
              <ListItemIcon>{item.icon}</ListItemIcon>
              <ListItemText primary={item.text} />
            </ListItemButton>
          </ListItem>
        ))}
      </List>
    </Box>
  );

  return (
    <Box
      component="nav"
      sx={{ width: { sm: drawerWidth }, flexShrink: { sm: 0 } }}
    >
      {/* Mobile drawer */}
      <Drawer
        variant="temporary"
        open={open}
        onClose={onClose}
        ModalProps={{
          keepMounted: true,
        }}
        sx={{
          display: { xs: 'block', sm: 'none' },
          '& .MuiDrawer-paper': {
            boxSizing: 'border-box',
            width: drawerWidth,
          },
        }}
      >
        {drawer}
      </Drawer>
      {/* Desktop drawer */}
      <Drawer
        variant="permanent"
        sx={{
          display: { xs: 'none', sm: 'block' },
          '& .MuiDrawer-paper': {
            boxSizing: 'border-box',
            width: drawerWidth,
          },
        }}
        open
      >
        {drawer}
      </Drawer>
    </Box>
  );
}
