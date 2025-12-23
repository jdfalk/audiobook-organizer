// file: web/src/components/audiobooks/MetadataEditDialog.tsx
// version: 1.1.0
// guid: 4a5b6c7d-8e9f-0a1b-2c3d-4e5f6a7b8c9d

import React, { useState, useEffect } from 'react';
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
} from '@mui/material';
import type { Audiobook } from '../../types';

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

  useEffect(() => {
    if (audiobook) {
      setFormData(audiobook);
    }
  }, [audiobook]);

  const handleChange = (field: keyof Audiobook, value: string | number) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleSave = async () => {
    if (!audiobook) return;

    setSaving(true);
    setError(null);

    try {
      await onSave({ ...audiobook, ...formData });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save metadata');
    } finally {
      setSaving(false);
    }
  };

  if (!audiobook) return null;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Edit Metadata</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        <Box sx={{ mt: 2 }}>
          <Grid container spacing={2}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Title"
                value={formData.title || ''}
                onChange={(e) => handleChange('title', e.target.value)}
                required
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="Author"
                value={formData.author || ''}
                onChange={(e) => handleChange('author', e.target.value)}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="Narrator"
                value={formData.narrator || ''}
                onChange={(e) => handleChange('narrator', e.target.value)}
              />
            </Grid>

            <Grid item xs={12} sm={8}>
              <TextField
                fullWidth
                label="Series"
                value={formData.series || ''}
                onChange={(e) => handleChange('series', e.target.value)}
              />
            </Grid>

            <Grid item xs={12} sm={4}>
              <TextField
                fullWidth
                label="Series Number"
                type="number"
                value={formData.series_number || ''}
                onChange={(e) => handleChange('series_number', parseInt(e.target.value) || 0)}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="Genre"
                value={formData.genre || ''}
                onChange={(e) => handleChange('genre', e.target.value)}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="Year"
                type="number"
                value={formData.year || ''}
                onChange={(e) => handleChange('year', parseInt(e.target.value) || 0)}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="Language"
                value={formData.language || ''}
                onChange={(e) => handleChange('language', e.target.value)}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="Publisher"
                value={formData.publisher || ''}
                onChange={(e) => handleChange('publisher', e.target.value)}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="ISBN-10"
                value={formData.isbn10 || ''}
                onChange={(e) => handleChange('isbn10', e.target.value)}
              />
            </Grid>

            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="ISBN-13"
                value={formData.isbn13 || ''}
                onChange={(e) => handleChange('isbn13', e.target.value)}
              />
            </Grid>

            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Description"
                multiline
                rows={4}
                value={formData.description || ''}
                onChange={(e) => handleChange('description', e.target.value)}
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
          {saving ? 'Saving...' : 'Save'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
