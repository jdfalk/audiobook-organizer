// file: web/src/components/settings/ITunesImport.tsx
// version: 1.2.0
// guid: 4eb9b74d-7192-497b-849a-092833ae63a4

import { useEffect, useRef, useState } from 'react';
import {
  Alert,
  AlertTitle,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Checkbox,
  Divider,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  FormLabel,
  LinearProgress,
  List,
  ListItem,
  ListItemText,
  Paper,
  Radio,
  RadioGroup,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import { ITunesConflictDialog, type ConflictItem } from './ITunesConflictDialog';
import {
  getBook,
  getITunesImportStatus,
  importITunesLibrary,
  type Book,
  type ITunesImportRequest,
  type ITunesImportStatus,
  type ITunesValidateResponse,
  type ITunesWriteBackResponse,
  validateITunesLibrary,
  writeBackITunesLibrary,
} from '../../services/api';

interface ITunesImportSettings {
  libraryPath: string;
  importMode: 'organized' | 'import' | 'organize';
  preserveLocation: boolean;
  importPlaylists: boolean;
  skipDuplicates: boolean;
}

const defaultSettings: ITunesImportSettings = {
  libraryPath: '',
  importMode: 'import',
  preserveLocation: false,
  importPlaylists: true,
  skipDuplicates: true,
};

/**
 * ITunesImport provides a guided workflow to validate and import iTunes
 * Library.xml metadata into the audiobook organizer.
 */
export function ITunesImport() {
  const [settings, setSettings] = useState<ITunesImportSettings>(
    defaultSettings
  );
  const [validationResult, setValidationResult] =
    useState<ITunesValidateResponse | null>(null);
  const [validating, setValidating] = useState(false);
  const [importing, setImporting] = useState(false);
  const [importStatus, setImportStatus] =
    useState<ITunesImportStatus | null>(null);
  const [showMissingFiles, setShowMissingFiles] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [writeBackOpen, setWriteBackOpen] = useState(false);
  const [writeBackIds, setWriteBackIds] = useState('');
  const [writeBackBooks, setWriteBackBooks] = useState<Book[]>([]);
  const [writeBackLoading, setWriteBackLoading] = useState(false);
  const [writeBackNotice, setWriteBackNotice] = useState<{
    severity: 'error' | 'warning' | 'success';
    message: string;
  } | null>(null);
  const [writeBackResult, setWriteBackResult] =
    useState<ITunesWriteBackResponse | null>(null);
  const [writeBackBackup, setWriteBackBackup] = useState(true);
  const [writeBackLibraryPath, setWriteBackLibraryPath] = useState('');
  const [showConflictDialog, setShowConflictDialog] = useState(false);
  const [pendingConflicts] = useState<ConflictItem[]>([]);
  const [syncingWithConflicts, setSyncingWithConflicts] = useState(false);
  const pollTimeoutRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (pollTimeoutRef.current) {
        window.clearTimeout(pollTimeoutRef.current);
      }
    };
  }, []);

  const handleBrowseFile = () => {
    const path = window.prompt('Enter path to iTunes Library.xml:');
    if (path) {
      setSettings((prev) => ({ ...prev, libraryPath: path }));
    }
  };

  const handleValidate = async () => {
    setValidating(true);
    setError(null);
    setValidationResult(null);

    try {
      const result = await validateITunesLibrary({
        library_path: settings.libraryPath,
      });
      setValidationResult(result);
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Validation failed';
      setError(message);
    } finally {
      setValidating(false);
    }
  };

  const handleImport = async () => {
    setImporting(true);
    setError(null);
    setImportStatus(null);

    try {
      const request: ITunesImportRequest = {
        library_path: settings.libraryPath,
        import_mode: settings.importMode,
        preserve_location: settings.preserveLocation,
        import_playlists: settings.importPlaylists,
        skip_duplicates: settings.skipDuplicates,
      };

      const result = await importITunesLibrary(request);
      await pollImportStatus(result.operation_id);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Import failed';
      setError(message);
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
    setWriteBackBooks([]);
    setWriteBackNotice(null);
    setWriteBackResult(null);
    setWriteBackBackup(true);
    setWriteBackLibraryPath(settings.libraryPath);
  };

  const handleLoadWriteBackPreview = async () => {
    const ids = parseWriteBackIds(writeBackIds);
    if (!writeBackLibraryPath.trim()) {
      setWriteBackBooks([]);
      setWriteBackNotice({
        severity: 'error',
        message: 'Library path is required for write-back.',
      });
      return;
    }
    if (ids.length === 0) {
      setWriteBackBooks([]);
      setWriteBackNotice({
        severity: 'error',
        message: 'Enter one or more audiobook IDs to preview.',
      });
      return;
    }

    setWriteBackLoading(true);
    setWriteBackNotice(null);
    setWriteBackResult(null);

    try {
      const results = await Promise.allSettled(ids.map((id) => getBook(id)));
      const books: Book[] = [];
      const missing: string[] = [];

      results.forEach((result, index) => {
        if (result.status === 'fulfilled') {
          books.push(result.value);
        } else {
          missing.push(ids[index]);
        }
      });

      const eligible = books.filter((book) => book.itunes_persistent_id);
      setWriteBackBooks(eligible);

      const excludedCount = books.length - eligible.length;
      if (missing.length > 0 || excludedCount > 0) {
        const parts = [];
        if (missing.length > 0) {
          parts.push(
            `${missing.length} missing ID${missing.length === 1 ? '' : 's'}`
          );
        }
        if (excludedCount > 0) {
          parts.push(`${excludedCount} missing iTunes persistent ID`);
        }
        setWriteBackNotice({
          severity: 'warning',
          message: `Preview loaded with ${parts.join(' and ')}.`,
        });
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Failed to load preview.';
      setWriteBackNotice({ severity: 'error', message });
    } finally {
      setWriteBackLoading(false);
    }
  };

  const handleWriteBack = async () => {
    if (!writeBackLibraryPath.trim()) {
      setWriteBackNotice({
        severity: 'error',
        message: 'Library path is required for write-back.',
      });
      return;
    }
    if (writeBackBooks.length === 0) {
      setWriteBackNotice({
        severity: 'error',
        message: 'Load at least one audiobook with an iTunes persistent ID.',
      });
      return;
    }

    setWriteBackLoading(true);
    setWriteBackNotice(null);
    setWriteBackResult(null);

    try {
      const result = await writeBackITunesLibrary({
        library_path: writeBackLibraryPath,
        audiobook_ids: writeBackBooks.map((book) => book.id),
        create_backup: writeBackBackup,
      });
      setWriteBackResult(result);
      setWriteBackNotice({
        severity: 'success',
        message: result.message || `Updated ${result.updated_count} entries.`,
      });
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Write-back failed.';
      setWriteBackNotice({ severity: 'error', message });
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
        setError(message);
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
      setError(null);
      // Refresh sync status
      if (importStatus?.operation_id) {
        await pollImportStatus(importStatus.operation_id);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Conflict resolution failed');
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

        {error && (
          <Alert severity="error" sx={{ mt: 2 }} onClose={() => setError(null)}>
            {error}
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
              <FormControlLabel
                value="organized"
                control={<Radio />}
                label="Files already organized"
              />
              <FormControlLabel
                value="import"
                control={<Radio />}
                label="Import metadata only"
              />
              <FormControlLabel
                value="organize"
                control={<Radio />}
                label="Import and organize now"
              />
            </RadioGroup>
          </FormControl>

          <Box sx={{ mt: 2 }}>
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
          </Box>

          <Box>
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
          </Box>

          <Box>
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
          </Box>
        </Box>

        <Box sx={{ mt: 3 }}>
          <Button
            variant="outlined"
            onClick={handleValidate}
            disabled={!settings.libraryPath || validating}
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
          <Box sx={{ mt: 3 }}>
            <Typography variant="body2" gutterBottom>
              {importStatus.message}
            </Typography>
            <LinearProgress
              variant="determinate"
              value={importStatus.progress}
              sx={{ mt: 1 }}
            />
            <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
              {importStatus.progress}% complete
              {importStatus.processed !== undefined &&
                importStatus.total_books !== undefined && (
                <>
                  {' '}
                  ({importStatus.processed} / {importStatus.total_books} processed)
                </>
              )}
            </Typography>

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
          </Box>
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
              startIcon={<CloudDownloadIcon />}
              onClick={async () => {
                if (window.confirm('Force import from iTunes will overwrite organizer changes. Continue?')) {
                  setImporting(true);
                  try {
                    const request: ITunesImportRequest = {
                      library_path: settings.libraryPath,
                      import_mode: 'import',
                      preserve_location: settings.preserveLocation,
                      import_playlists: settings.importPlaylists,
                      skip_duplicates: settings.skipDuplicates,
                    };
                    const result = await importITunesLibrary(request);
                    await pollImportStatus(result.operation_id);
                  } catch (err) {
                    setError(err instanceof Error ? err.message : 'Force import failed');
                    setImporting(false);
                  }
                }
              }}
              disabled={!validationResult || importing}
            >
              Force Import from iTunes
            </Button>

            <Button
              variant="contained"
              startIcon={<CloudUploadIcon />}
              onClick={async () => {
                if (window.confirm('Force sync to iTunes will overwrite iTunes changes. Continue?')) {
                  setWriteBackOpen(true);
                  // Set all books for write-back (force mode)
                  setWriteBackIds('*');
                }
              }}
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
          <DialogTitle>Missing Files</DialogTitle>
          <DialogContent>
            <List>
              {validationResult?.missing_paths?.map((path) => (
                <ListItem key={path}>
                  <ListItemText primary={path} />
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
          maxWidth="md"
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
                placeholder="/Users/username/Music/iTunes/Library.xml"
              />
              <TextField
                label="Audiobook IDs"
                value={writeBackIds}
                onChange={(event) => setWriteBackIds(event.target.value)}
                placeholder="One ID per line or comma-separated"
                helperText="Paste audiobook IDs to update in iTunes."
                fullWidth
                multiline
                minRows={3}
              />
              <Stack direction="row" spacing={2} alignItems="center">
                <Button
                  variant="outlined"
                  onClick={handleLoadWriteBackPreview}
                  disabled={writeBackLoading}
                >
                  Load Preview
                </Button>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={writeBackBackup}
                      onChange={(event) =>
                        setWriteBackBackup(event.target.checked)
                      }
                    />
                  }
                  label="Create backup before writing"
                />
              </Stack>
              {writeBackBooks.length > 0 && (
                <TableContainer component={Paper} variant="outlined">
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>Title</TableCell>
                        <TableCell>File Path</TableCell>
                        <TableCell>iTunes ID</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {writeBackBooks.map((book) => (
                        <TableRow key={book.id}>
                          <TableCell>{book.title}</TableCell>
                          <TableCell>{book.file_path}</TableCell>
                          <TableCell>{book.itunes_persistent_id}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button
              onClick={() => setWriteBackOpen(false)}
              disabled={writeBackLoading}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              onClick={handleWriteBack}
              disabled={writeBackLoading || writeBackBooks.length === 0}
            >
              {writeBackLoading
                ? 'Writing...'
                : `Update ${writeBackBooks.length} entries`}
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
      </CardContent>
    </Card>
  );
}
