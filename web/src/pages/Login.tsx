// file: web/src/pages/Login.tsx
// version: 1.1.0
// guid: 9a3f2c1d-4b5e-6f70-8a9b-0c1d2e3f4a5b

import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Paper,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useAuth } from '../contexts/AuthContext';

type AuthMode = 'login' | 'setup';

export function Login() {
  const auth = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [mode, setMode] = useState<AuthMode>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [email, setEmail] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const redirectTo = useMemo(() => {
    const state = location.state as { from?: string } | null;
    return state?.from || '/dashboard';
  }, [location.state]);

  useEffect(() => {
    if (auth.bootstrapReady) {
      setMode('setup');
    } else {
      setMode('login');
    }
  }, [auth.bootstrapReady]);

  useEffect(() => {
    if (auth.isAuthenticated) {
      navigate(redirectTo, { replace: true });
    }
  }, [auth.isAuthenticated, navigate, redirectTo]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError('');
    setLoading(true);
    try {
      if (mode === 'setup') {
        await auth.setupAdmin({ username, password, email });
      }
      await auth.login(username, password);
      navigate(redirectTo, { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Authentication failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box
      sx={{
        width: '100%',
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        p: 3,
      }}
    >
      <Paper component="form" onSubmit={submit} sx={{ p: 4, maxWidth: 480, width: '100%' }}>
        <Stack spacing={2}>
          <Typography variant="h4" gutterBottom>
            {mode === 'setup' ? 'Create Admin Account' : 'Login'}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {mode === 'setup'
              ? 'First run detected. Create your first admin account.'
              : 'Sign in to access audiobook organizer.'}
          </Typography>

          {error && <Alert severity="error">{error}</Alert>}

          <TextField
            label="Username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
            fullWidth
          />

          {mode === 'setup' && (
            <TextField
              label="Email (optional)"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="email"
              fullWidth
            />
          )}

          <TextField
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={
              mode === 'setup' ? 'new-password' : 'current-password'
            }
            helperText={
              mode === 'setup'
                ? 'Use at least 8 characters for the admin password.'
                : undefined
            }
            required
            fullWidth
          />

          <Button
            type="submit"
            variant="contained"
            size="large"
            disabled={loading}
            fullWidth
          >
            {loading ? (
              <CircularProgress size={20} />
            ) : mode === 'setup' ? (
              'Create And Login'
            ) : (
              'Login'
            )}
          </Button>

          {auth.bootstrapReady && (
            <Button
              variant="text"
              onClick={() =>
                setMode((current) =>
                  current === 'setup' ? 'login' : 'setup'
                )
              }
              disabled={loading}
            >
              {mode === 'setup'
                ? 'Already have credentials? Login'
                : 'Need to create first admin? Setup'}
            </Button>
          )}
        </Stack>
      </Paper>
    </Box>
  );
}
