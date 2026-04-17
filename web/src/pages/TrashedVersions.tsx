// file: web/src/pages/TrashedVersions.tsx
// version: 1.0.0
// guid: 6f4a5b3c-7d8e-4a70-b8c5-3d7e0f1b9a99

import { useCallback, useEffect, useState } from 'react';
import {
  Box,
  Button,
  Chip,
  Paper,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tabs,
  Typography,
} from '@mui/material';
import {
  type BookVersion,
  restoreVersion,
  purgeVersion,
  hardDeleteVersion,
} from '../services/versionApi';

const API_BASE = '/api/v1';

export default function TrashedVersions() {
  const [tab, setTab] = useState<'trash' | 'purged'>('trash');
  const [versions, setVersions] = useState<BookVersion[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const endpoint = tab === 'trash' ? '/audiobooks/trashed-versions' : '/audiobooks/purged-versions';
      const resp = await fetch(`${API_BASE}${endpoint}`);
      if (resp.ok) {
        const data = await resp.json();
        setVersions(data.versions || []);
      }
    } catch {
      setVersions([]);
    } finally {
      setLoading(false);
    }
  }, [tab]);

  useEffect(() => { load(); }, [load]);

  const handleRestore = useCallback(async (v: BookVersion) => {
    await restoreVersion(v.book_id, v.id);
    load();
  }, [load]);

  const handlePurge = useCallback(async (v: BookVersion) => {
    if (!confirm('Permanently delete files for this version? This cannot be undone.')) return;
    await purgeVersion(v.book_id, v.id);
    load();
  }, [load]);

  const handleHardDelete = useCallback(async (v: BookVersion) => {
    if (!confirm('Remove all traces of this version? Fingerprint data will be lost.')) return;
    await hardDeleteVersion(v.id);
    load();
  }, [load]);

  return (
    <Box sx={{ p: 3 }}>
      <Typography variant="h4" sx={{ mb: 2 }}>Version Management</Typography>

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2 }}>
        <Tab label="Trash" value="trash" />
        <Tab label="Purged" value="purged" />
      </Tabs>

      {loading ? (
        <Typography color="text.secondary">Loading...</Typography>
      ) : versions.length === 0 ? (
        <Typography color="text.secondary">
          {tab === 'trash' ? 'No trashed versions.' : 'No purged versions.'}
        </Typography>
      ) : (
        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Book ID</TableCell>
                <TableCell>Format</TableCell>
                <TableCell>Source</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Date</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {versions.map((v) => (
                <TableRow key={v.id}>
                  <TableCell>
                    <Typography variant="caption" fontFamily="monospace">
                      {v.book_id.slice(0, 12)}...
                    </Typography>
                  </TableCell>
                  <TableCell>{v.format?.toUpperCase()}</TableCell>
                  <TableCell>{v.source}</TableCell>
                  <TableCell>
                    <Chip label={v.status} size="small" color={v.status === 'trash' ? 'warning' : 'error'} />
                  </TableCell>
                  <TableCell>
                    {new Date(v.purged_date || v.created_at).toLocaleDateString()}
                  </TableCell>
                  <TableCell align="right">
                    {v.status === 'trash' && (
                      <>
                        <Button size="small" onClick={() => handleRestore(v)}>Restore</Button>
                        <Button size="small" color="error" onClick={() => handlePurge(v)}>Purge Now</Button>
                      </>
                    )}
                    {(v.status === 'inactive_purged' || v.status === 'blocked_for_redownload') && (
                      <Button size="small" color="error" onClick={() => handleHardDelete(v)}>
                        Hard Delete
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}
