// file: web/src/components/audiobooks/MetadataEditDialog.tsx
// version: 1.3.0
// guid: 4a5b6c7d-8e9f-0a1b-2c3d-4e5f6a7b8c9d

import React, { useState, useEffect, useCallback } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Grid,
  Box,
  Alert,
  Typography,
  IconButton,
  Tooltip,
  CircularProgress,
} from '@mui/material';
import LockIcon from '@mui/icons-material/Lock.js';
import LockOpenIcon from '@mui/icons-material/LockOpen.js';
import type { Audiobook } from '../../types';
import type { MetadataFieldStates, MetadataFieldStateEntry } from '../../services/api';
import * as api from '../../services/api';

// Maps Audiobook field names to their backend field-state keys
const FIELD_STATE_KEYS: Record<string, string> = {
  title: 'title',
  author: 'author_name',
  narrator: 'narrator',
  series: 'series_name',
  series_number: 'series_position',
  genre: 'genre',
  year: 'audiobook_release_year',
  language: 'language',
  publisher: 'publisher',
  isbn10: 'isbn10',
  isbn13: 'isbn13',
  description: 'description',
};

function formatSourceLabel(entry: MetadataFieldStateEntry): string {
  if (entry.override_value !== undefined && entry.override_value !== null) {
    return 'Manual override';
  }
  if (entry.fetched_value !== undefined && entry.fetched_value !== null) {
    return 'Fetched';
  }
  return 'File tags';
}

function formatValue(val: unknown): string {
  if (val === undefined || val === null || val === '') return '';
  return String(val);
}

interface ProvenanceIndicatorProps {
  entry: MetadataFieldStateEntry | undefined;
  currentValue: string;
}

const ProvenanceIndicator: React.FC<ProvenanceIndicatorProps> = ({ entry, currentValue }) => {
  if (!entry) return null;

  const source = formatSourceLabel(entry);
  const fetchedStr = formatValue(entry.fetched_value);
  const showFetched = fetchedStr !== '' && fetchedStr !== currentValue;

  return (
    <Box sx={{ mt: 0.25 }}>
      <Typography variant="caption" color="text.secondary" component="span">
        Source: {source}
      </Typography>
      {showFetched && (
        <Typography variant="caption" color="text.disabled" component="span" sx={{ ml: 1 }}>
          Fetched: {fetchedStr.length > 60 ? fetchedStr.slice(0, 57) + '...' : fetchedStr}
        </Typography>
      )}
    </Box>
  );
};

interface MetadataEditDialogProps {
  open: boolean;
  audiobook: Audiobook | null;
  onClose: () => void;
  onSave: (audiobook: Audiobook) => Promise<void>;
}

