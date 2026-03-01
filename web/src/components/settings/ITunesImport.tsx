// file: web/src/components/settings/ITunesImport.tsx
// version: 1.10.0
// guid: 4eb9b74d-7192-497b-849a-092833ae63a4

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Alert,
  AlertTitle,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Checkbox,
  Chip,
  Divider,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  FormLabel,
  InputAdornment,
  LinearProgress,
  List,
  ListItem,
  ListItemText,
  Paper,
  Radio,
  RadioGroup,
  Stack,
  Tab,
  Tabs,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add.js';
import DeleteIcon from '@mui/icons-material/Delete.js';
import FolderOpenIcon from '@mui/icons-material/FolderOpen.js';
import CloudUploadIcon from '@mui/icons-material/CloudUpload.js';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload.js';
import CheckCircleIcon from '@mui/icons-material/CheckCircle.js';
import SearchIcon from '@mui/icons-material/Search.js';
import SyncIcon from '@mui/icons-material/Sync.js';
import IconButton from '@mui/material/IconButton';
import { ITunesConflictDialog, type ConflictItem } from './ITunesConflictDialog';
import {
  ApiError,
  cancelOperation,
  getActiveOperations,
  getITunesBooks,
  getITunesImportStatus,
  getITunesLibraryStatus,
  importITunesLibrary,
  previewITunesWriteBack,
  startITunesSync,
  type ITunesBookMapping,
  type ITunesImportRequest,
  type ITunesImportStatus,
  type ITunesValidateResponse,
  type ITunesWriteBackResponse,
  type PathMapping,
  validateITunesLibrary,
  writeBackITunesLibrary,
} from '../../services/api';
import { useOperationsStore } from '../../stores/useOperationsStore';
import { useToast } from '../toast/ToastProvider';

interface ITunesImportSettings {
  libraryPath: string;
  importMode: 'organized' | 'import' | 'organize';
  preserveLocation: boolean;
  importPlaylists: boolean;
  skipDuplicates: boolean;
  pathMappings: PathMapping[];
}

const defaultSettings: ITunesImportSettings = {
  libraryPath: '',
  importMode: 'import',
  preserveLocation: false,
  importPlaylists: true,
  skipDuplicates: true,
  pathMappings: [],
};

/**
 * ITunesImport provides a guided workflow to validate and import iTunes
 * Library.xml metadata into the audiobook organizer.
 */
