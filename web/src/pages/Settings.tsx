// file: web/src/pages/Settings.tsx
// version: 1.29.0
// guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d

import { useState, useEffect, useMemo, useRef, ChangeEvent } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useUnsavedChangesBlocker } from '../hooks/useUnsavedChangesBlocker';
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
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  ListItemSecondaryAction,
  Chip,
  CircularProgress,
  Stack,
} from '@mui/material';
import * as api from '../services/api';
import { ServerFileBrowser } from '../components/common/ServerFileBrowser';
import BlockedHashesTab from '../components/settings/BlockedHashesTab';
import { ITunesImport } from '../components/settings/ITunesImport';
import { OpenLibraryDumps } from '../components/settings/OpenLibraryDumps';
import { SystemInfoTab } from '../components/system/SystemInfoTab';
import {
  Save as SaveIcon,
  RestartAlt as RestartAltIcon,
  DragHandle as DragHandleIcon,
  CheckBox as CheckBoxIcon,
  CheckBoxOutlineBlank as CheckBoxOutlineBlankIcon,
  ExpandMore as ExpandMoreIcon,
  Settings as SettingsIcon,
  FolderOpen as FolderOpenIcon,
  Folder as FolderIcon,
  Add as AddIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

interface ScanStatus {
  status: 'scanning' | 'complete' | 'error' | 'cancelled';
  scanned: number;
  total: number;
  operationId?: string;
  errors?: string[];
}

interface ScanErrorTarget {
  path: string;
  errors: string[];
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
        p: 3,
      }}
      {...other}
    >
      {value === index && children}
    </Box>
  );
}

const TAB_KEYS = ['library', 'itunes', 'metadata', 'performance', 'security', 'system'] as const;

function tabFromHash(hash: string): number {
  const key = hash.replace('#', '');
  const idx = TAB_KEYS.indexOf(key as (typeof TAB_KEYS)[number]);
  return idx >= 0 ? idx : 0;
}