export const MetadataEditDialog: React.FC<MetadataEditDialogProps> = ({
  open,
  audiobook,
  onClose,
  onSave,
}) => {
  const [formData, setFormData] = useState<Partial<Audiobook>>({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [yearInput, setYearInput] = useState('');
  const [yearError, setYearError] = useState<string | null>(null);
  const [fieldStates, setFieldStates] = useState<MetadataFieldStates>({});
  const [loadingStates, setLoadingStates] = useState(false);

  const loadFieldStates = useCallback(async (bookId: string) => {
    setLoadingStates(true);
    try {
      const states = await api.getAudiobookFieldStates(bookId);
      setFieldStates(states);
    } catch {
      // Silently fail -- provenance is supplementary
      setFieldStates({});
    } finally {
      setLoadingStates(false);
    }
  }, []);

  useEffect(() => {
    if (audiobook) {
      setFormData(audiobook);
      setYearInput(
        typeof audiobook.year === 'number' ? String(audiobook.year) : ''
      );
      setYearError(null);
    }
  }, [audiobook]);

  useEffect(() => {
    if (open && audiobook?.id) {
      loadFieldStates(audiobook.id);
    }
    if (!open) {
      setFieldStates({});
    }
  }, [open, audiobook?.id, loadFieldStates]);

  const handleChange = (field: keyof Audiobook, value: string | number) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleYearChange = (value: string) => {
    setYearInput(value);
    if (value.trim() === '') {
      setYearError(null);
      setFormData((prev) => ({ ...prev, year: undefined }));
      return;
    }

    const parsed = Number(value);
    if (!Number.isFinite(parsed)) {
      setYearError('Year must be a number');
      return;
    }

    setYearError(null);
    setFormData((prev) => ({ ...prev, year: parsed }));
  };

  const handleSave = async () => {
    if (!audiobook) return;

    setSaving(true);
    setError(null);

    try {
      if (yearError) {
        setError(yearError);
        setSaving(false);
        return;
      }
      await onSave({ ...audiobook, ...formData });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save metadata');
    } finally {
      setSaving(false);
    }
  };

  const getFieldState = (formField: string): MetadataFieldStateEntry | undefined => {
    const stateKey = FIELD_STATE_KEYS[formField];
    return stateKey ? fieldStates[stateKey] : undefined;
  };

  const renderLockButton = (formField: string) => {
    const entry = getFieldState(formField);
    if (!entry) return null;
    const locked = entry.override_locked;
    return (
      <Tooltip title={locked ? 'Field locked -- will not be overwritten by metadata fetches' : 'Field unlocked -- may be updated by metadata fetches'}>
        <IconButton size="small" sx={{ ml: 0.5 }} disabled>
          {locked ? <LockIcon fontSize="small" color="warning" /> : <LockOpenIcon fontSize="small" color="disabled" />}
        </IconButton>
      </Tooltip>
    );
  };

  const renderField = (
    field: keyof Audiobook,
    label: string,
    options?: { multiline?: boolean; rows?: number; type?: string; gridXs?: number; gridSm?: number }
  ) => {
    const { multiline, rows, type, gridXs = 12, gridSm } = options || {};
    const currentVal = field === 'year' ? yearInput : String(formData[field] ?? '');
    const entry = getFieldState(field);

    return (
      <Grid item xs={gridXs} sm={gridSm}>
        <Box sx={{ display: 'flex', alignItems: 'flex-start' }}>
          <Box sx={{ flexGrow: 1 }}>
            <TextField
              fullWidth
              label={label}
              value={field === 'year' ? yearInput : (formData[field] ?? '')}
              onChange={(e) => {
                if (field === 'year') {
                  handleYearChange(e.target.value);
                } else if (type === 'number') {
                  handleChange(field, parseInt(e.target.value) || 0);
                } else {
                  handleChange(field, e.target.value);
                }
              }}
              multiline={multiline}
              rows={rows}
              type={field === 'year' ? 'text' : type}
              inputMode={field === 'year' ? 'numeric' : undefined}
              error={field === 'year' ? Boolean(yearError) : undefined}
              helperText={field === 'year' ? (yearError || ' ') : undefined}
              required={field === 'title'}
            />
            <ProvenanceIndicator entry={entry} currentValue={currentVal} />
          </Box>
          {renderLockButton(field)}
        </Box>
      </Grid>
    );
  };

  if (!audiobook) return null;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>
        Edit Metadata
        {loadingStates && <CircularProgress size={16} sx={{ ml: 1 }} />}
      </DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        <Box sx={{ mt: 2 }}>
          <Grid container spacing={2}>
            {renderField('title', 'Title')}
            {renderField('author', 'Author', { gridSm: 6 })}
            {renderField('narrator', 'Narrator', { gridSm: 6 })}
            {renderField('series', 'Series', { gridSm: 8 })}
            {renderField('series_number', 'Series Number', { type: 'number', gridSm: 4 })}
            {renderField('genre', 'Genre', { gridSm: 6 })}
            {renderField('year', 'Year', { gridSm: 6 })}
            {renderField('language', 'Language', { gridSm: 6 })}
            {renderField('publisher', 'Publisher', { gridSm: 6 })}
            {renderField('isbn10', 'ISBN-10', { gridSm: 6 })}
            {renderField('isbn13', 'ISBN-13', { gridSm: 6 })}
            {renderField('description', 'Description', { multiline: true, rows: 4 })}
          </Grid>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          Cancel
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {saving ? 'Saving...' : 'Save'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
