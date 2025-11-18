// file: web/src/pages/Settings.tsx
// version: 1.11.0
// guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d

import { useState, useEffect } from 'react';
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
  Collapse,
  IconButton,
  MenuItem,
  InputAdornment,
  Radio,
  RadioGroup,
  FormControl,
  FormLabel,
} from '@mui/material';
import * as api from '../services/api';
import {
  Save as SaveIcon,
  RestartAlt as RestartAltIcon,
  DragHandle as DragHandleIcon,
  CheckBox as CheckBoxIcon,
  CheckBoxOutlineBlank as CheckBoxOutlineBlankIcon,
  ExpandMore as ExpandMoreIcon,
  Settings as SettingsIcon,
} from '@mui/icons-material';

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index, ...other } = props;

  return (
    <Box
      role="tabpanel"
      hidden={value !== index}
      id={`settings-tabpanel-${index}`}
      aria-labelledby={`settings-tab-${index}`}
      sx={{
        overflowY: 'auto',
        overflowX: 'hidden',
        flex: 1,
        minHeight: 0,
        p: 3
      }}
      {...other}
    >
      {value === index && children}
    </Box>
  );
}

export function Settings() {
  const [tabValue, setTabValue] = useState(0);
  const [settings, setSettings] = useState({
    // Library settings
    libraryPath: '/path/to/audiobooks/library',
    organizationStrategy: 'auto', // 'auto', 'copy', 'hardlink', 'reflink', 'symlink'
    scanOnStartup: false,
    autoOrganize: true,
    folderNamingPattern: '{author}/{series}/{title} ({print_year})',
    fileNamingPattern: '{title} - {author} - read by {narrator}',
    createBackups: true,

    // Storage quotas
    enableDiskQuota: false,
    diskQuotaPercent: 80,
    enableUserQuotas: false,
    defaultUserQuotaGB: 100,

    // Metadata settings
    autoFetchMetadata: true,
    metadataSources: [
      {
        id: 'audible',
        name: 'Audible',
        enabled: true,
        priority: 1,
        requiresAuth: false,
        credentials: {}
      },
      {
        id: 'goodreads',
        name: 'Goodreads',
        enabled: true,
        priority: 2,
        requiresAuth: true,
        credentials: { apiKey: '', apiSecret: '' }
      },
      {
        id: 'openlibrary',
        name: 'Open Library',
        enabled: false,
        priority: 3,
        requiresAuth: true,
        credentials: { apiKey: '' }
      },
      {
        id: 'google-books',
        name: 'Google Books',
        enabled: false,
        priority: 4,
        requiresAuth: true,
        credentials: { apiKey: '' }
      },
    ],
    language: 'en',

    // Performance settings
    concurrentScans: 4,

    // Memory management
    memoryLimitType: 'items', // 'items', 'percent', 'absolute'
    cacheSize: 1000, // items
    memoryLimitPercent: 25, // % of system memory
    memoryLimitMB: 512, // MB

    // Logging
    logLevel: 'info',
    logFormat: 'text', // 'text' or 'json'
    enableJsonLogging: false,
  });
  const [saved, setSaved] = useState(false);
  const [expandedSource, setExpandedSource] = useState<string | null>(null);

  // Load configuration on mount
  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    try {
      const config = await api.getConfig();
      // Map all backend config fields to frontend settings format
      setSettings({
        // Library settings
        libraryPath: config.root_dir || '/path/to/audiobooks/library',
        organizationStrategy: config.organization_strategy || 'auto',
        scanOnStartup: config.scan_on_startup ?? false,
        autoOrganize: config.auto_organize ?? true,
        folderNamingPattern: config.folder_naming_pattern || '{author}/{series}/{title} ({print_year})',
        fileNamingPattern: config.file_naming_pattern || '{title} - {author} - read by {narrator}',
        createBackups: config.create_backups ?? true,

        // Storage quotas
        enableDiskQuota: config.enable_disk_quota ?? false,
        diskQuotaPercent: config.disk_quota_percent || 80,
        enableUserQuotas: config.enable_user_quotas ?? false,
        defaultUserQuotaGB: config.default_user_quota_gb || 100,

        // Metadata settings
        autoFetchMetadata: config.auto_fetch_metadata ?? true,
        metadataSources: (config.metadata_sources && config.metadata_sources.length > 0) ? config.metadata_sources.map(source => ({
          id: source.id,
          name: source.name,
          enabled: source.enabled,
          priority: source.priority,
          requiresAuth: source.requires_auth,
          credentials: source.credentials || {} as { [key: string]: string }
        })) : [
          {
            id: 'audible',
            name: 'Audible',
            enabled: true,
            priority: 1,
            requiresAuth: false,
            credentials: {}
          },
          {
            id: 'goodreads',
            name: 'Goodreads',
            enabled: true,
            priority: 2,
            requiresAuth: true,
            credentials: { apiKey: '', apiSecret: '' }
          },
          {
            id: 'openlibrary',
            name: 'Open Library',
            enabled: false,
            priority: 3,
            requiresAuth: true,
            credentials: { apiKey: '' }
          },
          {
            id: 'google-books',
            name: 'Google Books',
            enabled: false,
            priority: 4,
            requiresAuth: true,
            credentials: { apiKey: '' }
          },
        ],
        language: config.language || 'en',

        // Performance settings
        concurrentScans: config.concurrent_scans || 4,

        // Memory management
        memoryLimitType: config.memory_limit_type || 'items',
        cacheSize: config.cache_size || 1000,
        memoryLimitPercent: config.memory_limit_percent || 25,
        memoryLimitMB: config.memory_limit_mb || 512,

        // Logging
        logLevel: config.log_level || 'info',
        logFormat: config.log_format || 'text',
        enableJsonLogging: config.enable_json_logging ?? false,
      });
    } catch (error) {
      console.error('Failed to load config:', error);
    }
  };

  // Example data for "To Kill a Mockingbird" audiobook (no series)
  const exampleNoSeries = {
    title: 'To Kill a Mockingbird',
    author: 'Harper Lee',
    narrator: 'Sissy Spacek',
    series: '',
    series_number: '',
    print_year: 1960,
    audiobook_release_year: 2014,
    year: 1960,
    publisher: 'Harper Audio',
    edition: 'Unabridged',
    language: 'English',
    isbn13: '9780061808128',
    isbn10: '0061808121',
    track_number: 3,
    total_tracks: 50,
    bitrate: '320kbps',
    codec: 'AAC',
    quality: '320kbps AAC'
  };

  // Example data for Nancy Drew series book
  const exampleWithSeries = {
    title: 'The Secret of the Old Clock',
    author: 'Carolyn Keene',
    narrator: 'Laura Linney',
    series: 'Nancy Drew Mystery Stories',
    series_number: '1',
    print_year: 1930,
    audiobook_release_year: 2018,
    year: 1930,
    publisher: 'Listening Library',
    edition: 'Unabridged',
    language: 'English',
    isbn13: '9781524780123',
    isbn10: '1524780120',
    track_number: 1,
    total_tracks: 12,
    bitrate: '128kbps',
    codec: 'MP3',
    quality: '128kbps MP3'
  };

  const generateExample = (pattern: string, exampleData: typeof exampleNoSeries, isFolder: boolean = false) => {
    let result = pattern;
    const replacements: Record<string, string> = {
      '{title}': exampleData.title,
      '{author}': exampleData.author,
      '{narrator}': exampleData.narrator,
      '{series}': exampleData.series || '',
      '{series_number}': exampleData.series_number || '',
      '{print_year}': exampleData.print_year.toString(),
      '{audiobook_release_year}': exampleData.audiobook_release_year.toString(),
      '{year}': exampleData.year.toString(),
      '{publisher}': exampleData.publisher,
      '{edition}': exampleData.edition,
      '{language}': exampleData.language,
      '{isbn13}': exampleData.isbn13,
      '{isbn10}': exampleData.isbn10,
      '{track_number}': exampleData.track_number.toString().padStart(2, '0'),
      '{total_tracks}': exampleData.total_tracks.toString(),
      '{bitrate}': exampleData.bitrate || '',
      '{codec}': exampleData.codec || '',
      '{quality}': exampleData.quality || '',
    };

    Object.entries(replacements).forEach(([key, value]) => {
      result = result.replace(new RegExp(key.replace(/[{}]/g, '\\$&'), 'g'), value);
    });

    // Clean up paths: remove empty segments (e.g., when series is empty)
    if (isFolder) {
      result = result.split('/').filter(segment => segment.trim() !== '').join('/');
      return result + '/';
    }

    return result + '.m4b';
  };

  const handleChange = (field: string, value: string | boolean | number) => {
    setSettings((prev) => ({ ...prev, [field]: value }));
    setSaved(false);
  };

  const handleSourceToggle = (sourceId: string) => {
    setSettings((prev) => ({
      ...prev,
      metadataSources: prev.metadataSources.map((source) =>
        source.id === sourceId ? { ...source, enabled: !source.enabled } : source
      ),
    }));
    setSaved(false);
  };

  const handleCredentialChange = (sourceId: string, field: string, value: string) => {
    setSettings((prev) => ({
      ...prev,
      metadataSources: prev.metadataSources.map((source) =>
        source.id === sourceId
          ? { ...source, credentials: { ...source.credentials, [field]: value } as any }
          : source
      ) as any,
    }));
    setSaved(false);
  };

  const handleSourceReorder = (sourceId: string, direction: 'up' | 'down') => {
    setSettings((prev) => {
      const sources = [...prev.metadataSources];
      const index = sources.findIndex((s) => s.id === sourceId);
      if (index === -1) return prev;

      const targetIndex = direction === 'up' ? index - 1 : index + 1;
      if (targetIndex < 0 || targetIndex >= sources.length) return prev;

      // Swap priorities
      const temp = sources[index].priority;
      sources[index] = { ...sources[index], priority: sources[targetIndex].priority };
      sources[targetIndex] = { ...sources[targetIndex], priority: temp };

      // Sort by priority
      sources.sort((a, b) => a.priority - b.priority);

      return { ...prev, metadataSources: sources };
    });
    setSaved(false);
  };

  const handleSave = async () => {
    try {
      // Map all frontend settings to backend config format
      const updates: Partial<api.Config> = {
        // Core paths
        root_dir: settings.libraryPath,
        playlist_dir: settings.libraryPath + '/playlists',
        
        // Library organization
        organization_strategy: settings.organizationStrategy,
        scan_on_startup: settings.scanOnStartup,
        auto_organize: settings.autoOrganize,
        folder_naming_pattern: settings.folderNamingPattern,
        file_naming_pattern: settings.fileNamingPattern,
        create_backups: settings.createBackups,
        
        // Storage quotas
        enable_disk_quota: settings.enableDiskQuota,
        disk_quota_percent: settings.diskQuotaPercent,
        enable_user_quotas: settings.enableUserQuotas,
        default_user_quota_gb: settings.defaultUserQuotaGB,
        
        // Metadata
        auto_fetch_metadata: settings.autoFetchMetadata,
        metadata_sources: settings.metadataSources.map(source => ({
          id: source.id,
          name: source.name,
          enabled: source.enabled,
          priority: source.priority,
          requires_auth: source.requiresAuth,
          credentials: source.credentials as { [key: string]: string }
        })),
        language: settings.language,
        
        // Performance
        concurrent_scans: settings.concurrentScans,
        
        // Memory management
        memory_limit_type: settings.memoryLimitType,
        cache_size: settings.cacheSize,
        memory_limit_percent: settings.memoryLimitPercent,
        memory_limit_mb: settings.memoryLimitMB,
        
        // Logging
        log_level: settings.logLevel,
        log_format: settings.logFormat,
        enable_json_logging: settings.enableJsonLogging,
      };

      await api.updateConfig(updates);
      console.log('Saved settings:', settings);
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    } catch (error) {
      console.error('Failed to save settings:', error);
      alert('Failed to save settings. Please try again.');
    }
  };

  const handleReset = () => {
    if (!confirm('Reset all settings to defaults?')) return;

    setSettings({
      libraryPath: '/path/to/audiobooks/library',
      organizationStrategy: 'auto',
      scanOnStartup: false,
      autoOrganize: true,
      folderNamingPattern: '{author}/{series}/{title} ({print_year})',
      fileNamingPattern: '{title} - {author} - read by {narrator}',
      createBackups: true,
      enableDiskQuota: false,
      diskQuotaPercent: 80,
      enableUserQuotas: false,
      defaultUserQuotaGB: 100,
      autoFetchMetadata: true,
      metadataSources: [
        { id: 'audible', name: 'Audible', enabled: true, priority: 1, requiresAuth: false, credentials: {} },
        { id: 'goodreads', name: 'Goodreads', enabled: true, priority: 2, requiresAuth: true, credentials: { apiKey: '', apiSecret: '' } },
        { id: 'openlibrary', name: 'Open Library', enabled: false, priority: 3, requiresAuth: true, credentials: { apiKey: '' } },
        { id: 'google-books', name: 'Google Books', enabled: false, priority: 4, requiresAuth: true, credentials: { apiKey: '' } },
      ],
      language: 'en',
      concurrentScans: 4,
      memoryLimitType: 'items',
      cacheSize: 1000,
      memoryLimitPercent: 25,
      memoryLimitMB: 512,
      logLevel: 'info',
      logFormat: 'text',
      enableJsonLogging: false,
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

      <Paper sx={{ maxHeight: 'calc(100vh - 200px)', display: 'flex', flexDirection: 'column' }}>
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
              <TextField
                fullWidth
                label="Library Path"
                value={settings.libraryPath}
                onChange={(e) => handleChange('libraryPath', e.target.value)}
                helperText="Main library directory where organized audiobooks are stored. Import paths are configured in File Manager."
              />
              <Alert severity="info" sx={{ mt: 1 }}>
                <Typography variant="caption">
                  <strong>Library vs Import Paths:</strong> The library path is where organized audiobooks live.
                  Import paths (configured in File Manager) are watched for new files to import into the library.
                </Typography>
              </Alert>
            </Grid>

            <Grid item xs={12}>
              <TextField
                fullWidth
                select
                label="File Organization Strategy"
                value={settings.organizationStrategy}
                onChange={(e) => handleChange('organizationStrategy', e.target.value)}
                helperText="How files are organized into the library"
              >
                <MenuItem value="auto">Auto (tries Reflink → Hard Link → Copy)</MenuItem>
                <MenuItem value="reflink">Reflink (CoW - fastest, space-efficient)</MenuItem>
                <MenuItem value="hardlink">Hard Link (fast, space-efficient)</MenuItem>
                <MenuItem value="symlink">Symbolic Link (fast, but fragile)</MenuItem>
                <MenuItem value="copy">Copy (slow, uses double space, safest)</MenuItem>
              </TextField>
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                Auto mode tries methods in order of efficiency: Reflink (CoW clone) → Hard Link → Copy as fallback.
              </Typography>
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
                label="Folder Naming Pattern"
                value={settings.folderNamingPattern}
                onChange={(e) => handleChange('folderNamingPattern', e.target.value)}
                helperText="Available: {title}, {author}, {series}, {series_number}, {print_year}, {audiobook_release_year}, {year}, {publisher}, {edition}, {narrator}, {language}, {isbn10}, {isbn13}, {track_number}, {total_tracks}."
              />
              <Alert severity="info" sx={{ mt: 1, mb: 1 }}>
                <Typography variant="caption">
                  <strong>Smart Path Handling:</strong> Empty fields (like {'{series}'}) are automatically removed from paths.
                  If a book has no series, that segment disappears gracefully—no duplicate slashes or empty folders.
                </Typography>
              </Alert>
              <Box sx={{ mt: 1, p: 2, bgcolor: 'action.hover', border: 1, borderColor: 'divider', borderRadius: 1 }}>
                <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word', display: 'block', fontWeight: 'bold', mb: 0.5 }}>
                  With Series:
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word', display: 'block', mb: 1 }}>
                  {generateExample(settings.folderNamingPattern, exampleWithSeries, true)}
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word', display: 'block', fontWeight: 'bold', mb: 0.5 }}>
                  Without Series:
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word', display: 'block' }}>
                  {generateExample(settings.folderNamingPattern, exampleNoSeries, true)}
                </Typography>
              </Box>
            </Grid>

            <Grid item xs={12}>
              <TextField
                fullWidth
                label="File Naming Pattern"
                value={settings.fileNamingPattern}
                onChange={(e) => handleChange('fileNamingPattern', e.target.value)}
                helperText="Pattern for individual audiobook files. All folder fields plus {track_number}, {total_tracks}, {bitrate}, {codec}, {quality} (parsed from media)"
              />
              <Box sx={{ mt: 1, p: 2, bgcolor: 'action.hover', border: 1, borderColor: 'divider', borderRadius: 1 }}>
                <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word', display: 'block', fontWeight: 'bold', mb: 0.5 }}>
                  With Series (single file):
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word', display: 'block', mb: 1 }}>
                  {generateExample(settings.fileNamingPattern, exampleWithSeries, false)}
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word', display: 'block', fontWeight: 'bold', mb: 0.5 }}>
                  Without Series (multi-file):
                </Typography>
                <Typography variant="caption" color="text.secondary" display="block" sx={{ wordBreak: 'break-word' }}>
                  {generateExample(settings.fileNamingPattern, exampleNoSeries, false).replace('.m4b', ' 03 of 50.mp3')}
                </Typography>
              </Box>
              <Alert severity="info" sx={{ mt: 1 }}>
                <Typography variant="caption">
                  <strong>Multi-file audiobooks:</strong> For books with multiple files (e.g., 50 MP3s), the system automatically appends track numbers.
                  Pattern detection: Uses hyphens if pattern contains "-", underscores if "_", otherwise spaces.
                  Example: "Title - Narrator-03-of-50.mp3" or "Title Narrator 03 of 50.mp3"<br/>
                  <strong>Override:</strong> Include {'{track_number}'} and {'{total_tracks}'} in your pattern to control exact formatting.
                  Example: "{'{title}'} - Part {'{track_number}'} of {'{total_tracks}'}" → "To Kill a Mockingbird - Part 03 of 50.m4b"
                </Typography>
              </Alert>
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

            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom sx={{ mt: 2 }}>
                Storage Quotas
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.enableDiskQuota}
                    onChange={(e) => handleChange('enableDiskQuota', e.target.checked)}
                  />
                }
                label="Enable disk quota limit"
              />
            </Grid>

            {settings.enableDiskQuota && (
              <Grid item xs={12} sm={6}>
                <TextField
                  fullWidth
                  type="number"
                  label="Maximum Disk Usage"
                  value={settings.diskQuotaPercent}
                  onChange={(e) => handleChange('diskQuotaPercent', parseInt(e.target.value) || 0)}
                  InputProps={{
                    endAdornment: <InputAdornment position="end">%</InputAdornment>,
                  }}
                  inputProps={{ min: 1, max: 100 }}
                  helperText="Maximum percentage of disk space the library can use"
                />
              </Grid>
            )}

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.enableUserQuotas}
                    onChange={(e) => handleChange('enableUserQuotas', e.target.checked)}
                  />
                }
                label="Enable per-user storage quotas (multi-user mode)"
              />
            </Grid>

            {settings.enableUserQuotas && (
              <Grid item xs={12} sm={6}>
                <TextField
                  fullWidth
                  type="number"
                  label="Default User Quota"
                  value={settings.defaultUserQuotaGB}
                  onChange={(e) => handleChange('defaultUserQuotaGB', parseInt(e.target.value) || 0)}
                  InputProps={{
                    endAdornment: <InputAdornment position="end">GB</InputAdornment>,
                  }}
                  inputProps={{ min: 1, max: 10000 }}
                  helperText="Storage limit per user"
                />
              </Grid>
            )}
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

            <Grid item xs={12}>
              <Typography variant="subtitle1" gutterBottom>
                Metadata Sources (Priority Order)
              </Typography>
              <Alert severity="info" sx={{ mb: 2 }}>
                <Typography variant="caption">
                  Sources are queried in order. If a field is missing from the first source,
                  the system automatically falls back to the next enabled source to fill in additional fields.
                </Typography>
              </Alert>
              <Paper variant="outlined" sx={{ p: 2 }}>
                {settings.metadataSources.map((source, index) => (
                  <Box key={source.id}>
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        py: 1.5,
                        px: 1,
                        bgcolor: source.enabled ? 'transparent' : 'action.disabledBackground',
                      }}
                    >
                      <Box sx={{ display: 'flex', flexDirection: 'column', mr: 1 }}>
                        <Button
                          size="small"
                          onClick={() => handleSourceReorder(source.id, 'up')}
                          disabled={index === 0}
                          sx={{ minWidth: 'auto', p: 0.5 }}
                        >
                          ▲
                        </Button>
                        <Button
                          size="small"
                          onClick={() => handleSourceReorder(source.id, 'down')}
                          disabled={index === settings.metadataSources.length - 1}
                          sx={{ minWidth: 'auto', p: 0.5 }}
                        >
                          ▼
                        </Button>
                      </Box>
                      <DragHandleIcon sx={{ mr: 2, color: 'text.disabled' }} />
                      <Box sx={{ display: 'flex', alignItems: 'center', flex: 1 }}>
                        <Typography
                          variant="body1"
                          sx={{
                            fontWeight: source.enabled ? 500 : 400,
                            color: source.enabled ? 'text.primary' : 'text.disabled',
                          }}
                        >
                          {source.priority}. {source.name}
                        </Typography>
                        {source.requiresAuth && (
                          <Typography
                            variant="caption"
                            sx={{ ml: 1, color: 'text.secondary' }}
                          >
                            (requires API key)
                          </Typography>
                        )}
                      </Box>
                      {source.requiresAuth && (
                        <IconButton
                          size="small"
                          onClick={() => setExpandedSource(expandedSource === source.id ? null : source.id)}
                          sx={{ mr: 1 }}
                        >
                          <ExpandMoreIcon
                            sx={{
                              transform: expandedSource === source.id ? 'rotate(180deg)' : 'rotate(0deg)',
                              transition: 'transform 0.3s',
                            }}
                          />
                        </IconButton>
                      )}
                      <Button
                        size="small"
                        onClick={() => handleSourceToggle(source.id)}
                        startIcon={
                          source.enabled ? <CheckBoxIcon /> : <CheckBoxOutlineBlankIcon />
                        }
                      >
                        {source.enabled ? 'Enabled' : 'Disabled'}
                      </Button>
                    </Box>
                    {source.requiresAuth && (
                      <Collapse in={expandedSource === source.id}>
                        <Box sx={{ p: 2, bgcolor: 'background.default', borderTop: 1, borderColor: 'divider' }}>
                          <Typography variant="subtitle2" gutterBottom sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
                            <SettingsIcon sx={{ mr: 1, fontSize: 18 }} />
                            API Configuration
                          </Typography>
                          <Grid container spacing={2}>
                            {source.id === 'goodreads' && (
                              <>
                                <Grid item xs={12} sm={6}>
                                  <TextField
                                    fullWidth
                                    size="small"
                                    label="API Key"
                                    value={source.credentials.apiKey || ''}
                                    onChange={(e) => handleCredentialChange(source.id, 'apiKey', e.target.value)}
                                    placeholder="Enter your Goodreads API key"
                                  />
                                </Grid>
                                <Grid item xs={12} sm={6}>
                                  <TextField
                                    fullWidth
                                    size="small"
                                    label="API Secret"
                                    type="password"
                                    value={source.credentials.apiSecret || ''}
                                    onChange={(e) => handleCredentialChange(source.id, 'apiSecret', e.target.value)}
                                    placeholder="Enter your Goodreads API secret"
                                  />
                                </Grid>
                                <Grid item xs={12}>
                                  <Typography variant="caption" color="text.secondary">
                                    Get your API credentials at: <a href="https://www.goodreads.com/api" target="_blank" rel="noopener noreferrer">goodreads.com/api</a>
                                  </Typography>
                                </Grid>
                              </>
                            )}
                            {(source.id === 'openlibrary' || source.id === 'google-books') && (
                              <>
                                <Grid item xs={12}>
                                  <TextField
                                    fullWidth
                                    size="small"
                                    label="API Key"
                                    value={source.credentials.apiKey || ''}
                                    onChange={(e) => handleCredentialChange(source.id, 'apiKey', e.target.value)}
                                    placeholder={`Enter your ${source.name} API key`}
                                  />
                                </Grid>
                                <Grid item xs={12}>
                                  <Typography variant="caption" color="text.secondary">
                                    {source.id === 'google-books' ? (
                                      <>Get your API key at: <a href="https://console.cloud.google.com/apis/credentials" target="_blank" rel="noopener noreferrer">Google Cloud Console</a></>
                                    ) : (
                                      <>Open Library API is free and doesn't require authentication for basic usage</>
                                    )}
                                  </Typography>
                                </Grid>
                              </>
                            )}
                          </Grid>
                        </Box>
                      </Collapse>
                    )}
                    {index < settings.metadataSources.length - 1 && (
                      <Divider />
                    )}
                  </Box>
                ))}
              </Paper>
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

            <Grid item xs={12}>
              <Typography variant="subtitle1" gutterBottom>
                Memory Management
              </Typography>
            </Grid>

            <Grid item xs={12}>
              <FormControl component="fieldset">
                <FormLabel component="legend">Memory Limit Type</FormLabel>
                <RadioGroup
                  row
                  value={settings.memoryLimitType}
                  onChange={(e) => handleChange('memoryLimitType', e.target.value)}
                >
                  <FormControlLabel value="items" control={<Radio />} label="Number of Items" />
                  <FormControlLabel value="percent" control={<Radio />} label="% of System Memory" />
                  <FormControlLabel value="absolute" control={<Radio />} label="Absolute MB" />
                </RadioGroup>
              </FormControl>
            </Grid>

            {settings.memoryLimitType === 'items' && (
              <Grid item xs={12} sm={6}>
                <TextField
                  fullWidth
                  type="number"
                  label="Cache Size (items)"
                  value={settings.cacheSize}
                  onChange={(e) => handleChange('cacheSize', parseInt(e.target.value) || 100)}
                  inputProps={{ min: 100, max: 10000 }}
                  helperText="Number of audiobook records to cache in memory"
                />
              </Grid>
            )}

            {settings.memoryLimitType === 'percent' && (
              <Grid item xs={12} sm={6}>
                <TextField
                  fullWidth
                  type="number"
                  label="Memory Limit"
                  value={settings.memoryLimitPercent}
                  onChange={(e) => handleChange('memoryLimitPercent', parseInt(e.target.value) || 1)}
                  InputProps={{
                    endAdornment: <InputAdornment position="end">%</InputAdornment>,
                  }}
                  inputProps={{ min: 1, max: 90 }}
                  helperText="Maximum percentage of system memory to use"
                />
              </Grid>
            )}

            {settings.memoryLimitType === 'absolute' && (
              <Grid item xs={12} sm={6}>
                <TextField
                  fullWidth
                  type="number"
                  label="Memory Limit"
                  value={settings.memoryLimitMB}
                  onChange={(e) => handleChange('memoryLimitMB', parseInt(e.target.value) || 128)}
                  InputProps={{
                    endAdornment: <InputAdornment position="end">MB</InputAdornment>,
                  }}
                  inputProps={{ min: 128, max: 16384 }}
                  helperText="Absolute memory limit in megabytes"
                />
              </Grid>
            )}

            <Grid item xs={12}>
              <Divider sx={{ my: 2 }} />
              <Typography variant="subtitle1" gutterBottom>
                Logging
              </Typography>
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                select
                label="Log Level"
                value={settings.logLevel}
                onChange={(e) => handleChange('logLevel', e.target.value)}
                helperText="Logging verbosity level"
              >
                <MenuItem value="debug">Debug</MenuItem>
                <MenuItem value="info">Info</MenuItem>
                <MenuItem value="warn">Warning</MenuItem>
                <MenuItem value="error">Error</MenuItem>
              </TextField>
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                select
                label="Log Format"
                value={settings.logFormat}
                onChange={(e) => handleChange('logFormat', e.target.value)}
              >
                <MenuItem value="text">Text (human-readable)</MenuItem>
                <MenuItem value="json">JSON (structured)</MenuItem>
              </TextField>
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                JSON logging is recommended for log aggregation and analysis tools
              </Typography>
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.enableJsonLogging}
                    onChange={(e) => handleChange('enableJsonLogging', e.target.checked)}
                  />
                }
                label="Enable JSON structured logging to separate file"
              />
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', ml: 4 }}>
                Creates a separate .json log file in addition to the main log
              </Typography>
            </Grid>
          </Grid>
        </TabPanel>

        <Box sx={{ p: 2, display: 'flex', gap: 2, justifyContent: 'flex-end', borderTop: 1, borderColor: 'divider', bgcolor: 'background.paper' }}>
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
