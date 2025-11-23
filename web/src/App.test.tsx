// file: web/src/App.test.tsx
// version: 1.0.1
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

  it('renders navigation items', async () => {
    render(
      <BrowserRouter>
        <ThemeProvider theme={theme}>
          <App />
        </ThemeProvider>
      </BrowserRouter>
    );

    expect((await screen.findAllByText('Dashboard')).length).toBeGreaterThan(0);
    expect((await screen.findAllByText('Library')).length).toBeGreaterThan(0);
  });
});