export function Settings() {
  const navigate = useNavigate();
  const location = useLocation();
  const [tabValue, setTabValue] = useState(() => tabFromHash(location.hash));
  const [browserOpen, setBrowserOpen] = useState(false);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const scanIntervalsRef = useRef<Record<number, number>>({});

  // Import folder management
  const [importPaths, setImportFolders] = useState<api.ImportPath[]>([]);
  const [addFolderDialogOpen, setAddFolderDialogOpen] = useState(false);
  const [newFolderPath, setNewFolderPath] = useState('');
  const [showFolderBrowser, setShowFolderBrowser] = useState(false);
  const [scanStatuses, setScanStatuses] = useState<
    Record<number, ScanStatus>
  >({});
  const [cancelScanTarget, setCancelScanTarget] =
    useState<api.ImportPath | null>(null);
  const [scanErrorTarget, setScanErrorTarget] =
    useState<ScanErrorTarget | null>(null);
  const [backups, setBackups] = useState<api.BackupInfo[]>([]);
  const [backupsLoading, setBackupsLoading] = useState(false);
  const [backupNotice, setBackupNotice] = useState<{
    severity: 'success' | 'error' | 'info';
    message: string;
  } | null>(null);
  const [restoreDialogOpen, setRestoreDialogOpen] = useState(false);
  const [restoreTarget, setRestoreTarget] = useState<api.BackupInfo | null>(
    null
  );
  const [restoreInProgress, setRestoreInProgress] = useState(false);
  const [restoreVerify, setRestoreVerify] = useState(true);
  const [deleteBackupTarget, setDeleteBackupTarget] =
    useState<api.BackupInfo | null>(null);
  const [deleteBackupInProgress, setDeleteBackupInProgress] = useState(false);
  const [createBackupInProgress, setCreateBackupInProgress] = useState(false);
  const [openaiTestState, setOpenaiTestState] = useState<{
    status: 'idle' | 'loading' | 'success' | 'error';
    message?: string;
    model?: string;
  }>({ status: 'idle' });
  const [libraryPathError, setLibraryPathError] = useState<string | null>(null);
  const [openaiKeyError, setOpenaiKeyError] = useState<string | null>(null);
  const [extensionsInput, setExtensionsInput] = useState('');
  const [excludePatternInput, setExcludePatternInput] = useState('');
  const [extensionsError, setExtensionsError] = useState<string | null>(null);
  const [excludePatternError, setExcludePatternError] =
    useState<string | null>(null);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [importPayload, setImportPayload] =
    useState<Partial<api.Config> | null>(null);
  const [importFileName, setImportFileName] = useState('');
  const [importNotice, setImportNotice] = useState<string | null>(null);
  const [exportInProgress, setExportInProgress] = useState(false);
  const [importInProgress, setImportInProgress] = useState(false);
  const [savedSnapshot, setSavedSnapshot] = useState('');
  const importInputRef = useRef<HTMLInputElement | null>(null);

  // Factory reset state
  const [factoryResetStep, setFactoryResetStep] = useState<0 | 1 | 2>(0);
  const [factoryResetConfirmText, setFactoryResetConfirmText] = useState('');
  const [factoryResetInProgress, setFactoryResetInProgress] = useState(false);

  interface UiMetadataSource {
    id: string;
    name: string;
    enabled: boolean;
    priority: number;
    requiresAuth: boolean;
    credentials: { [key: string]: string };
  }

  interface SettingsState {
    libraryPath: string;
    organizationStrategy: string;
    scanOnStartup: boolean;
    autoOrganize: boolean;
    folderNamingPattern: string;
    fileNamingPattern: string;
    createBackups: boolean;
    supportedExtensions: string[];
    excludePatterns: string[];
    enableDiskQuota: boolean;
    diskQuotaPercent: number;
    enableUserQuotas: boolean;
    defaultUserQuotaGB: number;
    autoFetchMetadata: boolean;
    enableAIParsing: boolean;
    openaiApiKey: string;
    metadataSources: UiMetadataSource[];
    language: string;
    concurrentScans: number;
    memoryLimitType: string;
    cacheSize: number;
    memoryLimitPercent: number;
    memoryLimitMB: number;
    logLevel: string;
    logFormat: string;
    enableJsonLogging: boolean;
    purgeSoftDeletedAfterDays: number;
    purgeSoftDeletedDeleteFiles: boolean;
  }

  const initialSettings: SettingsState = {
    // Library settings
    libraryPath: '/path/to/audiobooks/library',
    // 'auto', 'copy', 'hardlink', 'reflink', 'symlink'
    organizationStrategy: 'auto',
    scanOnStartup: false,
    autoOrganize: true,
    folderNamingPattern: '{author}/{series}/{title} ({print_year})',
    fileNamingPattern: '{title} - {author} - read by {narrator}',
    createBackups: true,
    supportedExtensions: ['.m4b', '.mp3', '.m4a'],
    excludePatterns: [],

    // Storage quotas
    enableDiskQuota: false,
    diskQuotaPercent: 80,
    enableUserQuotas: false,
    defaultUserQuotaGB: 100,

    // Metadata settings
    autoFetchMetadata: true,
    enableAIParsing: false,
    openaiApiKey: '',
    metadataSources: [
      {
        id: 'audible',
        name: 'Audible',
        enabled: true,
        priority: 1,
        requiresAuth: false,
        credentials: {},
      },
      {
        id: 'openlibrary',
        name: 'Open Library',
        enabled: true,
        priority: 2,
        requiresAuth: false,
        credentials: {},
      },
      {
        id: 'google-books',
        name: 'Google Books',
        enabled: false,
        priority: 3,
        requiresAuth: true,
        credentials: { apiKey: '' },
      },
    ],
    language: 'en',

    // Performance settings
    concurrentScans: 4,

    // Memory management
    // 'items', 'percent', 'absolute'
    memoryLimitType: 'items',
    cacheSize: 1000, // items
    memoryLimitPercent: 25, // % of system memory
    memoryLimitMB: 512, // MB

    // Lifecycle / retention
    purgeSoftDeletedAfterDays: 30,
    purgeSoftDeletedDeleteFiles: false,

    // Logging
    logLevel: 'info',
    logFormat: 'text', // 'text' or 'json'
    enableJsonLogging: false,
  };

  const [settings, setSettings] = useState<SettingsState>(initialSettings);
  const [saved, setSaved] = useState(false);
  const [expandedSource, setExpandedSource] = useState<string | null>(null);
  const [savedApiKeyMask, setSavedApiKeyMask] = useState<string>('');
  const settingsSnapshot = useMemo(
    () => JSON.stringify(settings),
    [settings]
  );
  const hasUnsavedChanges =
    savedSnapshot !== '' && settingsSnapshot !== savedSnapshot;
  const blocker = useUnsavedChangesBlocker(hasUnsavedChanges);
  const savedSettings = useMemo(() => {
    if (!savedSnapshot) return null;
    try {
      return JSON.parse(savedSnapshot) as SettingsState;
    } catch {
      return null;
    }
  }, [savedSnapshot]);
  const importKeys = useMemo(() => {
    if (!importPayload) return [];
    return Object.keys(importPayload);
  }, [importPayload]);
  const openaiPlaceholder = savedApiKeyMask
    ? `Key saved: ${savedApiKeyMask} (enter new key to change)`
    : 'sk-...';
  const openaiHelperText =
    openaiKeyError ||
    (settings.enableAIParsing
      ? savedApiKeyMask
        ? 'Key is currently set. Enter a new key to update it.'
        : 'Get your API key from ' +
          'https://platform.openai.com/api-keys'
      : 'Enable AI parsing to configure API key');

  // Load configuration on mount
  useEffect(() => {
    loadConfig();
    loadImportFolders();
    loadBackups();
  }, []);

  useEffect(() => {
    if (!hasUnsavedChanges) return;
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [hasUnsavedChanges]);

  const loadConfig = async () => {
    try {
      const config = await api.getConfig();
      console.log(
        '[Settings] Loaded config, OpenAI key:',
        config.openai_api_key
      );
      // Store masked key if present
      if (config.openai_api_key && config.openai_api_key.includes('***')) {
        setSavedApiKeyMask(config.openai_api_key);
      }
      // Map all backend config fields to frontend settings format
      const nextSettings: SettingsState = {
        // Library settings
        libraryPath: config.root_dir || '',
        organizationStrategy: config.organization_strategy || 'auto',
        scanOnStartup: config.scan_on_startup ?? false,
        autoOrganize: config.auto_organize ?? true,
        folderNamingPattern:
          config.folder_naming_pattern ||
          '{author}/{series}/{title} ({print_year})',
        fileNamingPattern:
          config.file_naming_pattern ||
          '{title} - {author} - read by {narrator}',
        createBackups: config.create_backups ?? true,
        supportedExtensions: config.supported_extensions?.length
          ? config.supported_extensions
          : ['.m4b', '.mp3', '.m4a'],
        excludePatterns: config.exclude_patterns || [],

        // Storage quotas
        enableDiskQuota: config.enable_disk_quota ?? false,
        diskQuotaPercent: config.disk_quota_percent || 80,
        enableUserQuotas: config.enable_user_quotas ?? false,
        defaultUserQuotaGB: config.default_user_quota_gb || 100,

        // Metadata settings
        autoFetchMetadata: config.auto_fetch_metadata ?? true,
        enableAIParsing: config.enable_ai_parsing ?? false,
        openaiApiKey: '', // Clear field when loading, show placeholder instead
        metadataSources:
          config.metadata_sources && config.metadata_sources.length > 0
            ? config.metadata_sources.map((source) => ({
                id: source.id,
                name: source.name,
                enabled: source.enabled,
                priority: source.priority,
                requiresAuth: source.requires_auth,
                credentials:
                  source.credentials || ({} as { [key: string]: string }),
              }))
            : [
                {
                  id: 'audible',
                  name: 'Audible',
                  enabled: true,
                  priority: 1,
                  requiresAuth: false,
                  credentials: {},
                },
                {
                  id: 'openlibrary',
                  name: 'Open Library',
                  enabled: true,
                  priority: 2,
                  requiresAuth: false,
                  credentials: {},
                },
                {
                  id: 'google-books',
                  name: 'Google Books',
                  enabled: false,
                  priority: 3,
                  requiresAuth: true,
                  credentials: { apiKey: '' },
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

        // Lifecycle / retention
        purgeSoftDeletedAfterDays: config.purge_soft_deleted_after_days ?? 30,
        purgeSoftDeletedDeleteFiles:
          config.purge_soft_deleted_delete_files ?? false,

        // Logging
        logLevel: config.log_level || 'info',
        logFormat: config.log_format || 'text',
        enableJsonLogging: config.enable_json_logging ?? false,
      };
      setSettings(nextSettings);
      setSavedSnapshot(JSON.stringify(nextSettings));
    } catch (error) {
      if (error instanceof api.ApiError && error.status === 401) {
        navigate('/login');
        return;
      }
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
    quality: '320kbps AAC',
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
    quality: '128kbps MP3',
  };

  const generateExample = (
    pattern: string,
    exampleData: typeof exampleNoSeries,
    isFolder: boolean = false
  ) => {
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
      result = result.replace(
        new RegExp(key.replace(/[{}]/g, '\\$&'), 'g'),
        value
      );
    });

    // Clean up paths: remove empty segments (e.g., when series is empty)
    if (isFolder) {
      result = result
        .split('/')
        .filter((segment) => segment.trim() !== '')
        .join('/');
      return result + '/';
    }

    return result + '.m4b';
  };

  const normalizeExtension = (value: string): string => {
    const trimmed = value.trim();
    if (!trimmed) return '';
    const withDot = trimmed.startsWith('.') ? trimmed : `.${trimmed}`;
    return withDot.toLowerCase();
  };

  const isValidOpenAIKey = (value: string): boolean => {
    if (!value) return true;
    return /^sk-[A-Za-z0-9_-]{8,}$/.test(value);
  };

  const handleChange = (
    field: string,
    value: string | boolean | number | string[]
  ) => {
    setSettings((prev) => ({ ...prev, [field]: value }));
    setSaved(false);
    if (field === 'libraryPath') {
      setLibraryPathError(null);
    }
    if (field === 'openaiApiKey') {
      setOpenaiKeyError(null);
      setOpenaiTestState({ status: 'idle' });
    }
  };

  const handleBrowseLibraryPath = () => {
    setSelectedPath(settings.libraryPath);
    setBrowserOpen(true);
  };

  const handleBrowserSelect = (path: string, isDir: boolean) => {
    if (isDir) {
      setSelectedPath(path);
    }
  };

  const handleBrowserConfirm = () => {
    if (selectedPath) {
      handleChange('libraryPath', selectedPath);
    }
    setBrowserOpen(false);
  };

  const handleBrowserCancel = () => {
    setBrowserOpen(false);
    setSelectedPath(null);
  };

  // Import folder management handlers
  const loadImportFolders = async () => {
    try {
      const folders = await api.getImportPaths();
      setImportFolders(folders);
    } catch (error) {
      console.error('Failed to load import folders:', error);
    }
  };

  const handleAddImportFolder = async () => {
    if (!newFolderPath.trim()) return;

    try {
      const folder = await api.addImportPath(
        newFolderPath,
        newFolderPath.split('/').pop() || 'Import Folder'
      );
      setImportFolders((prev) => [...prev, folder]);
      setNewFolderPath('');
      setShowFolderBrowser(false);
      setAddFolderDialogOpen(false);
    } catch (error) {
      console.error('Failed to add import folder:', error);
    }
  };

  const handleRemoveImportFolder = async (id: number) => {
    try {
      await api.removeImportPath(id);
      setImportFolders((prev) => prev.filter((f) => f.id !== id));
    } catch (error) {
      console.error('Failed to remove import folder:', error);
    }
  };

  const handleScanImportFolder = async (folder: api.ImportPath) => {
    setScanStatuses((prev) => ({
      ...prev,
      [folder.id]: {
        status: 'scanning',
        scanned: 0,
        total: prev[folder.id]?.total || 0,
      },
    }));

    let total = 50;
    let errors: string[] = [];
    let operationId: string | undefined;

    try {
      const response = await api.startScan(folder.path);
      if (typeof response.total === 'number') {
        total = response.total;
      }
      if (Array.isArray(response.errors)) {
        errors = response.errors;
      }
      operationId = response.id;
    } catch (error) {
      console.error('Failed to scan import folder:', error);
      const message =
        error instanceof Error ? error.message : 'Scan failed.';
      setScanStatuses((prev) => ({
        ...prev,
        [folder.id]: {
          status: 'error',
          scanned: 0,
          total: 0,
          errors: [message],
        },
      }));
      return;
    }

    setScanStatuses((prev) => ({
      ...prev,
      [folder.id]: {
        status: 'scanning',
        scanned: 0,
        total,
        operationId,
        errors,
      },
    }));

    let scanned = 0;
    const increment = Math.max(1, Math.ceil(total / 10));
    const interval = window.setInterval(() => {
      scanned += increment;
      setScanStatuses((prev) => ({
        ...prev,
        [folder.id]: {
          status: scanned >= total ? 'complete' : 'scanning',
          scanned: Math.min(scanned, total),
          total,
          operationId,
          errors,
        },
      }));
      if (scanned >= total) {
        window.clearInterval(interval);
        delete scanIntervalsRef.current[folder.id];
      }
    }, 300);

    scanIntervalsRef.current[folder.id] = interval;
  };

  const handleRequestCancelScan = (folder: api.ImportPath) => {
    setCancelScanTarget(folder);
  };

  const handleConfirmCancelScan = async () => {
    if (!cancelScanTarget) return;
    const target = cancelScanTarget;
    setCancelScanTarget(null);
    const status = scanStatuses[target.id];
    if (!status) return;
    const interval = scanIntervalsRef.current[target.id];
    if (interval) {
      window.clearInterval(interval);
      delete scanIntervalsRef.current[target.id];
    }
    if (status.operationId) {
      try {
        await api.cancelOperation(status.operationId);
      } catch (error) {
        console.error('Failed to cancel scan operation:', error);
      }
    }
    setScanStatuses((prev) => ({
      ...prev,
      [target.id]: {
        ...status,
        status: 'cancelled',
      },
    }));
  };

  const handleViewScanErrors = (
    folder: api.ImportPath,
    status: ScanStatus
  ) => {
    if (!status.errors?.length) return;
    setScanErrorTarget({
      path: folder.path,
      errors: status.errors,
    });
  };

  const loadBackups = async () => {
    setBackupsLoading(true);
    try {
      const data = await api.listBackups();
      const sorted = [...(data.backups || [])].sort(
        (a, b) =>
          new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
      );
      setBackups(sorted);
    } catch (error) {
      console.error('Failed to load backups:', error);
      setBackupNotice({
        severity: 'error',
        message: 'Failed to load backups.',
      });
    } finally {
      setBackupsLoading(false);
    }
  };

  const handleCreateBackup = async () => {
    setCreateBackupInProgress(true);
    setBackupNotice(null);
    try {
      await api.createBackup();
      setBackupNotice({
        severity: 'success',
        message: 'Backup created successfully.',
      });
      await loadBackups();
    } catch (error) {
      console.error('Failed to create backup:', error);
      setBackupNotice({
        severity: 'error',
        message: 'Failed to create backup.',
      });
    } finally {
      setCreateBackupInProgress(false);
    }
  };

  const handleRequestRestore = (backup: api.BackupInfo) => {
    setRestoreTarget(backup);
    setRestoreDialogOpen(true);
  };

  const handleConfirmRestore = async () => {
    if (!restoreTarget) return;
    setRestoreInProgress(true);
    setBackupNotice(null);
    try {
      await api.restoreBackup(restoreTarget.filename, restoreVerify);
      setBackupNotice({
        severity: 'success',
        message: 'Backup restored successfully.',
      });
      setRestoreDialogOpen(false);
      window.location.reload();
    } catch (error) {
      console.error('Failed to restore backup:', error);
      setBackupNotice({
        severity: 'error',
        message: 'Backup file is corrupt.',
      });
    } finally {
      setRestoreInProgress(false);
    }
  };

  const handleRequestDeleteBackup = (backup: api.BackupInfo) => {
    setDeleteBackupTarget(backup);
  };

  const handleConfirmDeleteBackup = async () => {
    if (!deleteBackupTarget) return;
    setDeleteBackupInProgress(true);
    setBackupNotice(null);
    try {
      await api.deleteBackup(deleteBackupTarget.filename);
      setBackupNotice({
        severity: 'success',
        message: 'Backup deleted successfully.',
      });
      setDeleteBackupTarget(null);
      await loadBackups();
    } catch (error) {
      console.error('Failed to delete backup:', error);
      setBackupNotice({
        severity: 'error',
        message: 'Failed to delete backup.',
      });
    } finally {
      setDeleteBackupInProgress(false);
    }
  };

  const handleFolderBrowserSelect = (path: string, isDir: boolean) => {
    if (isDir) {
      setNewFolderPath(path);
    }
  };

  const handleSourceToggle = (sourceId: string) => {
    setSettings((prev) => ({
      ...prev,
      metadataSources: prev.metadataSources.map((source) =>
        source.id === sourceId
          ? { ...source, enabled: !source.enabled }
          : source
      ),
    }));
    setSaved(false);
  };

  const handleCredentialChange = (
    sourceId: string,
    field: string,
    value: string
  ) => {
    setSettings((prev) => ({
      ...prev,
      metadataSources: prev.metadataSources.map((source) =>
        source.id === sourceId
          ? {
              ...source,
              credentials: { ...source.credentials, [field]: value },
            }
          : source
      ),
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
      sources[index] = {
        ...sources[index],
        priority: sources[targetIndex].priority,
      };
      sources[targetIndex] = { ...sources[targetIndex], priority: temp };

      // Sort by priority
      sources.sort((a, b) => a.priority - b.priority);

      return { ...prev, metadataSources: sources };
    });
    setSaved(false);
  };

  const handleSave = async (): Promise<boolean> => {
    setLibraryPathError(null);
    setOpenaiKeyError(null);
    setExtensionsError(null);
    setExcludePatternError(null);

    const libraryPath = settings.libraryPath.trim();
    if (!libraryPath) {
      setLibraryPathError('Library path is required.');
      return false;
    }
    if (savedSettings && savedSettings.libraryPath !== libraryPath) {
      try {
        await api.browseFilesystem(libraryPath);
      } catch (error) {
        console.error('Library path validation failed:', error);
        setLibraryPathError('Directory does not exist.');
        return false;
      }
    }
    if (settings.supportedExtensions.length === 0) {
      setExtensionsError('Add at least one extension.');
      return false;
    }
    if (!isValidOpenAIKey(settings.openaiApiKey)) {
      setOpenaiKeyError('Invalid API key format.');
      return false;
    }

    try {
      // Map all frontend settings to backend config format
      const updates: Partial<api.Config> = {
        // Core paths
        root_dir: libraryPath,
        playlist_dir: `${libraryPath}/playlists`,

        // Library organization
        organization_strategy: settings.organizationStrategy,
        scan_on_startup: settings.scanOnStartup,
        auto_organize: settings.autoOrganize,
        folder_naming_pattern: settings.folderNamingPattern,
        file_naming_pattern: settings.fileNamingPattern,
        create_backups: settings.createBackups,
        supported_extensions: settings.supportedExtensions,
        exclude_patterns: settings.excludePatterns,

        // Storage quotas
        enable_disk_quota: settings.enableDiskQuota,
        disk_quota_percent: settings.diskQuotaPercent,
        enable_user_quotas: settings.enableUserQuotas,
        default_user_quota_gb: settings.defaultUserQuotaGB,

        // Metadata
        auto_fetch_metadata: settings.autoFetchMetadata,
        enable_ai_parsing: settings.enableAIParsing,
        // Only include API key if user entered a new one
        ...(settings.openaiApiKey
          ? { openai_api_key: settings.openaiApiKey }
          : {}),
        metadata_sources: settings.metadataSources.map((source) => ({
          id: source.id,
          name: source.name,
          enabled: source.enabled,
          priority: source.priority,
          requires_auth: source.requiresAuth,
          credentials: source.credentials as { [key: string]: string },
        })),
        language: settings.language,

        // Performance
        concurrent_scans: settings.concurrentScans,

        // Memory management
        memory_limit_type: settings.memoryLimitType,
        cache_size: settings.cacheSize,
        memory_limit_percent: settings.memoryLimitPercent,
        memory_limit_mb: settings.memoryLimitMB,

        // Lifecycle / retention
        purge_soft_deleted_after_days: settings.purgeSoftDeletedAfterDays,
        purge_soft_deleted_delete_files: settings.purgeSoftDeletedDeleteFiles,

        // Logging
        log_level: settings.logLevel,
        log_format: settings.logFormat,
        enable_json_logging: settings.enableJsonLogging,
      };

      console.log(
        'Saving config with OpenAI key:',
        settings.openaiApiKey
          ? `***${settings.openaiApiKey.slice(-4)}`
          : '(empty)'
      );
      console.log('Full updates object:', updates);
      const response = await api.updateConfig(updates);
      console.log('Save response:', response);

      let nextSettings = settings;
      if (settings.openaiApiKey && response.openai_api_key) {
        setSavedApiKeyMask(response.openai_api_key);
        nextSettings = { ...settings, openaiApiKey: '' };
        setSettings(nextSettings);
      }

      setSavedSnapshot(JSON.stringify(nextSettings));
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
      return true;
    } catch (error) {
      if (error instanceof api.ApiError && error.status === 401) {
        navigate('/login');
        return false;
      }
      console.error('Failed to save settings:', error);
      alert('Failed to save settings. Please try again.');
      return false;
    }
  };

  const handleReset = () => {
    if (!confirm('Reset all settings to defaults?')) return;

    setSettings(initialSettings);
    setSaved(false);
    setLibraryPathError(null);
    setOpenaiKeyError(null);
    setExtensionsError(null);
    setExcludePatternError(null);
  };

  const handleAddExtension = () => {
    const normalized = normalizeExtension(extensionsInput);
    if (!normalized) {
      setExtensionsError('Enter a file extension.');
      return;
    }
    if (!/^\.[a-z0-9]+$/i.test(normalized)) {
      setExtensionsError('Use letters or numbers, like .m4b');
      return;
    }
    if (settings.supportedExtensions.includes(normalized)) {
      setExtensionsError('Extension already added.');
      return;
    }
    setSettings((prev) => ({
      ...prev,
      supportedExtensions: [...prev.supportedExtensions, normalized].sort(),
    }));
    setExtensionsInput('');
    setExtensionsError(null);
    setSaved(false);
  };

  const handleRemoveExtension = (extension: string) => {
    setSettings((prev) => ({
      ...prev,
      supportedExtensions: prev.supportedExtensions.filter(
        (item) => item !== extension
      ),
    }));
    setExtensionsError(null);
    setSaved(false);
  };

  const handleAddExcludePattern = () => {
    const normalized = excludePatternInput.trim();
    if (!normalized) {
      setExcludePatternError('Enter a pattern to exclude.');
      return;
    }
    if (settings.excludePatterns.includes(normalized)) {
      setExcludePatternError('Pattern already added.');
      return;
    }
    setSettings((prev) => ({
      ...prev,
      excludePatterns: [...prev.excludePatterns, normalized],
    }));
    setExcludePatternInput('');
    setExcludePatternError(null);
    setSaved(false);
  };

  const handleRemoveExcludePattern = (pattern: string) => {
    setSettings((prev) => ({
      ...prev,
      excludePatterns: prev.excludePatterns.filter((item) => item !== pattern),
    }));
    setExcludePatternError(null);
    setSaved(false);
  };

  const handleTestAIConnection = async () => {
    const apiKey = settings.openaiApiKey.trim();
    if (!settings.enableAIParsing) {
      setOpenaiTestState({
        status: 'error',
        message: 'Enable AI parsing to test the connection.',
      });
      return;
    }
    if (apiKey && !isValidOpenAIKey(apiKey)) {
      setOpenaiKeyError('Invalid API key format.');
      return;
    }
    if (!apiKey && !savedApiKeyMask) {
      setOpenaiTestState({
        status: 'error',
        message: 'API key not provided.',
      });
      return;
    }
    setOpenaiTestState({ status: 'loading' });
    try {
      const response = await api.testAIConnection(apiKey || undefined);
      setOpenaiTestState({
        status: 'success',
        message: response.message || 'Connection successful.',
      });
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'Connection failed.';
      setOpenaiTestState({
        status: 'error',
        message,
      });
    }
  };

  const sanitizeImportPayload = (
    payload: Partial<api.Config>
  ): Partial<api.Config> => {
    const cleaned = { ...payload };
    delete cleaned.database_type;
    delete cleaned.enable_sqlite;
    if (
      typeof cleaned.openai_api_key === 'string' &&
      cleaned.openai_api_key.includes('***')
    ) {
      delete cleaned.openai_api_key;
    }
    return cleaned;
  };

  const handleExportSettings = async () => {
    setExportInProgress(true);
    setImportNotice(null);
    try {
      const config = await api.getConfig();
      const blob = new Blob([JSON.stringify(config, null, 2)], {
        type: 'application/json',
      });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `settings-${new Date().toISOString()}.json`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(url);
      setImportNotice('Settings exported.');
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'Export failed.';
      setImportNotice(message);
    } finally {
      setExportInProgress(false);
    }
  };

  const handleImportClick = () => {
    setImportNotice(null);
    if (importInputRef.current) {
      importInputRef.current.value = '';
      importInputRef.current.click();
    }
  };

  const handleImportFileChange = async (
    event: ChangeEvent<HTMLInputElement>
  ) => {
    const file = event.target.files?.[0];
    if (!file) return;
    try {
      const text = await file.text();
      const parsed = JSON.parse(text) as Partial<api.Config>;
      const cleaned = sanitizeImportPayload(parsed);
      setImportFileName(file.name);
      setImportPayload(cleaned);
      setImportDialogOpen(true);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'Import failed.';
      setImportNotice(message);
    }
  };

  const handleConfirmImport = async () => {
    if (!importPayload) return;
    setImportInProgress(true);
    try {
      await api.updateConfig(importPayload);
      await loadConfig();
      setImportDialogOpen(false);
      setImportPayload(null);
      setImportFileName('');
      setImportNotice('Settings imported successfully.');
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'Import failed.';
      setImportNotice(message);
    } finally {
      setImportInProgress(false);
    }
  };

  const handleSaveAndNavigate = async () => {
    const success = await handleSave();
    if (success) {
      blocker.proceed?.();
    }
  };

  const handleDiscardNavigation = () => {
    blocker.proceed?.();
  };

  const handleCancelNavigation = () => {
    blocker.reset?.();
  };

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100vh',
        maxHeight: '100vh',
        overflow: 'hidden',
        p: 2,
      }}
    >
      <Typography variant="h4" gutterBottom sx={{ flexShrink: 0 }}>
        Settings
      </Typography>

      {saved && (
        <Alert severity="success" sx={{ mb: 2, flexShrink: 0 }}>
          Settings saved successfully!
        </Alert>
      )}
      {importNotice && (
        <Alert severity="info" sx={{ mb: 2, flexShrink: 0 }}>
          {importNotice}
        </Alert>
      )}

      <Paper
        sx={{
          display: 'flex',
          flexDirection: 'column',
          flex: 1,
          minHeight: 0,
          overflow: 'hidden',
        }}
      >
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs
            value={tabValue}
            onChange={(_, newValue) => {
              setTabValue(newValue);
              window.history.replaceState(null, '', `#${TAB_KEYS[newValue]}`);
            }}
            aria-label="settings tabs"
          >
            <Tab label="Library" />
            <Tab label="iTunes Import" />
            <Tab label="Metadata" />
            <Tab label="Performance" />
            <Tab label="Security" />
            <Tab label="System Info" />
          </Tabs>
        </Box>

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
                error={Boolean(libraryPathError)}
                helperText={
                  libraryPathError ||
                  'Main library directory where organized audiobooks are ' +
                    'stored. Import paths are configured in File Manager.'
                }
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <Button
                        variant="outlined"
                        size="small"
                        startIcon={<FolderOpenIcon />}
                        onClick={handleBrowseLibraryPath}
                      >
                        Browse Server
                      </Button>
                    </InputAdornment>
                  ),
                }}
              />
              <Alert severity="info" sx={{ mt: 1 }}>
                <Typography variant="caption">
                  <strong>Library vs Import Paths:</strong> The library path is
                  where organized audiobooks live. Import paths (configured in
                  File Manager) are watched for new files to import into the
                  library.
                </Typography>
              </Alert>
            </Grid>

            <Grid item xs={12}>
              <TextField
                fullWidth
                select
                label="File Organization Strategy"
                value={settings.organizationStrategy}
                onChange={(e) =>
                  handleChange('organizationStrategy', e.target.value)
                }
                helperText="How files are organized into the library"
              >
                <MenuItem value="auto">
                  Auto (tries Reflink → Hard Link → Copy)
                </MenuItem>
                <MenuItem value="reflink">
                  Reflink (CoW - fastest, space-efficient)
                </MenuItem>
                <MenuItem value="hardlink">
                  Hard Link (fast, space-efficient)
                </MenuItem>
                <MenuItem value="symlink">
                  Symbolic Link (fast, but fragile)
                </MenuItem>
                <MenuItem value="copy">
                  Copy (slow, uses double space, safest)
                </MenuItem>
              </TextField>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mt: 1, display: 'block' }}
              >
                Auto mode tries methods in order of efficiency: Reflink (CoW
                clone) → Hard Link → Copy as fallback.
              </Typography>
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.scanOnStartup}
                    onChange={(e) =>
                      handleChange('scanOnStartup', e.target.checked)
                    }
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
                    onChange={(e) =>
                      handleChange('autoOrganize', e.target.checked)
                    }
                  />
                }
                label="Automatically organize audiobooks"
              />
            </Grid>

            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom sx={{ mt: 2 }}>
                Scan Settings
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12} md={6}>
              <Typography variant="subtitle2" gutterBottom>
                Supported Extensions
              </Typography>
              <Stack spacing={1}>
                <Stack direction="row" spacing={1}>
                  <TextField
                    fullWidth
                    size="small"
                    label="Add extension"
                    placeholder=".m4b"
                    value={extensionsInput}
                    onChange={(e) => setExtensionsInput(e.target.value)}
                    error={Boolean(extensionsError)}
                  />
                  <Button variant="outlined" onClick={handleAddExtension}>
                    Add
                  </Button>
                </Stack>
                {extensionsError && (
                  <Alert severity="error">{extensionsError}</Alert>
                )}
                <Stack direction="row" spacing={1} flexWrap="wrap">
                  {settings.supportedExtensions.map((extension) => (
                    <Chip
                      key={extension}
                      label={extension}
                      onDelete={() => handleRemoveExtension(extension)}
                    />
                  ))}
                </Stack>
              </Stack>
            </Grid>

            <Grid item xs={12} md={6}>
              <Typography variant="subtitle2" gutterBottom>
                Exclude Patterns
              </Typography>
              <Stack spacing={1}>
                <Stack direction="row" spacing={1}>
                  <TextField
                    fullWidth
                    size="small"
                    label="Add exclude pattern"
                    placeholder="*_preview.m4b"
                    value={excludePatternInput}
                    onChange={(e) => setExcludePatternInput(e.target.value)}
                    error={Boolean(excludePatternError)}
                  />
                  <Button variant="outlined" onClick={handleAddExcludePattern}>
                    Add
                  </Button>
                </Stack>
                {excludePatternError && (
                  <Alert severity="error">{excludePatternError}</Alert>
                )}
                <Stack direction="row" spacing={1} flexWrap="wrap">
                  {settings.excludePatterns.map((pattern) => (
                    <Chip
                      key={pattern}
                      label={pattern}
                      onDelete={() => handleRemoveExcludePattern(pattern)}
                    />
                  ))}
                </Stack>
              </Stack>
            </Grid>

            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Folder Naming Pattern"
                value={settings.folderNamingPattern}
                onChange={(e) =>
                  handleChange('folderNamingPattern', e.target.value)
                }
                helperText={
                  'Available: {title}, {author}, {series}, {series_number}, ' +
                  '{print_year}, {audiobook_release_year}, {year}, ' +
                  '{publisher}, {edition}, {narrator}, {language}, ' +
                  '{isbn10}, {isbn13}, {track_number}, {total_tracks}.'
                }
              />
              <Alert severity="info" sx={{ mt: 1, mb: 1 }}>
                <Typography variant="caption">
                  <strong>Smart Path Handling:</strong> Empty fields (like{' '}
                  {'{series}'}) are automatically removed from paths. If a
                  book has no series, that segment disappears gracefully—no
                  duplicate slashes or empty folders.
                </Typography>
              </Alert>
              <Box
                sx={{
                  mt: 1,
                  p: 2,
                  bgcolor: 'action.hover',
                  border: 1,
                  borderColor: 'divider',
                  borderRadius: 1,
                }}
              >
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    wordBreak: 'break-word',
                    display: 'block',
                    fontWeight: 'bold',
                    mb: 0.5,
                  }}
                >
                  With Series:
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ wordBreak: 'break-word', display: 'block', mb: 1 }}
                >
                  {generateExample(
                    settings.folderNamingPattern,
                    exampleWithSeries,
                    true
                  )}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    wordBreak: 'break-word',
                    display: 'block',
                    fontWeight: 'bold',
                    mb: 0.5,
                  }}
                >
                  Without Series:
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ wordBreak: 'break-word', display: 'block' }}
                >
                  {generateExample(
                    settings.folderNamingPattern,
                    exampleNoSeries,
                    true
                  )}
                </Typography>
              </Box>
            </Grid>

            <Grid item xs={12}>
              <TextField
                fullWidth
                label="File Naming Pattern"
                value={settings.fileNamingPattern}
                onChange={(e) =>
                  handleChange('fileNamingPattern', e.target.value)
                }
                helperText={
                  'Pattern for individual audiobook files. All folder fields ' +
                  'plus {track_number}, {total_tracks}, {bitrate}, {codec}, ' +
                  '{quality} (parsed from media)'
                }
              />
              <Box
                sx={{
                  mt: 1,
                  p: 2,
                  bgcolor: 'action.hover',
                  border: 1,
                  borderColor: 'divider',
                  borderRadius: 1,
                }}
              >
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    wordBreak: 'break-word',
                    display: 'block',
                    fontWeight: 'bold',
                    mb: 0.5,
                  }}
                >
                  With Series (single file):
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ wordBreak: 'break-word', display: 'block', mb: 1 }}
                >
                  {generateExample(
                    settings.fileNamingPattern,
                    exampleWithSeries,
                    false
                  )}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    wordBreak: 'break-word',
                    display: 'block',
                    fontWeight: 'bold',
                    mb: 0.5,
                  }}
                >
                  Without Series (multi-file):
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  display="block"
                  sx={{ wordBreak: 'break-word' }}
                >
                  {generateExample(
                    settings.fileNamingPattern,
                    exampleNoSeries,
                    false
                  ).replace('.m4b', ' 03 of 50.mp3')}
                </Typography>
              </Box>
              <Alert severity="info" sx={{ mt: 1 }}>
                <Typography variant="caption">
                  <strong>Multi-file audiobooks:</strong> For books with
                  multiple files (e.g., 50 MP3s), the system automatically
                  appends track numbers. Pattern detection: Uses hyphens if
                  pattern contains "-", underscores if "_", otherwise spaces.
                  Example: "Title - Narrator-03-of-50.mp3" or "Title Narrator
                  03 of 50.mp3"
                  <br />
                  <strong>Override:</strong> Include {'{track_number}'} and{' '}
                  {'{total_tracks}'} in your pattern to control exact
                  formatting. Example: "{'{title}'} - Part{' '}
                  {'{track_number}'} of {'{total_tracks}'}" → "To Kill a
                  Mockingbird - Part 03 of 50.m4b"
                </Typography>
              </Alert>
            </Grid>

            {/* Import Paths Section */}
            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom sx={{ mt: 2 }}>
                Import Paths (Watch Locations)
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12}>
              <Alert severity="info" sx={{ mb: 2 }}>
                <strong>Import Paths</strong> are watched for new audiobook
                files. Files found here are scanned and imported into the main
                library path where they are organized.
              </Alert>

              <Box>
                {importPaths.length === 0 ? (
                  <Alert severity="warning" sx={{ mb: 2 }}>
                    No import folders configured. Add folders to automatically
                    import audiobooks from specific locations.
                  </Alert>
                ) : (
                  <List>
                    {importPaths.map((folder) => {
                      const scanStatus = scanStatuses[folder.id];
                      const errorCount = scanStatus?.errors?.length || 0;
                      const isScanning = scanStatus?.status === 'scanning';
                      let secondaryText = `${folder.book_count || 0} books`;
                      if (scanStatus) {
                        if (scanStatus.status === 'scanning') {
                          secondaryText =
                            `Scanning... Scanned ${scanStatus.scanned} files`;
                        } else if (scanStatus.status === 'complete') {
                          if (errorCount > 0) {
                            secondaryText =
                              'Scan complete. Found ' +
                              scanStatus.scanned +
                              ' audiobooks, ' +
                              errorCount +
                              ' errors.';
                          } else {
                            secondaryText =
                              'Scan complete. Found ' +
                              scanStatus.scanned +
                              ' audiobooks.';
                          }
                        } else if (scanStatus.status === 'cancelled') {
                          secondaryText =
                            'Scan cancelled. Processed ' +
                            scanStatus.scanned +
                            ' files.';
                        } else if (scanStatus.status === 'error') {
                          secondaryText =
                            errorCount > 0
                              ? `Scan failed. ${errorCount} errors.`
                              : 'Scan failed.';
                        }
                      }

                      return (
                        <ListItem
                          key={folder.id}
                          secondaryAction={
                            <Stack direction="row" spacing={1}>
                              {scanStatus && errorCount > 0 && (
                                <Button
                                  size="small"
                                  onClick={() =>
                                    handleViewScanErrors(
                                      folder,
                                      scanStatus
                                    )
                                  }
                                >
                                  View Errors
                                </Button>
                              )}
                              {isScanning && (
                                <Button
                                  size="small"
                                  color="error"
                                  variant="outlined"
                                  onClick={() =>
                                    handleRequestCancelScan(folder)
                                  }
                                >
                                  Cancel Scan
                                </Button>
                              )}
                              <Button
                                size="small"
                                variant="outlined"
                                onClick={() => handleScanImportFolder(folder)}
                                disabled={isScanning}
                              >
                                {isScanning ? 'Scanning...' : 'Scan'}
                              </Button>
                              <IconButton
                                edge="end"
                                onClick={() =>
                                  handleRemoveImportFolder(folder.id)
                                }
                              >
                                <DeleteIcon />
                              </IconButton>
                            </Stack>
                          }
                        >
                          <ListItemIcon>
                            <FolderIcon />
                          </ListItemIcon>
                          <ListItemText
                            primary={folder.path}
                            secondary={secondaryText}
                          />
                        </ListItem>
                      );
                    })}
                  </List>
                )}

                <Button
                  variant="contained"
                  startIcon={<AddIcon />}
                  onClick={() => setAddFolderDialogOpen(true)}
                  sx={{ mt: 2 }}
                >
                  Add Import Path
                </Button>
              </Box>
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.createBackups}
                    onChange={(e) =>
                      handleChange('createBackups', e.target.checked)
                    }
                  />
                }
                label="Create backups before modifying files"
              />
            </Grid>

            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom sx={{ mt: 2 }}>
                Backups
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12}>
              {backupNotice && (
                <Alert severity={backupNotice.severity} sx={{ mb: 2 }}>
                  {backupNotice.message}
                </Alert>
              )}
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <Button
                  variant="contained"
                  onClick={handleCreateBackup}
                  disabled={createBackupInProgress}
                >
                  {createBackupInProgress ? 'Creating...' : 'Create Backup'}
                </Button>
                {createBackupInProgress && <CircularProgress size={20} />}
              </Stack>
              {backupsLoading ? (
                <Stack direction="row" spacing={1} alignItems="center">
                  <CircularProgress size={18} />
                  <Typography variant="body2">Loading backups...</Typography>
                </Stack>
              ) : backups.length === 0 ? (
                <Alert severity="info">No backups available yet.</Alert>
              ) : (
                <List>
                  {backups.map((backup) => (
                    <ListItem key={backup.filename}>
                      <ListItemIcon>
                        <FolderIcon />
                      </ListItemIcon>
                      <ListItemText
                        primary={
                          <Stack
                            direction="row"
                            spacing={1}
                            alignItems="center"
                          >
                            <Typography variant="body2">
                              {backup.filename}
                            </Typography>
                            {(backup.auto ||
                              backup.trigger === 'schedule') && (
                              <Chip label="Auto" size="small" color="info" />
                            )}
                          </Stack>
                        }
                        secondary={`${(backup.size / (1024 * 1024)).toFixed(
                          2
                        )} MB • ${new Date(
                          backup.created_at
                        ).toLocaleString()}`}
                      />
                      <ListItemSecondaryAction>
                        <Stack direction="row" spacing={1}>
                          <Button
                            size="small"
                            component="a"
                            href={`/api/v1/backup/${backup.filename}`}
                            download
                          >
                            Download
                          </Button>
                          <Button
                            size="small"
                            variant="outlined"
                            onClick={() => handleRequestRestore(backup)}
                          >
                            Restore
                          </Button>
                          <Button
                            size="small"
                            color="error"
                            variant="outlined"
                            onClick={() => handleRequestDeleteBackup(backup)}
                          >
                            Delete
                          </Button>
                        </Stack>
                      </ListItemSecondaryAction>
                    </ListItem>
                  ))}
                </List>
              )}
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
                    onChange={(e) =>
                      handleChange('enableDiskQuota', e.target.checked)
                    }
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
                  onChange={(e) =>
                    handleChange(
                      'diskQuotaPercent',
                      parseInt(e.target.value) || 0
                    )
                  }
                  InputProps={{
                    endAdornment: (
                      <InputAdornment position="end">%</InputAdornment>
                    ),
                  }}
                  inputProps={{ min: 1, max: 100 }}
                  helperText={
                    'Maximum percentage of disk space the library can use'
                  }
                />
              </Grid>
            )}

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.enableUserQuotas}
                    onChange={(e) =>
                      handleChange('enableUserQuotas', e.target.checked)
                    }
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
                  onChange={(e) =>
                    handleChange(
                      'defaultUserQuotaGB',
                      parseInt(e.target.value) || 0
                    )
                  }
                  InputProps={{
                    endAdornment: (
                      <InputAdornment position="end">GB</InputAdornment>
                    ),
                  }}
                  inputProps={{ min: 1, max: 10000 }}
                  helperText="Storage limit per user"
                />
              </Grid>
            )}
          </Grid>
        </TabPanel>

        <TabPanel value={tabValue} index={1}>
          <ITunesImport />
        </TabPanel>

        <TabPanel value={tabValue} index={2}>
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
                    onChange={(e) =>
                      handleChange('autoFetchMetadata', e.target.checked)
                    }
                  />
                }
                label="Automatically fetch missing metadata"
              />
            </Grid>

            {/* AI-Powered Parsing Section */}
            <Grid item xs={12}>
              <Typography variant="subtitle1" gutterBottom sx={{ mt: 2 }}>
                AI-Powered Filename Parsing
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.enableAIParsing}
                    onChange={(e) =>
                      handleChange('enableAIParsing', e.target.checked)
                    }
                  />
                }
                label="Enable AI-powered filename parsing"
              />
              <Alert severity="info" sx={{ mt: 1, mb: 2 }}>
                <Typography variant="caption">
                  <strong>What is this?</strong> Uses OpenAI to intelligently
                  parse complex audiobook filenames into title, author, series,
                  narrator, etc. This dramatically improves metadata extraction
                  from poorly named files where traditional parsing fails.
                </Typography>
              </Alert>
            </Grid>

            <Grid item xs={12}>
              <Typography variant="subtitle1" gutterBottom sx={{ mt: 2 }}>
                API Keys
              </Typography>
              <Divider sx={{ mb: 2 }} />
            </Grid>

            <Grid item xs={12}>
              <Stack
                direction={{ xs: 'column', md: 'row' }}
                spacing={2}
                alignItems={{ xs: 'stretch', md: 'center' }}
              >
                <TextField
                  fullWidth
                  label="OpenAI API Key"
                  type="password"
                  value={settings.openaiApiKey}
                  onChange={(e) => {
                    handleChange('openaiApiKey', e.target.value);
                    // Clear saved mask when user starts typing
                    if (e.target.value && savedApiKeyMask) {
                      setSavedApiKeyMask('');
                    }
                  }}
                  disabled={!settings.enableAIParsing}
                  error={Boolean(openaiKeyError)}
                  placeholder={openaiPlaceholder}
                  helperText={openaiHelperText}
                />
                <Button
                  variant="outlined"
                  onClick={handleTestAIConnection}
                  disabled={
                    !settings.enableAIParsing ||
                    openaiTestState.status === 'loading'
                  }
                >
                  {openaiTestState.status === 'loading'
                    ? 'Testing...'
                    : 'Test Connection'}
                </Button>
              </Stack>
              {openaiTestState.status === 'success' && (
                <Alert severity="success" sx={{ mt: 2 }}>
                  {openaiTestState.message}
                </Alert>
              )}
              {openaiTestState.status === 'error' && (
                <Alert severity="error" sx={{ mt: 2 }}>
                  {openaiTestState.message}
                </Alert>
              )}
            </Grid>

            <Grid item xs={12}>
              <Typography variant="subtitle1" gutterBottom>
                Metadata Sources (Priority Order)
              </Typography>
              <Alert severity="info" sx={{ mb: 2 }}>
                <Typography variant="caption">
                  Sources are queried in order. If a field is missing from the
                  first source, the system automatically falls back to the next
                  enabled source to fill in additional fields.
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
                        bgcolor: source.enabled
                          ? 'transparent'
                          : 'action.disabledBackground',
                      }}
                    >
                      <Box
                        sx={{ display: 'flex', flexDirection: 'column', mr: 1 }}
                      >
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
                          disabled={
                            index === settings.metadataSources.length - 1
                          }
                          sx={{ minWidth: 'auto', p: 0.5 }}
                        >
                          ▼
                        </Button>
                      </Box>
                      <DragHandleIcon sx={{ mr: 2, color: 'text.disabled' }} />
                      <Box
                        sx={{ display: 'flex', alignItems: 'center', flex: 1 }}
                      >
                        <Typography
                          variant="body1"
                          sx={{
                            fontWeight: source.enabled ? 500 : 400,
                            color: source.enabled
                              ? 'text.primary'
                              : 'text.disabled',
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
                          onClick={() =>
                            setExpandedSource(
                              expandedSource === source.id ? null : source.id
                            )
                          }
                          sx={{ mr: 1 }}
                        >
                          <ExpandMoreIcon
                            sx={{
                              transform:
                                expandedSource === source.id
                                  ? 'rotate(180deg)'
                                  : 'rotate(0deg)',
                              transition: 'transform 0.3s',
                            }}
                          />
                        </IconButton>
                      )}
                      <Button
                        size="small"
                        onClick={() => handleSourceToggle(source.id)}
                        startIcon={
                          source.enabled ? (
                            <CheckBoxIcon />
                          ) : (
                            <CheckBoxOutlineBlankIcon />
                          )
                        }
                      >
                        {source.enabled ? 'Enabled' : 'Disabled'}
                      </Button>
                    </Box>
                    {source.requiresAuth && (
                      <Collapse in={expandedSource === source.id}>
                        <Box
                          sx={{
                            p: 2,
                            bgcolor: 'background.default',
                            borderTop: 1,
                            borderColor: 'divider',
                          }}
                        >
                          <Typography
                            variant="subtitle2"
                            gutterBottom
                            sx={{
                              display: 'flex',
                              alignItems: 'center',
                              mb: 2,
                            }}
                          >
                            <SettingsIcon sx={{ mr: 1, fontSize: 18 }} />
                            API Configuration
                          </Typography>
                          <Grid container spacing={2}>
                            {source.id === 'google-books' && (
                              <>
                                <Grid item xs={12}>
                                  <TextField
                                    fullWidth
                                    size="small"
                                    label="API Key"
                                    value={source.credentials.apiKey || ''}
                                    onChange={(e) =>
                                      handleCredentialChange(
                                        source.id,
                                        'apiKey',
                                        e.target.value
                                      )
                                    }
                                    placeholder={
                                      'Enter your ' + source.name + ' API key'
                                    }
                                  />
                                </Grid>
                                <Grid item xs={12}>
                                  <Typography
                                    variant="caption"
                                    color="text.secondary"
                                  >
                                    Get your API key at:{' '}
                                    <a
                                      href={
                                        'https://console.' +
                                        'cloud.google.com/' +
                                        'apis/' +
                                        'credentials'
                                      }
                                      target="_blank"
                                      rel="noopener noreferrer"
                                    >
                                      Google Cloud Console
                                    </a>
                                  </Typography>
                                </Grid>
                              </>
                            )}
                          </Grid>
                        </Box>
                      </Collapse>
                    )}
                    {index < settings.metadataSources.length - 1 && <Divider />}
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

            {/* Open Library Data Dumps Section */}
            <Grid item xs={12}>
              <Typography variant="subtitle1" gutterBottom sx={{ mt: 2 }}>
                Open Library Data Dumps
              </Typography>
              <Divider sx={{ mb: 2 }} />
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Download Open Library data dumps for fast, offline metadata
                lookups. Dumps are ~12GB total and enable near-instant ISBN and
                title searches without API rate limits.
              </Typography>
            </Grid>

            <Grid item xs={12}>
              <OpenLibraryDumps />
            </Grid>
          </Grid>
        </TabPanel>

        <TabPanel value={tabValue} index={3}>
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
                onChange={(e) =>
                  handleChange('concurrentScans', parseInt(e.target.value) || 1)
                }
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
                  onChange={(e) =>
                    handleChange('memoryLimitType', e.target.value)
                  }
                >
                  <FormControlLabel
                    value="items"
                    control={<Radio />}
                    label="Number of Items"
                  />
                  <FormControlLabel
                    value="percent"
                    control={<Radio />}
                    label="% of System Memory"
                  />
                  <FormControlLabel
                    value="absolute"
                    control={<Radio />}
                    label="Absolute MB"
                  />
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
                  onChange={(e) =>
                    handleChange('cacheSize', parseInt(e.target.value) || 100)
                  }
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
                  onChange={(e) =>
                    handleChange(
                      'memoryLimitPercent',
                      parseInt(e.target.value) || 1
                    )
                  }
                  InputProps={{
                    endAdornment: (
                      <InputAdornment position="end">%</InputAdornment>
                    ),
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
                  onChange={(e) =>
                    handleChange(
                      'memoryLimitMB',
                      parseInt(e.target.value) || 128
                    )
                  }
                  InputProps={{
                    endAdornment: (
                      <InputAdornment position="end">MB</InputAdornment>
                    ),
                  }}
                  inputProps={{ min: 128, max: 16384 }}
                  helperText="Absolute memory limit in megabytes"
                />
              </Grid>
            )}

            <Grid item xs={12}>
              <Divider sx={{ my: 2 }} />
              <Typography variant="subtitle1" gutterBottom>
                Lifecycle &amp; Retention
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                Control how long soft-deleted books remain before automatic
                purge runs.
              </Typography>
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                type="number"
                label="Auto-Purge After (days)"
                value={settings.purgeSoftDeletedAfterDays}
                onChange={(e) =>
                  handleChange(
                    'purgeSoftDeletedAfterDays',
                    parseInt(e.target.value) || 0
                  )
                }
                inputProps={{ min: 0, max: 365 }}
                helperText="Set to 0 to disable automatic purge"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.purgeSoftDeletedDeleteFiles}
                    onChange={(e) =>
                      handleChange(
                        'purgeSoftDeletedDeleteFiles',
                        e.target.checked
                      )
                    }
                  />
                }
                label="Delete files from disk during purge"
              />
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: 'block', ml: 4 }}
              >
                Disable to keep files on disk while clearing database records.
              </Typography>
            </Grid>

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
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mt: 1, display: 'block' }}
              >
                JSON logging is recommended for log aggregation and analysis
                tools
              </Typography>
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.enableJsonLogging}
                    onChange={(e) =>
                      handleChange('enableJsonLogging', e.target.checked)
                    }
                  />
                }
                label="Enable JSON structured logging to separate file"
              />
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: 'block', ml: 4 }}
              >
                Creates a separate .json log file in addition to the main log
              </Typography>
            </Grid>
          </Grid>
        </TabPanel>

        <TabPanel value={tabValue} index={4}>
          <BlockedHashesTab />
        </TabPanel>

        <TabPanel value={tabValue} index={5}>
          <SystemInfoTab />

          <Paper
            sx={{
              mt: 4,
              p: 3,
              border: 2,
              borderColor: 'error.main',
              borderRadius: 1,
            }}
          >
            <Typography variant="h6" color="error" gutterBottom>
              Danger Zone
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Permanently delete all data including audiobooks, authors, series,
              settings, and metadata cache. This cannot be undone.
            </Typography>
            <Button
              variant="outlined"
              color="error"
              onClick={() => setFactoryResetStep(1)}
              disabled={factoryResetInProgress}
            >
              Factory Reset
            </Button>
          </Paper>

          {/* Factory Reset Dialog 1: Warning */}
          <Dialog
            open={factoryResetStep === 1}
            onClose={() => setFactoryResetStep(0)}
          >
            <DialogTitle>Factory Reset</DialogTitle>
            <DialogContent>
              <Typography>
                This will permanently delete <strong>ALL</strong> data —
                audiobooks, authors, series, settings, and metadata cache. This
                action cannot be undone.
              </Typography>
              <Typography sx={{ mt: 1 }}>Continue?</Typography>
            </DialogContent>
            <DialogActions>
              <Button onClick={() => setFactoryResetStep(0)}>Cancel</Button>
              <Button
                color="error"
                onClick={() => {
                  setFactoryResetConfirmText('');
                  setFactoryResetStep(2);
                }}
              >
                Continue
              </Button>
            </DialogActions>
          </Dialog>

          {/* Factory Reset Dialog 2: Type RESET */}
          <Dialog
            open={factoryResetStep === 2}
            onClose={() => setFactoryResetStep(0)}
          >
            <DialogTitle>Confirm Factory Reset</DialogTitle>
            <DialogContent>
              <Typography sx={{ mb: 2 }}>
                Type <strong>RESET</strong> to confirm.
              </Typography>
              <TextField
                autoFocus
                fullWidth
                value={factoryResetConfirmText}
                onChange={(e: ChangeEvent<HTMLInputElement>) =>
                  setFactoryResetConfirmText(e.target.value)
                }
                placeholder="Type RESET"
              />
            </DialogContent>
            <DialogActions>
              <Button onClick={() => setFactoryResetStep(0)}>Cancel</Button>
              <Button
                color="error"
                variant="contained"
                disabled={
                  factoryResetConfirmText !== 'RESET' ||
                  factoryResetInProgress
                }
                onClick={async () => {
                  setFactoryResetInProgress(true);
                  try {
                    await api.factoryReset('RESET');
                    localStorage.clear();
                    window.location.href = '/';
                  } catch (err) {
                    setFactoryResetInProgress(false);
                    setFactoryResetStep(0);
                    alert(
                      `Factory reset failed: ${err instanceof Error ? err.message : 'Unknown error'}`
                    );
                  }
                }}
              >
                {factoryResetInProgress ? 'Resetting...' : 'Reset Everything'}
              </Button>
            </DialogActions>
          </Dialog>
        </TabPanel>

        <Box
          sx={{
            p: 2,
            display: 'flex',
            gap: 2,
            justifyContent: 'flex-end',
            borderTop: 1,
            borderColor: 'divider',
            bgcolor: 'background.paper',
          }}
        >
          <input
            type="file"
            accept="application/json"
            ref={importInputRef}
            onChange={handleImportFileChange}
            style={{ display: 'none' }}
          />
          <Button
            variant="outlined"
            onClick={handleExportSettings}
            disabled={exportInProgress}
          >
            {exportInProgress ? 'Exporting...' : 'Export Settings'}
          </Button>
          <Button
            variant="outlined"
            onClick={handleImportClick}
            disabled={importInProgress}
          >
            {importInProgress ? 'Importing...' : 'Import Settings'}
          </Button>
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

      {/* Library Path Browser Dialog */}
      <Dialog
        open={browserOpen}
        onClose={handleBrowserCancel}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Browse Server Filesystem</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            Select the library path where organized audiobooks will be stored.
          </Typography>
          <Box sx={{ mt: 2 }}>
            <ServerFileBrowser
              initialPath={selectedPath || settings.libraryPath}
              onSelect={handleBrowserSelect}
              showFiles={false}
              allowDirSelect={true}
              allowFileSelect={false}
            />
          </Box>
          {selectedPath && (
            <Alert severity="info" sx={{ mt: 2 }}>
              <Typography variant="body2">
                <strong>Selected:</strong> {selectedPath}
              </Typography>
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleBrowserCancel}>Cancel</Button>
          <Button
            onClick={handleBrowserConfirm}
            variant="contained"
            disabled={!selectedPath}
          >
            Select Folder
          </Button>
        </DialogActions>
      </Dialog>

      {/* Import Folder Dialog */}
      <Dialog
        open={addFolderDialogOpen}
        onClose={() => setAddFolderDialogOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Add Import Path (Watch Location)</DialogTitle>
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            <strong>Import paths</strong> are separate from your main library.
            They are watched for new audiobook files that will be scanned,
            organized, and moved to your library path.
          </Alert>

          {!showFolderBrowser ? (
            <Box>
              <TextField
                autoFocus
                fullWidth
                label="Folder Path"
                value={newFolderPath}
                onChange={(e) => setNewFolderPath(e.target.value)}
                placeholder="/path/to/downloads"
                sx={{ mt: 1 }}
              />
              <Button
                startIcon={<FolderOpenIcon />}
                onClick={() => setShowFolderBrowser(true)}
                sx={{ mt: 2 }}
              >
                Browse Server Filesystem
              </Button>
            </Box>
          ) : (
            <Box>
              <Button
                onClick={() => setShowFolderBrowser(false)}
                sx={{ mb: 2 }}
              >
                ← Back to Manual Entry
              </Button>
              <ServerFileBrowser
                initialPath={newFolderPath || '/'}
                onSelect={handleFolderBrowserSelect}
                showFiles={false}
                allowDirSelect={true}
                allowFileSelect={false}
              />
              {newFolderPath && (
                <Alert severity="success" sx={{ mt: 2 }}>
                  <Typography variant="body2">
                    <strong>Selected:</strong> {newFolderPath}
                  </Typography>
                </Alert>
              )}
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setAddFolderDialogOpen(false);
              setShowFolderBrowser(false);
            }}
          >
            Cancel
          </Button>
          <Button
            onClick={handleAddImportFolder}
            variant="contained"
            disabled={!newFolderPath.trim()}
          >
            Add Path
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={Boolean(cancelScanTarget)}
        onClose={() => setCancelScanTarget(null)}
      >
        <DialogTitle>Cancel Scan</DialogTitle>
        <DialogContent>
          <Typography variant="body2" gutterBottom>
            Cancel scan for{' '}
            <strong>{cancelScanTarget?.path || 'this folder'}</strong>?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCancelScanTarget(null)}>
            Keep Scanning
          </Button>
          <Button
            color="error"
            variant="contained"
            onClick={handleConfirmCancelScan}
          >
            Cancel Scan
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={Boolean(scanErrorTarget)}
        onClose={() => setScanErrorTarget(null)}
      >
        <DialogTitle>Scan Errors</DialogTitle>
        <DialogContent>
          <Typography variant="body2" gutterBottom>
            Errors while scanning{' '}
            <strong>{scanErrorTarget?.path || 'this folder'}</strong>:
          </Typography>
          {scanErrorTarget?.errors?.length ? (
            <List dense>
              {scanErrorTarget.errors.map((error, index) => (
                <ListItem key={`${error}-${index}`}>
                  <ListItemText primary={error} />
                </ListItem>
              ))}
            </List>
          ) : (
            <Typography variant="body2" color="text.secondary">
              No errors recorded.
            </Typography>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setScanErrorTarget(null)}>Close</Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={restoreDialogOpen}
        onClose={() => setRestoreDialogOpen(false)}
      >
        <DialogTitle>Restore Backup</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            This will replace the current database with the selected backup.
          </Alert>
          <Typography variant="body2" gutterBottom>
            Restore from{' '}
            <strong>{restoreTarget?.filename || 'selected backup'}</strong>?
          </Typography>
          <FormControlLabel
            control={
              <Switch
                checked={restoreVerify}
                onChange={(e) => setRestoreVerify(e.target.checked)}
              />
            }
            label="Verify backup before restore"
          />
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setRestoreDialogOpen(false)}
            disabled={restoreInProgress}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleConfirmRestore}
            disabled={restoreInProgress}
          >
            {restoreInProgress ? 'Restoring...' : 'Restore'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={Boolean(deleteBackupTarget)}
        onClose={() => setDeleteBackupTarget(null)}
      >
        <DialogTitle>Delete Backup</DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            Delete{' '}
            <strong>{deleteBackupTarget?.filename || 'this backup'}</strong>?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setDeleteBackupTarget(null)}
            disabled={deleteBackupInProgress}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            color="error"
            onClick={handleConfirmDeleteBackup}
            disabled={deleteBackupInProgress}
          >
            {deleteBackupInProgress ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={importDialogOpen}
        onClose={() => setImportDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Import Settings</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            Importing settings will overwrite your current configuration.
          </Alert>
          <Typography variant="body2" gutterBottom>
            Import from <strong>{importFileName || 'selected file'}</strong>?
          </Typography>
          {importKeys.length > 0 && (
            <List dense>
              {importKeys.slice(0, 12).map((key) => (
                <ListItem key={key}>
                  <ListItemText primary={key} />
                </ListItem>
              ))}
              {importKeys.length > 12 && (
                <ListItem>
                  <ListItemText
                    primary={`+${importKeys.length - 12} more fields`}
                  />
                </ListItem>
              )}
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setImportDialogOpen(false)}
            disabled={importInProgress}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleConfirmImport}
            disabled={importInProgress}
          >
            {importInProgress ? 'Importing...' : 'Import Settings'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={blocker.state === 'blocked'}
        onClose={handleCancelNavigation}
      >
        <DialogTitle>Unsaved Changes</DialogTitle>
        <DialogContent>
          <Typography variant="body2" gutterBottom>
            You have unsaved changes. Save before leaving?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCancelNavigation}>Cancel</Button>
          <Button onClick={handleDiscardNavigation}>Discard</Button>
          <Button variant="contained" onClick={handleSaveAndNavigate}>
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
