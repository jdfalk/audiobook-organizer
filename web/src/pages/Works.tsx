// file: web/src/pages/Works.tsx
// version: 1.1.0
// guid: 4b5c6d7e-8f9a-0b1c-2d3e-4f5a6b7c8d9e

import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import * as api from '../services/api';

export function Works() {
  const [works, setWorks] = useState<api.Work[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadWorks = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.getWorks();
      setWorks(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load works');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadWorks();
  }, [loadWorks]);

  if (loading) {
    return (
      <Box sx={{ py: 6, display: 'flex', justifyContent: 'center' }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Stack spacing={2}>
        <Typography variant="h4">Works</Typography>
        <Alert
          severity="error"
          action={
            <Button color="inherit" size="small" onClick={() => void loadWorks()}>
              Retry
            </Button>
          }
        >
          {error}
        </Alert>
      </Stack>
    );
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      <Typography variant="h4" gutterBottom>
        Works
      </Typography>
      <Typography variant="body2" color="text.secondary">
        Logical title-level groupings across editions and narrations.
      </Typography>

      {works.length === 0 ? (
        <Alert severity="info">
          No works found yet. Works are created during scans and metadata imports.
        </Alert>
      ) : (
        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Title</TableCell>
                <TableCell>Work ID</TableCell>
                <TableCell>Author ID</TableCell>
                <TableCell>Series ID</TableCell>
                <TableCell>Alternate Titles</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {works.map((work) => (
                <TableRow key={work.id} hover>
                  <TableCell>{work.title || 'Untitled'}</TableCell>
                  <TableCell>{work.id}</TableCell>
                  <TableCell>{work.author_id ?? '—'}</TableCell>
                  <TableCell>{work.series_id ?? '—'}</TableCell>
                  <TableCell>{work.alt_titles?.length ?? 0}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}
