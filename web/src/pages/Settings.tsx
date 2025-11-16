// file: web/src/pages/Settings.tsx
// version: 1.0.0
// guid: 6d7e8f9a-0b1c-2d3e-4f5a-6b7c8d9e0f1a

import { Box, Typography } from '@mui/material';

// file: web/src/pages/Settings.tsx
// version: 1.1.0
// guid: 5a6b7c8d-9e0f-1a2b-3c4d-5e6f7a8b9c0d

import { useState } from 'react';
import {
  Box,
  Typography,
  Paper,
  Tabs,
  Tab,
  TextField,
  Button,
  Grid,
  Switch,
  FormControlLabel,
  Alert,
  Divider,
} from '@mui/material';
import { Save as SaveIcon, RestartAlt as RestartAltIcon } from '@mui/icons-material';

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index, ...other } = props;

  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`settings-tabpanel-${index}`}
      aria-labelledby={`settings-tab-${index}`}
      {...other}
    >
      {value === index && <Box sx={{ p: 3 }}>{children}</Box>}
    </div>
  );
}

export function Settings() {
  const [tabValue, setTabValue] = useState(0);
  const [settings, setSettings] = useState({
    // Library settings
    scanOnStartup: false,
    autoOrganize: true,
    fileNamingPattern: '{author} - {title}',
    createBackups: true,
    
    // Metadata settings
    autoFetchMetadata: true,
    metadataSource: 'audible',
    language: 'en',
    
    // Performance settings
    concurrentScans: 4,
    cacheSize: 1000,
    logLevel: 'info',
  });
  const [saved, setSaved] = useState(false);

  const handleChange = (field: string, value: string | boolean | number) => {
    setSettings((prev) => ({ ...prev, [field]: value }));
    setSaved(false);
  };

  const handleSave = async () => {
    try {
      // TODO: Replace with actual API call
      // await fetch('/api/v1/settings', {
      //   method: 'PUT',
      //   headers: { 'Content-Type': 'application/json' },
      //   body: JSON.stringify(settings)
      // });
      console.log('Saved settings:', settings);
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    } catch (error) {
      console.error('Failed to save settings:', error);
    }
  };

  const handleReset = () => {
    if (!confirm('Reset all settings to defaults?')) return;
    
    setSettings({
      scanOnStartup: false,
      autoOrganize: true,
      fileNamingPattern: '{author} - {title}',
      createBackups: true,
      autoFetchMetadata: true,
      metadataSource: 'audible',
      language: 'en',
      concurrentScans: 4,
      cacheSize: 1000,
      logLevel: 'info',
    });
  };

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        Settings
      </Typography>

      {saved && (
        <Alert severity="success" sx={{ mb: 2 }}>
          Settings saved successfully!
        </Alert>
      )}

      <Paper>
        <Tabs
          value={tabValue}
          onChange={(_, newValue) => setTabValue(newValue)}
          aria-label="settings tabs"
        >
          <Tab label="Library" />
          <Tab label="Metadata" />
          <Tab label="Performance" />
        </Tabs>

        <TabPanel value={tabValue} index={0}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom>
                Library Settings
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.scanOnStartup}
                    onChange={(e) => handleChange('scanOnStartup', e.target.checked)}
                  />
                }
                label="Scan library on startup"
              />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.autoOrganize}
                    onChange={(e) => handleChange('autoOrganize', e.target.checked)}
                  />
                }
                label="Automatically organize audiobooks"
              />
            </Grid>

            <Grid item xs={12}>
              <TextField
                fullWidth
                label="File Naming Pattern"
                value={settings.fileNamingPattern}
                onChange={(e) => handleChange('fileNamingPattern', e.target.value)}
                helperText="Available variables: {title}, {author}, {series}, {series_number}, {year}"
              />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.createBackups}
                    onChange={(e) => handleChange('createBackups', e.target.checked)}
                  />
                }
                label="Create backups before modifying files"
              />
            </Grid>
          </Grid>
        </TabPanel>

        <TabPanel value={tabValue} index={1}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom>
                Metadata Settings
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.autoFetchMetadata}
                    onChange={(e) => handleChange('autoFetchMetadata', e.target.checked)}
                  />
                }
                label="Automatically fetch missing metadata"
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                select
                label="Metadata Source"
                value={settings.metadataSource}
                onChange={(e) => handleChange('metadataSource', e.target.value)}
                SelectProps={{ native: true }}
              >
                <option value="audible">Audible</option>
                <option value="goodreads">Goodreads</option>
                <option value="openlibrary">Open Library</option>
              </TextField>
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="Default Language"
                value={settings.language}
                onChange={(e) => handleChange('language', e.target.value)}
                helperText="ISO 639-1 language code (e.g., en, es, fr)"
              />
            </Grid>
          </Grid>
        </TabPanel>

        <TabPanel value={tabValue} index={2}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom>
                Performance Settings
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                type="number"
                label="Concurrent Scans"
                value={settings.concurrentScans}
                onChange={(e) => handleChange('concurrentScans', parseInt(e.target.value) || 1)}
                inputProps={{ min: 1, max: 16 }}
                helperText="Number of folders to scan simultaneously"
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                type="number"
                label="Cache Size (items)"
                value={settings.cacheSize}
                onChange={(e) => handleChange('cacheSize', parseInt(e.target.value) || 100)}
                inputProps={{ min: 100, max: 10000 }}
                helperText="Number of items to cache in memory"
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                select
                label="Log Level"
                value={settings.logLevel}
                onChange={(e) => handleChange('logLevel', e.target.value)}
                SelectProps={{ native: true }}
                helperText="Logging verbosity level"
              >
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warning</option>
                <option value="error">Error</option>
              </TextField>
            </Grid>
          </Grid>
        </TabPanel>

        <Box sx={{ p: 2, display: 'flex', gap: 2, justifyContent: 'flex-end' }}>
          <Button
            variant="outlined"
            startIcon={<RestartAltIcon />}
            onClick={handleReset}
          >
            Reset to Defaults
          </Button>
          <Button
            variant="contained"
            startIcon={<SaveIcon />}
            onClick={handleSave}
          >
            Save Settings
          </Button>
        </Box>
      </Paper>
    </Box>
  );
}
