// file: web/src/pages/Login.tsx
// version: 1.0.0
// guid: 9a3f2c1d-4b5e-6f70-8a9b-0c1d2e3f4a5b

import { Box, Typography, Paper } from '@mui/material';

export function Login() {
  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        p: 3,
      }}
    >
      <Paper sx={{ p: 4, maxWidth: 480, width: '100%' }}>
        <Typography variant="h4" gutterBottom>
          Login
        </Typography>
        <Typography variant="body1" color="text.secondary">
          Your session has expired. Please sign in again to continue.
        </Typography>
      </Paper>
    </Box>
  );
}
