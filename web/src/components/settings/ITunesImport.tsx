// file: web/src/components/settings/ITunesImport.tsx
// version: 1.0.0
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
  Radio,
  RadioGroup,
  TextField,
  Typography,
} from '@mui/material';
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import {
  getITunesImportStatus,
  importITunesLibrary,
  type ITunesImportRequest,
  type ITunesImportStatus,
  type ITunesValidateResponse,
  validateITunesLibrary,
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
      </CardContent>
    </Card>
  );
}
