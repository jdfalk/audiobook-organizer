// file: web/src/pages/AuthorDedup.tsx
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

import { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Typography,
  Paper,
  Button,
  Alert,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  Tooltip,
  Card,
  CardContent,
  CardActions,
  Stack,
} from '@mui/material';
import MergeIcon from '@mui/icons-material/MergeType';
import RefreshIcon from '@mui/icons-material/Refresh';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import MenuBookIcon from '@mui/icons-material/MenuBook';
import * as api from '../services/api';
import type { AuthorDedupGroup } from '../services/api';

export function AuthorDedup() {
  const [groups, setGroups] = useState<AuthorDedupGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [merging, setMerging] = useState<number | null>(null);
  const [mergeSuccess, setMergeSuccess] = useState<string | null>(null);

  const fetchDuplicates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.getAuthorDuplicates();
      setGroups(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch duplicates');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDuplicates();
  }, [fetchDuplicates]);

  const handleMerge = async (group: AuthorDedupGroup) => {
    const keepId = group.canonical.id;
    const mergeIds = group.variants.map((v) => v.id);
    setMerging(keepId);
    setMergeSuccess(null);
    try {
      const result = await api.mergeAuthors(keepId, mergeIds);
      if (result.errors && result.errors.length > 0) {
        setError(result.errors.join('; '));
      } else {
        setMergeSuccess(
          `Merged ${result.merged} author(s) into "${group.canonical.name}"`
        );
        // Remove merged group from list
        setGroups((prev) => prev.filter((g) => g.canonical.id !== keepId));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Merge failed');
    } finally {
      setMerging(null);
    }
  };

  return (
    <Box sx={{ p: 3 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 3 }}>
        <Typography variant="h5" sx={{ flexGrow: 1 }}>
          Author Deduplication
        </Typography>
        <Tooltip title="Refresh">
          <IconButton onClick={fetchDuplicates} disabled={loading}>
            <RefreshIcon />
          </IconButton>
        </Tooltip>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {mergeSuccess && (
        <Alert
          severity="success"
          sx={{ mb: 2 }}
          icon={<CheckCircleIcon />}
          onClose={() => setMergeSuccess(null)}
        >
          {mergeSuccess}
        </Alert>
      )}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
          <CircularProgress />
        </Box>
      ) : groups.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <CheckCircleIcon sx={{ fontSize: 48, color: 'success.main', mb: 1 }} />
          <Typography variant="h6">No duplicate authors found</Typography>
          <Typography variant="body2" color="text.secondary">
            All author names in your library are unique.
          </Typography>
        </Paper>
      ) : (
        <Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Found {groups.length} group(s) of potentially duplicate authors.
            Review each group and merge if appropriate.
          </Typography>

          <Stack spacing={2}>
            {groups.map((group) => (
              <Card key={group.canonical.id} variant="outlined">
                <CardContent>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                    <Typography variant="subtitle1" fontWeight="bold">
                      {group.canonical.name}
                    </Typography>
                    <Chip
                      icon={<MenuBookIcon />}
                      label={`${group.book_count} book(s)`}
                      size="small"
                      variant="outlined"
                    />
                  </Box>

                  <Divider sx={{ my: 1 }} />

                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                    Variants that will be merged:
                  </Typography>

                  <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                    {group.variants.map((variant) => (
                      <Chip
                        key={variant.id}
                        label={variant.name}
                        color="warning"
                        variant="outlined"
                        size="small"
                      />
                    ))}
                  </Box>
                </CardContent>

                <CardActions>
                  <Button
                    startIcon={<MergeIcon />}
                    variant="contained"
                    size="small"
                    onClick={() => handleMerge(group)}
                    disabled={merging !== null}
                  >
                    {merging === group.canonical.id
                      ? 'Merging...'
                      : `Merge into "${group.canonical.name}"`}
                  </Button>
                </CardActions>
              </Card>
            ))}
          </Stack>
        </Box>
      )}
    </Box>
  );
}
