// file: web/src/components/BatchToolbar.tsx
// version: 1.0.0
// guid: 5e6f7a8b-9c0d-1e2f-3a4b5c6d7e8f9a0b
// last-edited: 2026-04-30

import { Button, Chip, Box, Stack } from '@mui/material';

export interface BatchToolbarProps {
  selectedCount: number;
  onBatchEditClick: () => void;
  onFetchReviewClick: () => void;
  onResumeReviewClick: () => void;
  onSearchMetadataClick: () => void;
  onSaveToFilesClick: () => void;
  onOrganizeSelectedClick: () => void;
  onMergeAsVersionsClick: () => void;
  onTagClick: () => void;
  onRateClick: () => void;
  onDeleteSelectedClick: () => void;
  onRestoreSelectedClick: () => void;
  batchRestoreInProgress: boolean;
  selectedHasActive: boolean;
  selectedHasDeleted: boolean;
  selectedHasImport: boolean;
  selectedAudiobooksLength: number;
}

export const BatchToolbar = ({
  selectedCount,
  onBatchEditClick,
  onFetchReviewClick,
  onResumeReviewClick,
  onSearchMetadataClick,
  onSaveToFilesClick,
  onOrganizeSelectedClick,
  onMergeAsVersionsClick,
  onTagClick,
  onRateClick,
  onDeleteSelectedClick,
  onRestoreSelectedClick,
  batchRestoreInProgress,
  selectedHasActive,
  selectedHasDeleted,
  selectedHasImport,
  selectedAudiobooksLength,
}: BatchToolbarProps) => {
  if (selectedCount === 0) {
    return null;
  }

  return (
    <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" sx={{ width: '100%' }}>
      <Chip label={`${selectedCount} selected`} size="small" color="primary" />
      <Button size="small" variant="outlined" onClick={onBatchEditClick} disabled={!selectedCount}>
        Batch Edit
      </Button>
      <Button
        size="small"
        variant="outlined"
        color="primary"
        onClick={onFetchReviewClick}
        disabled={selectedAudiobooksLength < 2}
      >
        Fetch & Review
      </Button>
      <Button
        size="small"
        variant="outlined"
        onClick={onResumeReviewClick}
        title="Pick a recent fetch operation to review — useful when multiple fetches completed without review or after a page reload"
      >
        Resume Review
      </Button>
      <Button size="small" variant="outlined" onClick={onSearchMetadataClick} disabled={!selectedCount}>
        Search Metadata
      </Button>
      <Box sx={{ borderLeft: 1, borderColor: 'divider', height: 24 }} />
      <Button size="small" variant="outlined" onClick={onSaveToFilesClick} disabled={!selectedHasActive}>
        Save to Files
      </Button>
      <Button size="small" variant="outlined" onClick={onOrganizeSelectedClick} disabled={!selectedHasImport}>
        Organize Selected
      </Button>
      <Box sx={{ borderLeft: 1, borderColor: 'divider', height: 24 }} />
      <Button
        size="small"
        variant="outlined"
        color="primary"
        onClick={onMergeAsVersionsClick}
        disabled={selectedAudiobooksLength < 2}
      >
        Merge as Versions
      </Button>
      <Button size="small" variant="outlined" onClick={onTagClick} disabled={!selectedCount}>
        Tag
      </Button>
      <Button size="small" variant="outlined" onClick={onRateClick} disabled={!selectedCount}>
        Rate
      </Button>
      <Box sx={{ flex: 1 }} />
      <Button
        size="small"
        variant="outlined"
        color="secondary"
        onClick={onDeleteSelectedClick}
        disabled={!selectedHasActive}
      >
        Delete Selected
      </Button>
      <Button
        size="small"
        variant="outlined"
        color="success"
        onClick={onRestoreSelectedClick}
        disabled={!selectedHasDeleted || batchRestoreInProgress}
      >
        {batchRestoreInProgress ? 'Restoring...' : 'Restore Selected'}
      </Button>
    </Stack>
  );
};
