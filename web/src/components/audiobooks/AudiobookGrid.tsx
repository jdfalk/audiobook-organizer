// file: web/src/components/audiobooks/AudiobookGrid.tsx
// version: 1.3.0
// guid: 9b0c1d2e-3f4a-5b6c-7d8e-9f0a1b2c3d4e

import React from 'react';
import { Grid, Box, Typography, CircularProgress } from '@mui/material';
import { AudiobookCard } from './AudiobookCard';
import type { Audiobook } from '../../types';

interface AudiobookGridProps {
  audiobooks: Audiobook[];
  loading?: boolean;
  onEdit?: (audiobook: Audiobook) => void;
  onDelete?: (audiobook: Audiobook) => void;
  onClick?: (audiobook: Audiobook) => void;
  onVersionManage?: (audiobook: Audiobook) => void;
  onFetchMetadata?: (audiobook: Audiobook) => void;
  onParseWithAI?: (audiobook: Audiobook) => void;
}

export const AudiobookGrid: React.FC<AudiobookGridProps> = ({
  audiobooks,
  loading = false,
  onEdit,
  onDelete,
  onClick,
  onVersionManage,
  onFetchMetadata,
  onParseWithAI,
}) => {
  if (loading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="400px"
      >
        <CircularProgress />
      </Box>
    );
  }

  if (audiobooks.length === 0) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="400px"
        flexDirection="column"
        gap={2}
      >
        <Typography variant="h6" color="text.secondary">
          No audiobooks found
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Try adjusting your filters or add audiobooks to your library
        </Typography>
      </Box>
    );
  }

  return (
    <Grid container spacing={3}>
      {audiobooks.map((audiobook) => (
        <Grid item key={audiobook.id} xs={12} sm={6} md={4} lg={3} xl={2}>
          <AudiobookCard
            audiobook={audiobook}
            onEdit={onEdit}
            onDelete={onDelete}
            onClick={onClick}
            onVersionManage={onVersionManage}
            onFetchMetadata={onFetchMetadata}
            onParseWithAI={onParseWithAI}
          />
        </Grid>
      ))}
    </Grid>
  );
};
