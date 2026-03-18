// file: web/src/components/ChangeLog.tsx
// version: 1.0.0
// guid: 00f575de-ecea-45b7-9aa5-d6dbbc3f21f6

import { useCallback, useEffect, useState } from 'react';
import {
  Box,
  CircularProgress,
  Stack,
  Typography,
} from '@mui/material';
import type { ChangeLogEntry } from '../services/api';
import * as api from '../services/api';

interface ChangeLogProps {
  bookId: string;
  refreshKey?: number;
}

const TYPE_ICONS: Record<string, string> = {
  tag_write: '\uD83C\uDFF7\uFE0F',     // label/tag
  rename: '\uD83D\uDCC1',              // folder
  metadata_apply: '\uD83D\uDCE5',      // inbox tray
  import: '\uD83D\uDCE6',              // package
  transcode: '\uD83D\uDD04',           // arrows cycle
};

const TYPE_LABELS: Record<string, string> = {
  tag_write: 'Tag Write',
  rename: 'Rename',
  metadata_apply: 'Metadata Apply',
  import: 'Import',
  transcode: 'Transcode',
};

const formatTimestamp = (ts: string): string => {
  const date = new Date(ts);
  if (isNaN(date.getTime())) return ts;
  return date.toLocaleString();
};

export const ChangeLog = ({ bookId, refreshKey }: ChangeLogProps) => {
  const [entries, setEntries] = useState<ChangeLogEntry[]>([]);
  const [loading, setLoading] = useState(true);

  const loadChangelog = useCallback(async () => {
    setLoading(true);
    try {
      const result = await api.getBookChangelog(bookId);
      setEntries(result.entries || []);
    } catch {
      setEntries([]);
    } finally {
      setLoading(false);
    }
  }, [bookId]);

  useEffect(() => {
    loadChangelog();
  }, [loadChangelog, refreshKey]);

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  if (entries.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ py: 2 }} data-testid="changelog-empty">
        No changes recorded yet.
      </Typography>
    );
  }

  return (
    <Stack spacing={0} data-testid="changelog-timeline">
      {entries.map((entry, idx) => (
        <Box
          key={`${entry.timestamp}-${idx}`}
          sx={{
            display: 'flex',
            alignItems: 'flex-start',
            gap: 2,
            py: 1.5,
            px: 1,
            borderBottom: idx < entries.length - 1 ? '1px solid' : 'none',
            borderColor: 'divider',
            '&:hover': { bgcolor: 'action.hover' },
          }}
        >
          {/* Timestamp */}
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ minWidth: 140, flexShrink: 0, pt: 0.25 }}
          >
            {formatTimestamp(entry.timestamp)}
          </Typography>

          {/* Icon + summary */}
          <Stack direction="row" spacing={1} alignItems="center" sx={{ flex: 1, minWidth: 0 }}>
            <Typography variant="body2" sx={{ flexShrink: 0 }}>
              {TYPE_ICONS[entry.type] || '\u2022'}
            </Typography>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body2" sx={{ fontWeight: 500 }}>
                {TYPE_LABELS[entry.type] || entry.type}
              </Typography>
              <Typography variant="body2" color="text.secondary" noWrap>
                {entry.summary}
              </Typography>
            </Box>
          </Stack>

          {/* Compare snapshot link for tag_write entries */}
          {entry.type === 'tag_write' && (
            <Typography
              variant="caption"
              color="primary"
              sx={{
                cursor: 'pointer',
                flexShrink: 0,
                pt: 0.25,
                '&:hover': { textDecoration: 'underline' },
              }}
            >
              Compare snapshot &rarr;
            </Typography>
          )}
        </Box>
      ))}
    </Stack>
  );
};
