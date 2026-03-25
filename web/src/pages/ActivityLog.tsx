// file: web/src/pages/ActivityLog.tsx
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

import { useEffect, useState } from 'react';
import {
  Box,
  Chip,
  CircularProgress,
  MenuItem,
  Pagination,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { fetchActivity } from '../services/activityApi';
import type { ActivityEntry } from '../services/activityApi';

const PAGE_SIZE = 50;

const EVENT_TYPES = [
  'all',
  'book_added',
  'book_updated',
  'book_deleted',
  'book_restored',
  'tag_written',
  'metadata_applied',
  'scan_started',
  'scan_completed',
  'organize_started',
  'organize_completed',
  'import_started',
  'import_completed',
  'maintenance_run',
  'config_changed',
  'user_action',
];

function tierChip(tier: string) {
  const colorMap: Record<string, 'primary' | 'secondary' | 'default'> = {
    audit: 'primary',
    change: 'secondary',
    debug: 'default',
  };
  return (
    <Chip
      size="small"
      label={tier}
      color={colorMap[tier] ?? 'default'}
    />
  );
}

function levelChip(level: string) {
  const colorMap: Record<string, 'error' | 'warning' | 'info' | 'success' | 'default'> = {
    error: 'error',
    warn: 'warning',
    warning: 'warning',
    info: 'info',
    debug: 'default',
  };
  return (
    <Chip
      size="small"
      label={level}
      color={colorMap[level] ?? 'default'}
      variant="outlined"
    />
  );
}

const formatTimestamp = (ts: string): string => {
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleString();
};

export default function ActivityLog() {
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [tierFilter, setTierFilter] = useState('all');
  const [typeFilter, setTypeFilter] = useState('all');
  const [operationId, setOperationId] = useState('');

  const load = async (currentPage: number) => {
    setLoading(true);
    try {
      const result = await fetchActivity({
        limit: PAGE_SIZE,
        offset: (currentPage - 1) * PAGE_SIZE,
        tier: tierFilter !== 'all' ? tierFilter : undefined,
        type: typeFilter !== 'all' ? typeFilter : undefined,
        operation_id: operationId.trim() || undefined,
      });
      setEntries(result.entries || []);
      setTotal(result.total || 0);
    } catch (err) {
      console.error('Failed to load activity log', err);
      setEntries([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setPage(1);
    load(1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tierFilter, typeFilter, operationId]);

  useEffect(() => {
    load(page);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Typography variant="h4" gutterBottom>
        Activity Log
      </Typography>

      {/* Filters */}
      <Paper sx={{ p: 2, mb: 2 }}>
        <Stack direction="row" spacing={2} flexWrap="wrap">
          <TextField
            select
            size="small"
            label="Tier"
            value={tierFilter}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setTierFilter(e.target.value)}
            sx={{ minWidth: 140 }}
          >
            <MenuItem value="all">All tiers</MenuItem>
            <MenuItem value="audit">Audit</MenuItem>
            <MenuItem value="change">Change</MenuItem>
            <MenuItem value="debug">Debug</MenuItem>
          </TextField>

          <TextField
            select
            size="small"
            label="Type"
            value={typeFilter}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setTypeFilter(e.target.value)}
            sx={{ minWidth: 200 }}
          >
            {EVENT_TYPES.map((t) => (
              <MenuItem key={t} value={t}>
                {t === 'all' ? 'All types' : t.replace(/_/g, ' ')}
              </MenuItem>
            ))}
          </TextField>

          <TextField
            size="small"
            label="Operation ID"
            value={operationId}
            onChange={(e) => setOperationId(e.target.value)}
            sx={{ minWidth: 240 }}
            placeholder="Filter by operation ID"
          />
        </Stack>
      </Paper>

      {/* Table */}
      <Paper>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 6 }}>
            <CircularProgress />
          </Box>
        ) : entries.length === 0 ? (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ py: 4, textAlign: 'center' }}
          >
            No activity entries found.
          </Typography>
        ) : (
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Time</TableCell>
                <TableCell>Tier</TableCell>
                <TableCell>Type</TableCell>
                <TableCell>Level</TableCell>
                <TableCell>Summary</TableCell>
                <TableCell>Source</TableCell>
                <TableCell>Tags</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {entries.map((entry) => (
                <TableRow key={entry.id} hover>
                  <TableCell sx={{ whiteSpace: 'nowrap', color: 'text.secondary', fontSize: '0.75rem' }}>
                    {formatTimestamp(entry.timestamp)}
                  </TableCell>
                  <TableCell>{tierChip(entry.tier)}</TableCell>
                  <TableCell>
                    <Typography variant="caption">
                      {entry.type.replace(/_/g, ' ')}
                    </Typography>
                  </TableCell>
                  <TableCell>{levelChip(entry.level)}</TableCell>
                  <TableCell sx={{ maxWidth: 300 }}>
                    <Typography variant="body2" noWrap title={entry.summary}>
                      {entry.summary}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="caption" color="text.secondary">
                      {entry.source}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    {entry.tags && entry.tags.length > 0 ? (
                      <Stack direction="row" spacing={0.5} flexWrap="wrap">
                        {entry.tags.map((tag) => (
                          <Chip key={tag} size="small" label={tag} />
                        ))}
                      </Stack>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}

        {totalPages > 1 && (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
            <Pagination
              count={totalPages}
              page={page}
              onChange={(_, p) => setPage(p)}
              color="primary"
            />
          </Box>
        )}
      </Paper>
    </Box>
  );
}
