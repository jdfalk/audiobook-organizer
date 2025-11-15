// file: web/src/pages/Dashboard.tsx
// version: 1.0.0
// guid: 2f3a4b5c-6d7e-8f9a-0b1c-2d3e4f5a6b7c

import { Box, Typography, Paper, Grid } from '@mui/material';

export function Dashboard() {
  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        Dashboard
      </Typography>
      <Grid container spacing={3}>
        <Grid item xs={12} md={6} lg={3}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6" color="text.secondary">
              Total Books
            </Typography>
            <Typography variant="h3">0</Typography>
          </Paper>
        </Grid>
        <Grid item xs={12} md={6} lg={3}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6" color="text.secondary">
              Total Authors
            </Typography>
            <Typography variant="h3">0</Typography>
          </Paper>
        </Grid>
        <Grid item xs={12} md={6} lg={3}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6" color="text.secondary">
              Total Series
            </Typography>
            <Typography variant="h3">0</Typography>
          </Paper>
        </Grid>
        <Grid item xs={12} md={6} lg={3}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6" color="text.secondary">
              Storage Used
            </Typography>
            <Typography variant="h3">0 GB</Typography>
          </Paper>
        </Grid>
      </Grid>
    </Box>
  );
}
