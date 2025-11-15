// file: web/src/App.test.tsx
// version: 1.0.0
// guid: 9a0b1c2d-3e4f-5a6b-7c8d-9e0f1a2b3c4d

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material';
import App from './App';
import { theme } from './theme';

describe('App', () => {
  it('renders without crashing', () => {
    render(
      <BrowserRouter>
        <ThemeProvider theme={theme}>
          <App />
        </ThemeProvider>
      </BrowserRouter>
    );
    
    expect(screen.getByText('Audiobook Organizer')).toBeInTheDocument();
  });

  it('renders navigation items', () => {
    render(
      <BrowserRouter>
        <ThemeProvider theme={theme}>
          <App />
        </ThemeProvider>
      </BrowserRouter>
    );
    
    // Check for navigation items (they appear multiple times due to responsive drawer)
    const dashboardItems = screen.getAllByText('Dashboard');
    expect(dashboardItems.length).toBeGreaterThan(0);
    
    const libraryItems = screen.getAllByText('Library');
    expect(libraryItems.length).toBeGreaterThan(0);
    
    expect(screen.getAllByText('Works').length).toBeGreaterThan(0);
    expect(screen.getAllByText('File Manager').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Settings').length).toBeGreaterThan(0);
  });
});