export function ITunesImport() {
  const { toast } = useToast();
  const [settings, setSettings] = useState<ITunesImportSettings>(() => {
    try {
      const saved = localStorage.getItem('itunes_import_settings');
      if (saved) {
        return { ...defaultSettings, ...JSON.parse(saved) };
      }
    } catch {
      // ignore
    }
    return defaultSettings;
  });
  // Persist settings to localStorage
  useEffect(() => {
    try {
      localStorage.setItem('itunes_import_settings', JSON.stringify(settings));
    } catch {
      // ignore
    }
  }, [settings]);

  const [validationResult, setValidationResult] =
    useState<ITunesValidateResponse | null>(null);
  const [validating, setValidating] = useState(false);
  const [importing, setImporting] = useState(false);
  const [importStatus, setImportStatus] =
    useState<ITunesImportStatus | null>(null);
  const [showMissingFiles, setShowMissingFiles] = useState(false);
  const [writeBackOpen, setWriteBackOpen] = useState(false);
  const [writeBackIds, setWriteBackIds] = useState('');
  const [writeBackLoading, setWriteBackLoading] = useState(false);
  const [writeBackNotice, setWriteBackNotice] = useState<{
    severity: 'error' | 'warning' | 'success';
    message: string;
  } | null>(null);
  const [writeBackResult, setWriteBackResult] =
    useState<ITunesWriteBackResponse | null>(null);
  const [writeBackBackup, setWriteBackBackup] = useState(true);
  const [writeBackLibraryPath, setWriteBackLibraryPath] = useState('');
  const [writeBackMode, setWriteBackMode] = useState(0); // 0=manual, 1=sync all, 2=browse
  const [previewItems, setPreviewItems] = useState<ITunesBookMapping[]>([]);
  const [browseItems, setBrowseItems] = useState<ITunesBookMapping[]>([]);
  const [browseTotal, setBrowseTotal] = useState(0);
  const [browseSearch, setBrowseSearch] = useState('');
  const [browsePage, setBrowsePage] = useState(0);
  const [browseRowsPerPage, setBrowseRowsPerPage] = useState(25);
  const [browseSelected, setBrowseSelected] = useState<Set<string>>(new Set());
  const [browseLoading, setBrowseLoading] = useState(false);
  const [syncAllCount, setSyncAllCount] = useState<number | null>(null);
  const [confirmWriteBackOpen, setConfirmWriteBackOpen] = useState(false);
  const [pendingWriteBackIds, setPendingWriteBackIds] = useState<string[]>([]);
  const searchDebounceRef = useRef<number | null>(null);
  const [showConflictDialog, setShowConflictDialog] = useState(false);
  const [pendingConflicts] = useState<ConflictItem[]>([]);
  const [syncingWithConflicts, setSyncingWithConflicts] = useState(false);
  const [libraryChanged, setLibraryChanged] = useState(false);
  const [overwriteConfirmOpen, setOverwriteConfirmOpen] = useState(false);
  const [forceImportConfirmOpen, setForceImportConfirmOpen] = useState(false);
  const [forceSyncToITunesConfirmOpen, setForceSyncToITunesConfirmOpen] = useState(false);
  const pollTimeoutRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (pollTimeoutRef.current) {
        window.clearTimeout(pollTimeoutRef.current);
      }
    };
  }, []);

  // Poll library status when a library path is configured
  useEffect(() => {
    const itunesPath = settings.libraryPath;
    if (!itunesPath) return;

    const checkStatus = async () => {
      try {
        const status = await getITunesLibraryStatus(itunesPath);
        setLibraryChanged(status.changed_since_import === true);
      } catch {
        // Silently ignore — library status is non-critical
      }
    };

    checkStatus();
    const interval = setInterval(checkStatus, 30000);
    return () => clearInterval(interval);
  }, [settings.libraryPath]);

  // Detect an already-running iTunes import on mount
  useEffect(() => {
    let cancelled = false;
    const detectRunningImport = async () => {
      try {
        const ops = await getActiveOperations();
        if (cancelled) return;
        const running = ops.find(
          (op) =>
            op.type === 'itunes_import' &&
            !['completed', 'failed', 'canceled'].includes(op.status)
        );
        if (running) {
          setImporting(true);
          await pollImportStatus(running.id);
        }
      } catch {
        // Ignore — API may not be ready
      }
    };
    detectRunningImport();
    return () => { cancelled = true; };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleBrowseFile = () => {
    const path = window.prompt('Enter path to iTunes Library.xml:');
    if (path) {
      setSettings((prev) => ({ ...prev, libraryPath: path }));
    }
  };

  const handleValidate = async () => {
    setValidating(true);

    setValidationResult(null);

    try {
      const activeMappings = settings.pathMappings.filter((m) => m.from && m.to);
      const result = await validateITunesLibrary({
        library_path: settings.libraryPath,
        path_mappings: activeMappings.length > 0 ? activeMappings : undefined,
      });
      setValidationResult(result);
      // Auto-populate path mappings from detected prefixes
      if (result.path_prefixes?.length && settings.pathMappings.length === 0) {
        setSettings((prev) => ({
          ...prev,
          pathMappings: result.path_prefixes!.map((p) => ({ from: p, to: '' })),
        }));
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Validation failed';
      toast(message, 'error');
    } finally {
      setValidating(false);
    }
  };

  const handleImport = async () => {
    setImporting(true);

    setImportStatus(null);

    try {
      const request: ITunesImportRequest = {
        library_path: settings.libraryPath,
        import_mode: settings.importMode,
        preserve_location: settings.preserveLocation,
        import_playlists: settings.importPlaylists,
        skip_duplicates: settings.skipDuplicates,
        path_mappings: settings.pathMappings.filter((m) => m.from && m.to),
      };

      const result = await importITunesLibrary(request);
      useOperationsStore.getState().startPolling(result.operation_id, 'itunes_import');
      await pollImportStatus(result.operation_id);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Import failed';
      toast(message, 'error');
      setImporting(false);
    }
  };

  function parseWriteBackIds(raw: string): string[] {
    return raw
      .split(/[\n,]+/)
      .map((value) => value.trim())
      .filter((value) => value.length > 0);
  }

  const handleOpenWriteBack = () => {
    setWriteBackOpen(true);
    setWriteBackIds('');
    setWriteBackNotice(null);
    setWriteBackResult(null);
    setWriteBackBackup(true);
    setWriteBackLibraryPath(settings.libraryPath);
    setWriteBackMode(0);
    setPreviewItems([]);
    setBrowseItems([]);
    setBrowseSelected(new Set());
    setSyncAllCount(null);
  };

  const loadBrowseBooks = useCallback(async (search: string, page: number, rowsPerPage: number) => {
    setBrowseLoading(true);
    try {
      const result = await getITunesBooks(search || undefined, rowsPerPage, page * rowsPerPage);
      setBrowseItems(result.items || []);
      setBrowseTotal(result.count);
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to load books', 'error');
    } finally {
      setBrowseLoading(false);
    }
  }, [toast]);

  // Auto-load browse data when switching to browse tab
  useEffect(() => {
    if (writeBackOpen && writeBackMode === 2 && browseItems.length === 0) {
      loadBrowseBooks('', 0, browseRowsPerPage);
    }
  }, [writeBackOpen, writeBackMode]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleBrowseSearchChange = (value: string) => {
    setBrowseSearch(value);
    setBrowsePage(0);
    if (searchDebounceRef.current) {
      window.clearTimeout(searchDebounceRef.current);
    }
    searchDebounceRef.current = window.setTimeout(() => {
      loadBrowseBooks(value, 0, browseRowsPerPage);
    }, 300);
  };

  const handlePreviewAll = async () => {
    if (!writeBackLibraryPath.trim()) {
      setWriteBackNotice({ severity: 'error', message: 'Library path is required.' });
      return;
    }
    setWriteBackLoading(true);
    setWriteBackNotice(null);
    try {
      const result = await previewITunesWriteBack(writeBackLibraryPath);
      const differing = (result.items || []).filter((item) => item.path_differs);
      setPreviewItems(result.items || []);
      setSyncAllCount(differing.length);
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Preview failed', 'error');
    } finally {
      setWriteBackLoading(false);
    }
  };

  const handlePreviewManual = async () => {
    if (!writeBackLibraryPath.trim()) {
      setWriteBackNotice({ severity: 'error', message: 'Library path is required.' });
      return;
    }
    const ids = parseWriteBackIds(writeBackIds);
    if (ids.length === 0) {
      setWriteBackNotice({ severity: 'error', message: 'Enter one or more IDs to preview.' });
      return;
    }
    setWriteBackLoading(true);
    setWriteBackNotice(null);
    try {
      const result = await previewITunesWriteBack(writeBackLibraryPath, ids);
      setPreviewItems(result.items || []);
      if (result.items.length === 0) {
        setWriteBackNotice({ severity: 'warning', message: 'No books found with iTunes persistent IDs for those IDs.' });
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Preview failed', 'error');
    } finally {
      setWriteBackLoading(false);
    }
  };

  const handleConfirmAndWriteBack = (bookIds: string[]) => {
    setPendingWriteBackIds(bookIds);
    setConfirmWriteBackOpen(true);
  };

  const executeWriteBack = async (bookIds: string[], forceOverwrite = false) => {
    if (!writeBackLibraryPath.trim()) {
      setWriteBackNotice({ severity: 'error', message: 'Library path is required.' });
      return;
    }
    setWriteBackLoading(true);
    setWriteBackNotice(null);
    setWriteBackResult(null);
    try {
      const result = await writeBackITunesLibrary({
        library_path: writeBackLibraryPath,
        audiobook_ids: bookIds,
        create_backup: writeBackBackup,
        force_overwrite: forceOverwrite,
      });
      setWriteBackResult(result);
      setWriteBackNotice({
        severity: 'success',
        message: result.message || `Updated ${result.updated_count} entries.`,
      });
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setOverwriteConfirmOpen(true);
      } else {
        const message = err instanceof Error ? err.message : 'Write-back failed.';
        setWriteBackNotice({ severity: 'error', message });
      }
    } finally {
      setWriteBackLoading(false);
    }
  };

  const pollImportStatus = async (operationId: string) => {
    const poll = async () => {
      try {
        const status = await getITunesImportStatus(operationId);
        setImportStatus(status);

        if (status.status === 'completed' || status.status === 'failed') {
          setImporting(false);
          return;
        }

        pollTimeoutRef.current = window.setTimeout(poll, 2000);
      } catch (err) {
        const message =
          err instanceof Error ? err.message : 'Failed to get import status';
        toast(message, 'error');
        setImporting(false);
      }
    };

    await poll();
  };

  const handleConflictResolve = async (resolutions: Record<string, 'itunes' | 'organizer'>) => {
    setSyncingWithConflicts(true);
    try {
      // Send resolutions to backend for sync
      const response = await fetch(`${import.meta.env.VITE_API_BASE}/itunes/resolve-conflicts`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ resolutions }),
      });

      if (!response.ok) {
        throw new Error('Failed to apply conflict resolutions');
      }

      setShowConflictDialog(false);
  
      // Refresh sync status
      if (importStatus?.operation_id) {
        await pollImportStatus(importStatus.operation_id);
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Conflict resolution failed', 'error');
    } finally {
      setSyncingWithConflicts(false);
    }
  };

  return (
    <Card>
      <CardHeader title="iTunes Library Import" />
      <CardContent>
        <Typography variant="body2" color="text.secondary" gutterBottom>
          Import your iTunes Library.xml with play counts, ratings, and
          bookmarks preserved.
        </Typography>

        {libraryChanged && (
          <Alert severity="warning" sx={{ mt: 2 }}>
            iTunes library has been modified since last import. Consider re-importing to pick up changes.
          </Alert>
        )}

        <Box sx={{ mt: 3 }}>
          <TextField
            label="iTunes Library Path"
            value={settings.libraryPath}
            onChange={(event) =>
              setSettings((prev) => ({
                ...prev,
                libraryPath: event.target.value,
              }))
            }
            fullWidth
            placeholder="/Users/username/Music/iTunes/iTunes Music Library.xml"
            helperText="Path to iTunes Library.xml or Music Library.xml"
            InputProps={{
              endAdornment: (
                <Button startIcon={<FolderOpenIcon />} onClick={handleBrowseFile}>
                  Browse
                </Button>
              ),
            }}
          />
        </Box>

        <Box sx={{ mt: 3 }}>
          <Typography variant="subtitle2" gutterBottom>
            Path Mapping (for cross-platform imports)
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Map iTunes file prefixes to local paths. Validate to auto-detect, or add manually.
          </Typography>
          {settings.pathMappings.map((mapping, idx) => (
            <Box key={idx} sx={{ display: 'flex', gap: 1, mb: 1.5, alignItems: 'flex-start' }}>
              <Box sx={{ flex: 1 }}>
                <TextField
                  fullWidth
                  size="small"
                  label="From prefix"
                  placeholder="file://localhost/W:/itunes/iTunes%20Media"
                  value={mapping.from}
                  onChange={(e) => {
                    const updated = [...settings.pathMappings];
                    updated[idx] = { ...updated[idx], from: e.target.value };
                    setSettings((prev) => ({ ...prev, pathMappings: updated }));
                  }}
                />
              </Box>
              <Box sx={{ flex: 1 }}>
                <TextField
                  fullWidth
                  size="small"
                  label="To local path"
                  placeholder="/local/path/to/media"
                  value={mapping.to}
                  onChange={(e) => {
                    const updated = [...settings.pathMappings];
                    updated[idx] = { ...updated[idx], to: e.target.value };
                    setSettings((prev) => ({ ...prev, pathMappings: updated }));
                  }}
                />
              </Box>
              <IconButton
                size="small"
                color="error"
                onClick={() => {
                  setSettings((prev) => ({
                    ...prev,
                    pathMappings: prev.pathMappings.filter((_, i) => i !== idx),
                  }));
                }}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Box>
          ))}
          <Button
            size="small"
            startIcon={<AddIcon />}
            onClick={() =>
              setSettings((prev) => ({
                ...prev,
                pathMappings: [...prev.pathMappings, { from: '', to: '' }],
              }))
            }
          >
            Add Mapping
          </Button>
        </Box>

        <Box sx={{ mt: 3 }}>
          <FormControl component="fieldset">
            <FormLabel component="legend">Import Mode</FormLabel>
            <RadioGroup
              value={settings.importMode}
              onChange={(event) =>
                setSettings((prev) => ({
                  ...prev,
                  importMode: event.target.value as ITunesImportSettings['importMode'],
                }))
              }
            >
              <Tooltip title="Import all metadata (titles, authors, play counts, ratings, bookmarks) but mark files as already in their final location. Use this if your iTunes library folder structure is how you want it." placement="right" arrow>
                <FormControlLabel
                  value="organized"
                  control={<Radio />}
                  label="Files already organized"
                />
              </Tooltip>
              <Tooltip title="Import all metadata into the database but leave files where they are. You can organize them later from the Library page. Good for previewing what will be imported before moving anything." placement="right" arrow>
                <FormControlLabel
                  value="import"
                  control={<Radio />}
                  label="Import metadata only"
                />
              </Tooltip>
              <Tooltip title="Import all metadata AND move/rename files into the organized folder structure (Author/Series/Title). Files are copied to the root directory. Skips files that are already organized." placement="right" arrow>
                <FormControlLabel
                  value="organize"
                  control={<Radio />}
                  label="Import and organize now"
                />
              </Tooltip>
            </RadioGroup>
          </FormControl>

          <Box sx={{ mt: 2 }}>
            <Tooltip title="Don't move files during organize — only update the database with their current locations." placement="right" arrow>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={settings.preserveLocation}
                    onChange={(event) =>
                      setSettings((prev) => ({
                        ...prev,
                        preserveLocation: event.target.checked,
                      }))
                    }
                  />
                }
                label="Preserve original file locations"
              />
            </Tooltip>
          </Box>

          <Box>
            <Tooltip title="Convert iTunes playlist memberships into tags on each audiobook." placement="right" arrow>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={settings.importPlaylists}
                    onChange={(event) =>
                      setSettings((prev) => ({
                        ...prev,
                        importPlaylists: event.target.checked,
                      }))
                    }
                  />
                }
                label="Import playlists as tags"
              />
            </Tooltip>
          </Box>

          <Box>
            <Tooltip title="Skip audiobooks that already exist in the library (matched by file path or file hash). Uncheck to re-import and overwrite existing entries." placement="right" arrow>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={settings.skipDuplicates}
                    onChange={(event) =>
                      setSettings((prev) => ({
                        ...prev,
                        skipDuplicates: event.target.checked,
                      }))
                    }
                  />
                }
                label="Skip duplicates already in library"
              />
            </Tooltip>
          </Box>
        </Box>

        <Box sx={{ mt: 3 }}>
          <Button
            variant="outlined"
            onClick={handleValidate}
            disabled={!settings.libraryPath || validating || importing}
            startIcon={validating ? undefined : <CheckCircleIcon />}
          >
            {validating ? 'Validating...' : 'Validate Import'}
          </Button>
        </Box>

        {validationResult && (
          <Alert
            severity={validationResult.files_missing > 0 ? 'warning' : 'success'}
            sx={{ mt: 2 }}
          >
            <AlertTitle>Validation Results</AlertTitle>
            <Typography variant="body2">
              Found <strong>{validationResult.audiobook_tracks}</strong>{' '}
              audiobooks ({validationResult.files_found} files found,
              {` ${validationResult.files_missing} missing`})
            </Typography>
            {validationResult.duplicate_count > 0 && (
              <Typography variant="body2" sx={{ mt: 1 }}>
                {validationResult.duplicate_count} potential duplicates detected
              </Typography>
            )}
            <Typography variant="body2" sx={{ mt: 1 }}>
              Estimated import time: {validationResult.estimated_import_time}
            </Typography>
            {validationResult.files_missing > 0 && (
              <Button
                size="small"
                onClick={() => setShowMissingFiles(true)}
                sx={{ mt: 1 }}
              >
                View Missing Files
              </Button>
            )}
          </Alert>
        )}

        <Box sx={{ mt: 3 }}>
          <Button
            variant="contained"
            onClick={handleImport}
            disabled={!validationResult || importing}
            startIcon={importing ? undefined : <CloudUploadIcon />}
          >
            {importing ? 'Importing...' : 'Import Library'}
          </Button>
        </Box>

        {importStatus && (
          <Paper variant="outlined" sx={{ mt: 3, p: 2 }}>
            <Stack direction="row" justifyContent="space-between" alignItems="center" mb={1}>
              <Typography variant="subtitle2">
                Import Progress
              </Typography>
              {importing && (
                <Button
                  size="small"
                  color="error"
                  variant="outlined"
                  onClick={async () => {
                    try {
                      await cancelOperation(importStatus.operation_id);
                      setImporting(false);
                    } catch {
                      toast('Failed to cancel import', 'error');
                    }
                  }}
                >
                  Cancel Import
                </Button>
              )}
            </Stack>

            <LinearProgress
              variant="determinate"
              value={importStatus.progress}
              sx={{ height: 8, borderRadius: 1, mb: 1 }}
            />

            <Stack direction="row" justifyContent="space-between" alignItems="center">
              <Typography variant="body2" color="text.secondary">
                {importStatus.progress}% complete
                {importStatus.processed !== undefined &&
                  importStatus.total_books !== undefined && (
                  <>
                    {' '}&mdash; {importStatus.processed} / {importStatus.total_books} books
                  </>
                )}
              </Typography>
              <Stack direction="row" spacing={1}>
                {importStatus.imported !== undefined && importStatus.imported > 0 && (
                  <Typography variant="caption" color="success.main">
                    {importStatus.imported} imported
                  </Typography>
                )}
                {importStatus.skipped !== undefined && importStatus.skipped > 0 && (
                  <Typography variant="caption" color="text.secondary">
                    {importStatus.skipped} skipped
                  </Typography>
                )}
                {importStatus.failed !== undefined && importStatus.failed > 0 && (
                  <Typography variant="caption" color="error.main">
                    {importStatus.failed} failed
                  </Typography>
                )}
              </Stack>
            </Stack>

            {/* Show current item from message */}
            {importStatus.message && (
              <Typography
                variant="caption"
                color="text.secondary"
                display="block"
                noWrap
                title={importStatus.message}
                sx={{ mt: 0.5 }}
              >
                {importStatus.message}
              </Typography>
            )}

            {importStatus.status === 'completed' && (
              <Alert severity="success" sx={{ mt: 2 }}>
                <AlertTitle>Import Complete</AlertTitle>
                <Typography variant="body2">
                  Imported <strong>{importStatus.imported ?? 0}</strong>{' '}
                  audiobooks
                  {importStatus.skipped !== undefined && importStatus.skipped > 0
                    ? `, skipped ${importStatus.skipped}`
                    : ''}
                  {importStatus.failed !== undefined && importStatus.failed > 0
                    ? `, ${importStatus.failed} failed`
                    : ''}
                </Typography>
              </Alert>
            )}

            {importStatus.status === 'failed' && (
              <Alert severity="error" sx={{ mt: 2 }}>
                <AlertTitle>Import Failed</AlertTitle>
                <Typography variant="body2">{importStatus.message}</Typography>
              </Alert>
            )}
          </Paper>
        )}

        <Divider sx={{ my: 3 }} />

        {/* Force Sync Buttons Section */}
        <Box sx={{ mt: 3, mb: 3 }}>
          <Typography variant="h6" gutterBottom>
            Force Sync Options
          </Typography>
          <Typography variant="body2" color="textSecondary" sx={{ mb: 2 }}>
            Use these buttons for manual sync control. Choose which direction takes precedence.
          </Typography>

          <Stack direction="row" spacing={2} flexWrap="wrap">
            <Button
              variant="contained"
              color="primary"
              startIcon={<SyncIcon />}
              onClick={async () => {
                try {
                  const result = await startITunesSync(settings.libraryPath || undefined, true);
                  if (result.operation_id) {
                    useOperationsStore.getState().startPolling(result.operation_id, 'itunes_sync');
                  } else if (result.message) {
                    toast(result.message, 'warning');
                  }
                } catch (err) {
                  toast(err instanceof Error ? err.message : 'Sync failed', 'error');
                }
              }}
              disabled={!settings.libraryPath || importing}
            >
              Sync Now
            </Button>

            <Button
              variant="contained"
              startIcon={<CloudDownloadIcon />}
              onClick={() => setForceImportConfirmOpen(true)}
              disabled={!validationResult || importing}
            >
              Force Import from iTunes
            </Button>

            <Button
              variant="contained"
              startIcon={<CloudUploadIcon />}
              onClick={() => setForceSyncToITunesConfirmOpen(true)}
              disabled={importing || importStatus?.status === 'in_progress'}
            >
              Force Sync to iTunes
            </Button>

            <Button
              variant="outlined"
              onClick={() => {
                // Retry last failed operation
                if (importStatus?.status === 'failed') {
                  setImporting(true);
                  pollImportStatus(importStatus.operation_id);
                }
              }}
              disabled={!importStatus || importStatus.status !== 'failed'}
            >
              Retry Failed Sync
            </Button>
          </Stack>
        </Box>

        <Divider sx={{ my: 4 }} />

        <Stack spacing={1}>
          <Typography variant="subtitle1">Write Back to iTunes</Typography>
          <Typography variant="body2" color="text.secondary">
            Update file paths in your Library.xml after organizing audiobooks.
          </Typography>
          <Button variant="outlined" onClick={handleOpenWriteBack}>
            Open Write-Back Dialog
          </Button>
        </Stack>

        <Dialog
          open={showMissingFiles}
          onClose={() => setShowMissingFiles(false)}
          maxWidth="md"
          fullWidth
        >
          <DialogTitle>Missing Files ({validationResult?.files_missing ?? 0} total)</DialogTitle>
          <DialogContent>
            {(validationResult?.files_missing ?? 0) > 100 && (
              <Alert severity="info" sx={{ mb: 2 }}>
                Showing first 100 of {validationResult?.files_missing} missing files.
                If these paths are from a different OS, use the Path Mapping fields above
                to translate them to local paths.
              </Alert>
            )}
            <List dense>
              {validationResult?.missing_paths?.map((path) => (
                <ListItem key={path}>
                  <ListItemText primary={path} primaryTypographyProps={{ variant: 'body2', noWrap: true }} />
                </ListItem>
              ))}
            </List>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setShowMissingFiles(false)}>Close</Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={writeBackOpen}
          onClose={() => setWriteBackOpen(false)}
          maxWidth="lg"
          fullWidth
        >
          <DialogTitle>Write Back to iTunes</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              {writeBackLoading && <LinearProgress />}
              {writeBackNotice && (
                <Alert severity={writeBackNotice.severity}>
                  {writeBackNotice.message}
                </Alert>
              )}
              {writeBackResult && (
                <Alert severity={writeBackResult.success ? 'success' : 'warning'}>
                  <Typography variant="body2">
                    {writeBackResult.message}
                  </Typography>
                  <Typography variant="caption" display="block">
                    Updated {writeBackResult.updated_count} entries
                  </Typography>
                  {writeBackResult.backup_path && (
                    <Typography variant="caption" display="block">
                      Backup created at {writeBackResult.backup_path}
                    </Typography>
                  )}
                </Alert>
              )}
              <TextField
                label="Library.xml Path"
                value={writeBackLibraryPath}
                onChange={(event) => setWriteBackLibraryPath(event.target.value)}
                fullWidth
                size="small"
                placeholder="/Users/username/Music/iTunes/Library.xml"
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={writeBackBackup}
                    onChange={(event) => setWriteBackBackup(event.target.checked)}
                  />
                }
                label="Create backup before writing"
              />

              <Tabs value={writeBackMode} onChange={(_e, v) => setWriteBackMode(v)} sx={{ borderBottom: 1, borderColor: 'divider' }}>
                <Tab label="Enter IDs" />
                <Tab label="Sync All" />
                <Tab label="Browse & Select" />
              </Tabs>

              {/* Mode 0: Enter IDs manually */}
              {writeBackMode === 0 && (
                <Stack spacing={2}>
                  <TextField
                    label="Book IDs or iTunes Persistent IDs"
                    value={writeBackIds}
                    onChange={(event) => setWriteBackIds(event.target.value)}
                    placeholder="One ID per line or comma-separated"
                    helperText="Paste audiobook IDs or iTunes persistent IDs."
                    fullWidth
                    multiline
                    minRows={3}
                  />
                  <Stack direction="row" spacing={2}>
                    <Button
                      variant="outlined"
                      onClick={handlePreviewManual}
                      disabled={writeBackLoading || !writeBackIds.trim()}
                    >
                      Preview
                    </Button>
                    <Button
                      variant="contained"
                      onClick={() => {
                        const ids = previewItems.length > 0
                          ? previewItems.map((item) => item.book_id)
                          : parseWriteBackIds(writeBackIds);
                        handleConfirmAndWriteBack(ids);
                      }}
                      disabled={writeBackLoading || !writeBackIds.trim()}
                    >
                      Write Back
                    </Button>
                  </Stack>
                  {previewItems.length > 0 && (
                    <TableContainer component={Paper} variant="outlined" sx={{ maxHeight: 400 }}>
                      <Table size="small" stickyHeader>
                        <TableHead>
                          <TableRow>
                            <TableCell>Title</TableCell>
                            <TableCell>Author</TableCell>
                            <TableCell>Local Path</TableCell>
                            <TableCell>iTunes Path</TableCell>
                            <TableCell>Status</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {previewItems.map((item) => (
                            <TableRow key={item.book_id} sx={item.path_differs ? { bgcolor: 'warning.50' } : undefined}>
                              <TableCell>{item.title}</TableCell>
                              <TableCell>{item.author}</TableCell>
                              <TableCell sx={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                <Tooltip title={item.local_path}><span>{item.local_path}</span></Tooltip>
                              </TableCell>
                              <TableCell sx={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                <Tooltip title={item.itunes_path || '(not in library)'}><span>{item.itunes_path || '(not in library)'}</span></Tooltip>
                              </TableCell>
                              <TableCell>
                                {item.path_differs
                                  ? <Chip label="Differs" color="warning" size="small" />
                                  : <Chip label="Match" color="success" size="small" />}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  )}
                </Stack>
              )}

              {/* Mode 1: Sync All */}
              {writeBackMode === 1 && (
                <Stack spacing={2}>
                  <Paper variant="outlined" sx={{ p: 3, textAlign: 'center' }}>
                    {syncAllCount === null ? (
                      <Typography variant="body1" color="text.secondary">
                        Click "Preview All" to see how many books have different paths.
                      </Typography>
                    ) : (
                      <Typography variant="h5">
                        {syncAllCount} book{syncAllCount !== 1 ? 's' : ''} with path changes
                      </Typography>
                    )}
                  </Paper>
                  <Stack direction="row" spacing={2} justifyContent="center">
                    <Button
                      variant="outlined"
                      onClick={handlePreviewAll}
                      disabled={writeBackLoading || !writeBackLibraryPath.trim()}
                    >
                      Preview All
                    </Button>
                    <Button
                      variant="contained"
                      onClick={() => {
                        const ids = previewItems.filter((item) => item.path_differs).map((item) => item.book_id);
                        if (ids.length === 0) {
                          setWriteBackNotice({ severity: 'warning', message: 'No path changes to sync.' });
                          return;
                        }
                        handleConfirmAndWriteBack(ids);
                      }}
                      disabled={writeBackLoading || syncAllCount === null || syncAllCount === 0}
                    >
                      Sync All ({syncAllCount ?? 0})
                    </Button>
                  </Stack>
                  {previewItems.length > 0 && (
                    <TableContainer component={Paper} variant="outlined" sx={{ maxHeight: 400 }}>
                      <Table size="small" stickyHeader>
                        <TableHead>
                          <TableRow>
                            <TableCell>Title</TableCell>
                            <TableCell>Author</TableCell>
                            <TableCell>Local Path</TableCell>
                            <TableCell>iTunes Path</TableCell>
                            <TableCell>Status</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {previewItems.map((item) => (
                            <TableRow key={item.book_id} sx={item.path_differs ? { bgcolor: 'warning.50' } : undefined}>
                              <TableCell>{item.title}</TableCell>
                              <TableCell>{item.author}</TableCell>
                              <TableCell sx={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                <Tooltip title={item.local_path}><span>{item.local_path}</span></Tooltip>
                              </TableCell>
                              <TableCell sx={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                <Tooltip title={item.itunes_path || '(not in library)'}><span>{item.itunes_path || '(not in library)'}</span></Tooltip>
                              </TableCell>
                              <TableCell>
                                {item.path_differs
                                  ? <Chip label="Differs" color="warning" size="small" />
                                  : <Chip label="Match" color="success" size="small" />}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  )}
                </Stack>
              )}

              {/* Mode 2: Browse & Select */}
              {writeBackMode === 2 && (
                <Stack spacing={2}>
                  <TextField
                    size="small"
                    placeholder="Search by title, author, or path..."
                    value={browseSearch}
                    onChange={(e) => handleBrowseSearchChange(e.target.value)}
                    InputProps={{
                      startAdornment: (
                        <InputAdornment position="start">
                          <SearchIcon />
                        </InputAdornment>
                      ),
                    }}
                    fullWidth
                  />
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Button
                      size="small"
                      onClick={() => {
                        const allIds = new Set(browseItems.map((item) => item.book_id));
                        setBrowseSelected((prev) => {
                          const next = new Set(prev);
                          allIds.forEach((id) => next.add(id));
                          return next;
                        });
                      }}
                    >
                      Select All Visible
                    </Button>
                    <Button size="small" onClick={() => setBrowseSelected(new Set())}>
                      Deselect All
                    </Button>
                    <Typography variant="body2" color="text.secondary" sx={{ ml: 'auto' }}>
                      {browseSelected.size} selected
                    </Typography>
                  </Stack>
                  {browseLoading && <LinearProgress />}
                  <TableContainer component={Paper} variant="outlined" sx={{ maxHeight: 400 }}>
                    <Table size="small" stickyHeader>
                      <TableHead>
                        <TableRow>
                          <TableCell padding="checkbox">
                            <Checkbox
                              indeterminate={browseSelected.size > 0 && browseSelected.size < browseItems.length}
                              checked={browseItems.length > 0 && browseItems.every((item) => browseSelected.has(item.book_id))}
                              onChange={(e) => {
                                if (e.target.checked) {
                                  setBrowseSelected((prev) => {
                                    const next = new Set(prev);
                                    browseItems.forEach((item) => next.add(item.book_id));
                                    return next;
                                  });
                                } else {
                                  setBrowseSelected((prev) => {
                                    const next = new Set(prev);
                                    browseItems.forEach((item) => next.delete(item.book_id));
                                    return next;
                                  });
                                }
                              }}
                            />
                          </TableCell>
                          <TableCell>Title</TableCell>
                          <TableCell>Author</TableCell>
                          <TableCell>Local Path</TableCell>
                          <TableCell>iTunes ID</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {browseItems.map((item) => (
                          <TableRow
                            key={item.book_id}
                            hover
                            onClick={() => {
                              setBrowseSelected((prev) => {
                                const next = new Set(prev);
                                if (next.has(item.book_id)) {
                                  next.delete(item.book_id);
                                } else {
                                  next.add(item.book_id);
                                }
                                return next;
                              });
                            }}
                            sx={{ cursor: 'pointer' }}
                          >
                            <TableCell padding="checkbox">
                              <Checkbox checked={browseSelected.has(item.book_id)} />
                            </TableCell>
                            <TableCell>{item.title}</TableCell>
                            <TableCell>{item.author}</TableCell>
                            <TableCell sx={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                              <Tooltip title={item.local_path}><span>{item.local_path}</span></Tooltip>
                            </TableCell>
                            <TableCell>
                              <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                                {item.itunes_persistent_id}
                              </Typography>
                            </TableCell>
                          </TableRow>
                        ))}
                        {browseItems.length === 0 && !browseLoading && (
                          <TableRow>
                            <TableCell colSpan={5} align="center">
                              <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
                                {browseTotal === 0 ? 'No books with iTunes IDs found.' : 'Loading...'}
                              </Typography>
                            </TableCell>
                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                  </TableContainer>
                  <TablePagination
                    component="div"
                    count={browseTotal}
                    page={browsePage}
                    onPageChange={(_e, newPage) => {
                      setBrowsePage(newPage);
                      loadBrowseBooks(browseSearch, newPage, browseRowsPerPage);
                    }}
                    rowsPerPage={browseRowsPerPage}
                    onRowsPerPageChange={(e) => {
                      const rpp = parseInt(e.target.value, 10);
                      setBrowseRowsPerPage(rpp);
                      setBrowsePage(0);
                      loadBrowseBooks(browseSearch, 0, rpp);
                    }}
                    rowsPerPageOptions={[10, 25, 50, 100]}
                  />
                  <Button
                    variant="contained"
                    onClick={() => handleConfirmAndWriteBack(Array.from(browseSelected))}
                    disabled={writeBackLoading || browseSelected.size === 0}
                  >
                    Write Back Selected ({browseSelected.size})
                  </Button>
                </Stack>
              )}
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setWriteBackOpen(false)} disabled={writeBackLoading}>
              Close
            </Button>
          </DialogActions>
        </Dialog>

        {/* Confirmation dialog before write-back */}
        <Dialog open={confirmWriteBackOpen} onClose={() => setConfirmWriteBackOpen(false)}>
          <DialogTitle>Confirm Write-Back</DialogTitle>
          <DialogContent>
            <Typography>
              This will update {pendingWriteBackIds.length} book path{pendingWriteBackIds.length !== 1 ? 's' : ''} in your iTunes Library.xml.
              {writeBackBackup ? ' A backup will be created first.' : ' No backup will be created.'}
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setConfirmWriteBackOpen(false)}>Cancel</Button>
            <Button
              variant="contained"
              onClick={() => {
                setConfirmWriteBackOpen(false);
                executeWriteBack(pendingWriteBackIds);
              }}
            >
              Confirm ({pendingWriteBackIds.length})
            </Button>
          </DialogActions>
        </Dialog>

        <ITunesConflictDialog
          open={showConflictDialog}
          conflicts={pendingConflicts}
          loading={syncingWithConflicts}
          onResolve={handleConflictResolve}
          onCancel={() => setShowConflictDialog(false)}
        />

        <Dialog open={overwriteConfirmOpen} onClose={() => setOverwriteConfirmOpen(false)}>
          <DialogTitle>Library Modified</DialogTitle>
          <DialogContent>
            <Typography>
              The iTunes library has been modified since your last import. Writing back now may overwrite those external changes.
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setOverwriteConfirmOpen(false)}>Cancel</Button>
            <Button
              color="warning"
              variant="contained"
              onClick={() => {
                setOverwriteConfirmOpen(false);
                executeWriteBack(pendingWriteBackIds, true);
              }}
            >
              Overwrite Anyway
            </Button>
          </DialogActions>
        </Dialog>
        <Dialog open={forceImportConfirmOpen} onClose={() => setForceImportConfirmOpen(false)}>
          <DialogTitle>Force Import from iTunes</DialogTitle>
          <DialogContent>
            <Typography>
              Force import from iTunes will overwrite organizer changes. Continue?
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setForceImportConfirmOpen(false)}>Cancel</Button>
            <Button
              color="warning"
              variant="contained"
              onClick={async () => {
                setForceImportConfirmOpen(false);
                setImporting(true);
                try {
                  const request: ITunesImportRequest = {
                    library_path: settings.libraryPath,
                    import_mode: 'import',
                    preserve_location: settings.preserveLocation,
                    import_playlists: settings.importPlaylists,
                    skip_duplicates: settings.skipDuplicates,
                    path_mappings: settings.pathMappings.filter((m) => m.from && m.to),
                  };
                  const result = await importITunesLibrary(request);
                  useOperationsStore.getState().startPolling(result.operation_id, 'itunes_import');
                  await pollImportStatus(result.operation_id);
                } catch (err) {
                  toast(err instanceof Error ? err.message : 'Force import failed', 'error');
                  setImporting(false);
                }
              }}
            >
              Force Import
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog open={forceSyncToITunesConfirmOpen} onClose={() => setForceSyncToITunesConfirmOpen(false)}>
          <DialogTitle>Force Sync to iTunes</DialogTitle>
          <DialogContent>
            <Typography>
              Force sync to iTunes will overwrite iTunes changes. Continue?
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setForceSyncToITunesConfirmOpen(false)}>Cancel</Button>
            <Button
              color="warning"
              variant="contained"
              onClick={() => {
                setForceSyncToITunesConfirmOpen(false);
                setWriteBackOpen(true);
                setWriteBackIds('*');
              }}
            >
              Force Sync
            </Button>
          </DialogActions>
        </Dialog>
      </CardContent>
    </Card>
  );
}
