// file: web/src/components/wizard/WelcomeWizard.tsx
// version: 1.0.2
// guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e

import { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Stepper,
  Step,
  StepLabel,
  Typography,
  TextField,
  Box,
  Alert,
  InputAdornment,
  CircularProgress,
} from '@mui/material';
import {
  Folder as FolderIcon,
  Settings as SettingsIcon,
  CheckCircle as CheckCircleIcon,
} from '@mui/icons-material';
import { ServerFileBrowser } from '../common/ServerFileBrowser';
import * as api from '../../services/api';

interface WelcomeWizardProps {
  open: boolean;
  onComplete: () => void;
}

/**
 * WelcomeWizard component for first-run setup
 *
 * Guides users through initial configuration:
 * 1. Set library path (where organized books go)
 * 2. Optional OpenAI API key setup
 * 3. Add import/download folder paths
 */
export function WelcomeWizard({ open, onComplete }: WelcomeWizardProps) {
  const [activeStep, setActiveStep] = useState(0);
  const [libraryPath, setLibraryPath] = useState('');
  const [showLibraryBrowser, setShowLibraryBrowser] = useState(false);
  const [openaiKey, setOpenaiKey] = useState('');
  const [testingKey, setTestingKey] = useState(false);
  const [keyTestResult, setKeyTestResult] = useState<
    'success' | 'error' | null
  >(null);
  const [importFolders, setImportFolders] = useState<string[]>([]);
  const [showImportBrowser, setShowImportBrowser] = useState(false);
  const [saving, setSaving] = useState(false);

  const steps = ['Library Path', 'AI Setup (Optional)', 'Import Folders'];

  const handleNext = () => {
    setActiveStep((prev) => prev + 1);
  };

  const handleBack = () => {
    setActiveStep((prev) => prev - 1);
  };

  const handleLibraryPathSelect = (path: string) => {
    setLibraryPath(path);
    setShowLibraryBrowser(false);
  };

  const handleTestOpenAIKey = async () => {
    if (!openaiKey.trim()) {
      setKeyTestResult('error');
      return;
    }

    setTestingKey(true);
    setKeyTestResult(null);

    try {
      // Test the key by making a simple API call
      const response = await fetch('https://api.openai.com/v1/models', {
        headers: {
          Authorization: `Bearer ${openaiKey}`,
        },
      });

      if (response.ok) {
        setKeyTestResult('success');
      } else {
        setKeyTestResult('error');
      }
    } catch (error) {
      setKeyTestResult('error');
    } finally {
      setTestingKey(false);
    }
  };

  const handleAddImportFolder = (path: string) => {
    if (path && !importFolders.includes(path)) {
      setImportFolders([...importFolders, path]);
    }
    setShowImportBrowser(false);
  };

  const handleRemoveImportFolder = (index: number) => {
    setImportFolders(importFolders.filter((_, i) => i !== index));
  };

  const handleComplete = async () => {
    setSaving(true);

    try {
      // Step 1: Save library path to config
      await api.updateConfig({
        root_dir: libraryPath,
        playlist_dir: `${libraryPath}/playlists`,
      });

      // Step 2: Save OpenAI key if provided
      if (openaiKey.trim()) {
        await api.updateConfig({
          openai_api_key: openaiKey,
          enable_ai_parsing: true,
        });
      }

      // Step 3: Add import folders
      for (const path of importFolders) {
        try {
          await api.addImportPath(path, path.split('/').pop() || path);
        } catch (error) {
          console.error(`Failed to add import folder ${path}:`, error);
        }
      }

      // Mark wizard as completed (store in localStorage for now)
      localStorage.setItem('welcome_wizard_completed', 'true');

      onComplete();
    } catch (error) {
      console.error('Failed to complete setup:', error);
      alert('Failed to save settings. Please check the console for details.');
    } finally {
      setSaving(false);
    }
  };

  const canProceed = () => {
    switch (activeStep) {
      case 0:
        return libraryPath.trim() !== '';
      case 1:
        return true; // Optional step
      case 2:
        return true; // Import folders are optional
      default:
        return false;
    }
  };

  return (
    <>
      <Dialog open={open} maxWidth="md" fullWidth disableEscapeKeyDown>
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <SettingsIcon />
            <Typography variant="h6">Welcome to Audiobook Organizer</Typography>
          </Box>
        </DialogTitle>

        <DialogContent>
          <Stepper activeStep={activeStep} sx={{ mb: 4 }}>
            {steps.map((label) => (
              <Step key={label}>
                <StepLabel>{label}</StepLabel>
              </Step>
            ))}
          </Stepper>

          {/* Step 1: Library Path */}
          {activeStep === 0 && (
            <Box>
              <Typography variant="h6" gutterBottom>
                Set Your Library Path
              </Typography>
              <Typography variant="body2" color="text.secondary" paragraph>
                This is where your organized audiobooks will be stored. The app
                will create a structured folder hierarchy here based on your
                naming patterns.
              </Typography>

              <TextField
                fullWidth
                label="Library Path"
                value={libraryPath}
                onChange={(e) => setLibraryPath(e.target.value)}
                placeholder="/path/to/audiobooks/library"
                sx={{ mb: 2 }}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <Button
                        variant="outlined"
                        size="small"
                        onClick={() => setShowLibraryBrowser(true)}
                      >
                        Browse
                      </Button>
                    </InputAdornment>
                  ),
                }}
              />

              <Alert severity="info">
                Choose a location with plenty of storage space. This will be the
                permanent home for your organized audiobook collection.
              </Alert>
            </Box>
          )}

          {/* Step 2: OpenAI API Key */}
          {activeStep === 1 && (
            <Box>
              <Typography variant="h6" gutterBottom>
                AI-Powered Metadata (Optional)
              </Typography>
              <Typography variant="body2" color="text.secondary" paragraph>
                Enable AI-powered author name parsing and metadata enhancement
                with OpenAI. This is optional and can be configured later in
                Settings.
              </Typography>

              <TextField
                fullWidth
                label="OpenAI API Key"
                type="password"
                value={openaiKey}
                onChange={(e) => setOpenaiKey(e.target.value)}
                placeholder="sk-..."
                sx={{ mb: 2 }}
                helperText="Get your API key from platform.openai.com"
              />

              <Button
                variant="outlined"
                onClick={handleTestOpenAIKey}
                disabled={!openaiKey.trim() || testingKey}
                startIcon={testingKey ? <CircularProgress size={16} /> : null}
                sx={{ mb: 2 }}
              >
                Test Connection
              </Button>

              {keyTestResult === 'success' && (
                <Alert severity="success" sx={{ mb: 2 }}>
                  API key is valid and working!
                </Alert>
              )}

              {keyTestResult === 'error' && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  Invalid API key or connection failed. Please check your key.
                </Alert>
              )}

              <Alert severity="info">
                You can skip this step and configure it later in Settings if
                needed.
              </Alert>
            </Box>
          )}

          {/* Step 3: Import Folders */}
          {activeStep === 2 && (
            <Box>
              <Typography variant="h6" gutterBottom>
                Add Import Folders
              </Typography>
              <Typography variant="body2" color="text.secondary" paragraph>
                Import folders are watched locations where the scanner looks for
                new audiobooks. Files found here will be scanned and organized
                into your library path.
              </Typography>

              <Box sx={{ mb: 2 }}>
                <Button
                  variant="outlined"
                  startIcon={<FolderIcon />}
                  onClick={() => setShowImportBrowser(true)}
                  sx={{ mb: 2 }}
                >
                  Add Import Folder
                </Button>

                {importFolders.length > 0 && (
                  <Box>
                    <Typography variant="subtitle2" gutterBottom>
                      Import Folders ({importFolders.length}):
                    </Typography>
                    {importFolders.map((folder, index) => (
                      <Box
                        key={index}
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          p: 1,
                          mb: 1,
                          bgcolor: 'action.hover',
                          borderRadius: 1,
                        }}
                      >
                        <Typography variant="body2">{folder}</Typography>
                        <Button
                          size="small"
                          color="error"
                          onClick={() => handleRemoveImportFolder(index)}
                        >
                          Remove
                        </Button>
                      </Box>
                    ))}
                  </Box>
                )}
              </Box>

              <Alert severity="info">
                You can add import folders later in the Library page if you
                prefer to skip this step.
              </Alert>
            </Box>
          )}
        </DialogContent>

        <DialogActions>
          <Button onClick={handleBack} disabled={activeStep === 0 || saving}>
            Back
          </Button>
          <Box sx={{ flex: 1 }} />
          {activeStep === steps.length - 1 ? (
            <Button
              variant="contained"
              onClick={handleComplete}
              disabled={saving}
              startIcon={
                saving ? <CircularProgress size={16} /> : <CheckCircleIcon />
              }
            >
              {saving ? 'Saving...' : 'Complete Setup'}
            </Button>
          ) : (
            <Button
              variant="contained"
              onClick={handleNext}
              disabled={!canProceed()}
            >
              Next
            </Button>
          )}
        </DialogActions>
      </Dialog>

      {/* Library Path Browser Dialog */}
      <Dialog
        open={showLibraryBrowser}
        onClose={() => setShowLibraryBrowser(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Select Library Path</DialogTitle>
        <DialogContent>
          <ServerFileBrowser
            onSelect={handleLibraryPathSelect}
            initialPath="/home"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setShowLibraryBrowser(false)}>Cancel</Button>
        </DialogActions>
      </Dialog>

      {/* Import Folder Browser Dialog */}
      <Dialog
        open={showImportBrowser}
        onClose={() => setShowImportBrowser(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Select Import Folder</DialogTitle>
        <DialogContent>
          <ServerFileBrowser
            onSelect={handleAddImportFolder}
            initialPath="/home"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setShowImportBrowser(false)}>Cancel</Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
