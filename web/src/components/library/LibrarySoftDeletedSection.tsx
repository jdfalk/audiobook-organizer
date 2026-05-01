// file: web/src/components/library/LibrarySoftDeletedSection.tsx
// version: 1.0.0
// guid: 26804E8D-51BA-462C-9BBE-45ED69E17B9F
// last-edited: 2026-05-01

import {
  Paper,
  Stack,
  Typography,
  Button,
  Chip,
  Collapse,
  Alert,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
} from '@mui/material';
import {
  ExpandMore as ExpandMoreIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import type { Audiobook } from '../../types';

export interface LibrarySoftDeletedSectionProps {
  softDeletedCount: number;
  softDeletedBooks: Audiobook[];
  softDeletedLoading: boolean;
  softDeletedExpanded: boolean;
  restoringBookId: string | null;
  purgeInProgress: boolean;
  purgingBookId: string | null;
  onToggleExpanded: () => void;
  onRefresh: () => void;
  onRestoreOne: (book: Audiobook) => void;
  onPurgeOne: (book: Audiobook) => void;
}

export function LibrarySoftDeletedSection({
  softDeletedCount,
  softDeletedBooks,
  softDeletedLoading,
  softDeletedExpanded,
  restoringBookId,
  purgeInProgress,
  purgingBookId,
  onToggleExpanded,
  onRefresh,
  onRestoreOne,
  onPurgeOne,
}: LibrarySoftDeletedSectionProps) {
  return (
    <Paper sx={{ p: 2, mt: 3 }}>
      <Stack
        direction="row"
        alignItems="center"
        justifyContent="space-between"
        spacing={2}
        sx={{ cursor: 'pointer' }}
        onClick={onToggleExpanded}
      >
        <Stack direction="row" alignItems="center" spacing={1}>
          <ExpandMoreIcon
            sx={{
              transform: softDeletedExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
              transition: 'transform 0.2s',
            }}
          />
          <Typography variant="h6">Soft-Deleted Books</Typography>
        </Stack>
        <Stack direction="row" spacing={1} alignItems="center">
          <Chip
            label={`${softDeletedCount} ${softDeletedCount === 1 ? 'item' : 'items'}`}
            color={softDeletedCount > 0 ? 'warning' : 'default'}
          />
          <Button
            size="small"
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={(e) => {
              e.stopPropagation();
              onRefresh();
            }}
            disabled={softDeletedLoading}
          >
            {softDeletedLoading ? 'Refreshing...' : 'Refresh'}
          </Button>
        </Stack>
      </Stack>
      <Collapse in={softDeletedExpanded}>
        {softDeletedLoading ? (
          <Typography variant="body2" sx={{ mt: 2 }}>
            Loading soft-deleted books...
          </Typography>
        ) : softDeletedBooks.length === 0 ? (
          <Alert severity="info" sx={{ mt: 2 }}>
            No soft-deleted books at the moment.
          </Alert>
        ) : (
          <List dense sx={{ mt: 1 }}>
            {softDeletedBooks.map((book) => {
              const deletedAt =
                book.marked_for_deletion_at && new Date(book.marked_for_deletion_at);
              return (
                <ListItem key={book.id} alignItems="flex-start">
                  <ListItemText
                    primary={book.title || 'Untitled'}
                    secondary={
                      <Stack spacing={0.5}>
                        <Typography variant="body2" color="text.secondary">
                          {book.author || 'Unknown Author'}
                        </Typography>
                        {deletedAt && (
                          <Typography variant="caption" color="text.secondary">
                            Soft deleted at {deletedAt.toLocaleString()}
                          </Typography>
                        )}
                        {book.file_path && (
                          <Typography variant="caption" color="text.secondary">
                            {book.file_path}
                          </Typography>
                        )}
                      </Stack>
                    }
                  />
                  <ListItemSecondaryAction>
                    <Button
                      size="small"
                      variant="outlined"
                      sx={{ mr: 1 }}
                      onClick={() => onRestoreOne(book)}
                      disabled={
                        restoringBookId === book.id ||
                        purgeInProgress ||
                        purgingBookId === book.id
                      }
                    >
                      {restoringBookId === book.id ? 'Restoring...' : 'Restore'}
                    </Button>
                    <Button
                      size="small"
                      color="error"
                      variant="outlined"
                      onClick={() => onPurgeOne(book)}
                      disabled={purgingBookId === book.id || purgeInProgress}
                    >
                      {purgingBookId === book.id ? 'Purging...' : 'Purge now'}
                    </Button>
                  </ListItemSecondaryAction>
                </ListItem>
              );
            })}
          </List>
        )}
      </Collapse>
    </Paper>
  );
}
