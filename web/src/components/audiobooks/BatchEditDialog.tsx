// file: web/src/components/audiobooks/BatchEditDialog.tsx
// version: 1.0.0
// guid: 5b6c7d8e-9f0a-1b2c-3d4e-5f6a7b8c9d0e

import React, { useState } from 'react';
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
  FormControlLabel,
  Checkbox,
} from '@mui/material';
import type { Audiobook } from '../../types';

interface BatchEditDialogProps {
  open: boolean;
  audiobooks: Audiobook[];
  onClose: () => void;
  onSave: (updates: Partial<Audiobook>) => Promise<void>;
}

interface FieldUpdate {
  enabled: boolean;
  value: string | number;
}

export const BatchEditDialog: React.FC<BatchEditDialogProps> = ({
  open,
  audiobooks,
  onClose,
  onSave,
}) => {
  const [updates, setUpdates] = useState<Record<string, FieldUpdate>>({
    author: { enabled: false, value: '' },
    narrator: { enabled: false, value: '' },
    series: { enabled: false, value: '' },
    genre: { enabled: false, value: '' },
    language: { enabled: false, value: '' },
    publisher: { enabled: false, value: '' },
    year: { enabled: false, value: '' },
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleToggle = (field: string) => {
    setUpdates((prev) => ({
      ...prev,
      [field]: { ...prev[field], enabled: !prev[field].enabled },
    }));
  };

  const handleChange = (field: string, value: string | number) => {
    setUpdates((prev) => ({
      ...prev,
      [field]: { ...prev[field], value },
    }));
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);

    try {
      const changedFields: Partial<Audiobook> = {};
      Object.entries(updates).forEach(([field, update]) => {
        if (update.enabled) {
          changedFields[field as keyof Audiobook] = update.value as never;
        }
      });

      await onSave(changedFields);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update audiobooks');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>
        Batch Edit Metadata ({audiobooks.length} audiobook{audiobooks.length !== 1 ? 's' : ''})
      </DialogTitle>
      <DialogContent>
        <Alert severity="info" sx={{ mb: 2 }}>
          Select the fields you want to update. Only checked fields will be modified for all
          selected audiobooks.
        </Alert>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        <Box sx={{ mt: 2 }}>
          <Grid container spacing={2}>
            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={updates.author.enabled}
                    onChange={() => handleToggle('author')}
                  />
                }
                label="Author"
              />
              <TextField
                fullWidth
                placeholder="Author name"
                value={updates.author.value}
                onChange={(e) => handleChange('author', e.target.value)}
                disabled={!updates.author.enabled}
                sx={{ mt: 1 }}
              />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={updates.narrator.enabled}
                    onChange={() => handleToggle('narrator')}
                  />
                }
                label="Narrator"
              />
              <TextField
                fullWidth
                placeholder="Narrator name"
                value={updates.narrator.value}
                onChange={(e) => handleChange('narrator', e.target.value)}
                disabled={!updates.narrator.enabled}
                sx={{ mt: 1 }}
              />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={updates.series.enabled}
                    onChange={() => handleToggle('series')}
                  />
                }
                label="Series"
              />
              <TextField
                fullWidth
                placeholder="Series name"
                value={updates.series.value}
                onChange={(e) => handleChange('series', e.target.value)}
                disabled={!updates.series.enabled}
                sx={{ mt: 1 }}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={updates.genre.enabled}
                    onChange={() => handleToggle('genre')}
                  />
                }
                label="Genre"
              />
              <TextField
                fullWidth
                placeholder="Genre"
                value={updates.genre.value}
                onChange={(e) => handleChange('genre', e.target.value)}
                disabled={!updates.genre.enabled}
                sx={{ mt: 1 }}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={updates.language.enabled}
                    onChange={() => handleToggle('language')}
                  />
                }
                label="Language"
              />
              <TextField
                fullWidth
                placeholder="Language"
                value={updates.language.value}
                onChange={(e) => handleChange('language', e.target.value)}
                disabled={!updates.language.enabled}
                sx={{ mt: 1 }}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={updates.publisher.enabled}
                    onChange={() => handleToggle('publisher')}
                  />
                }
                label="Publisher"
              />
              <TextField
                fullWidth
                placeholder="Publisher"
                value={updates.publisher.value}
                onChange={(e) => handleChange('publisher', e.target.value)}
                disabled={!updates.publisher.enabled}
                sx={{ mt: 1 }}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={updates.year.enabled}
                    onChange={() => handleToggle('year')}
                  />
                }
                label="Year"
              />
              <TextField
                fullWidth
                type="number"
                placeholder="Year"
                value={updates.year.value}
                onChange={(e) => handleChange('year', parseInt(e.target.value) || 0)}
                disabled={!updates.year.enabled}
                sx={{ mt: 1 }}
              />
            </Grid>
          </Grid>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          Cancel
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {saving ? 'Updating...' : `Update ${audiobooks.length} audiobook${audiobooks.length !== 1 ? 's' : ''}`}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
