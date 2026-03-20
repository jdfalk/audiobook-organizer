// file: web/src/components/ChangeLog.tsx
// version: 1.2.0
// guid: 00f575de-ecea-45b7-9aa5-d6dbbc3f21f6

import { useCallback, useEffect, useState } from 'react';
import {
  Box,
  Button,
  CircularProgress,
  Stack,
  Typography,
} from '@mui/material';
import RestoreIcon from '@mui/icons-material/Restore.js';
import type { ChangeLogEntry } from '../services/api';
import * as api from '../services/api';

interface ChangeLogProps {
  bookId: string;
  refreshKey?: number;
  onRevert?: () => void; // called after successful revert so parent can refresh
  onCompareSnapshot?: (timestamp: string) => void; // called when user clicks "Compare →" on a tag_write entry
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

export const ChangeLog = ({ bookId, refreshKey, onRevert, onCompareSnapshot }: ChangeLogProps) => {
  const [reverting, setReverting] = useState<string | null>(null);

  const handleRevert = async (timestamp: string) => {
    setReverting(timestamp);
    try {
      await fetch(`/api/v1/audiobooks/${bookId}/revert-metadata`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ timestamp }),
      });
      // Also trigger write-back to sync tags to file
      await fetch(`/api/v1/audiobooks/${bookId}/write-back`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ rename: true }),
      });
      loadChangelog();
      onRevert?.();
    } catch {
      // silently fail
    } finally {
      setReverting(null);
    }
  };
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
            cursor: (entry.type === 'metadata_apply' || entry.type === 'tag_write') ? 'pointer' : undefined,
            '&:hover': { bgcolor: 'action.hover' },
          }}
          onClick={() => {
            if (entry.type === 'metadata_apply' || entry.type === 'tag_write') {
              onCompareSnapshot?.(entry.timestamp);
            }
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

          {/* Actions */}
          <Stack direction="row" spacing={1} sx={{ flexShrink: 0, alignItems: 'center' }}>
            {entry.type === 'tag_write' && (
              <Typography
                variant="caption"
                color="primary"
                sx={{
                  cursor: 'pointer',
                  '&:hover': { textDecoration: 'underline' },
                }}
                onClick={() => onCompareSnapshot?.(entry.timestamp)}
              >
                Compare snapshot &rarr;
              </Typography>
            )}
            {idx > 0 && (entry.type === 'metadata_apply' || entry.type === 'tag_write' || entry.type === 'rename') && (
              <Button
                size="small"
                variant="outlined"
                color="warning"
                startIcon={<RestoreIcon />}
                disabled={reverting === entry.timestamp}
                onClick={(e) => { e.stopPropagation(); handleRevert(entry.timestamp); }}
                sx={{ fontSize: '0.7rem', py: 0.25, px: 1 }}
              >
                {reverting === entry.timestamp ? 'Reverting...' : 'Revert'}
              </Button>
            )}
          </Stack>
        </Box>
      ))}
    </Stack>
  );
};
