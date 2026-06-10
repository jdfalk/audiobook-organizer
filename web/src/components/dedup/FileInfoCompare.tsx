// file: web/src/components/dedup/FileInfoCompare.tsx
// version: 1.0.0
// guid: e4d5f6a7-b8c9-0123-defa-ed4567890123
// last-edited: 2026-06-10

// FileInfoCompare renders a side-by-side comparison of two books' file lists.
// Used inside CandidateCompareDrawer.

import { Box, Chip, Divider, Stack, Tooltip, Typography } from '@mui/material';
import type { DedupBookDetail } from '../../services/api';

function formatBytes(bytes: number | undefined): string {
  if (bytes == null) return '';
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

function formatDuration(seconds: number | undefined): string {
  if (seconds == null) return '';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function truncatePath(path: string): string {
  const marker = 'audiobook-organizer/';
  const idx = path.indexOf(marker);
  return idx >= 0 ? path.slice(idx + marker.length) : path;
}

interface BookFilesColumnProps {
  book: DedupBookDetail;
  label: string;
}

function BookFilesColumn({ book, label }: BookFilesColumnProps) {
  return (
    <Box sx={{ flex: 1, minWidth: 0 }}>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5, display: 'block', mb: 0.75 }}
      >
        {label}
      </Typography>
      <Typography variant="body2" fontWeight={600} noWrap title={book.title}>
        {book.title}
      </Typography>
      {book.author_name && (
        <Typography variant="caption" color="text.secondary" noWrap display="block">
          {book.author_name}
        </Typography>
      )}
      <Stack direction="row" spacing={0.5} sx={{ mt: 0.5, mb: 1 }} flexWrap="wrap" useFlexGap>
        {book.format && <Chip label={book.format.toUpperCase()} size="small" />}
        {book.duration != null && (
          <Chip label={formatDuration(book.duration)} size="small" variant="outlined" />
        )}
      </Stack>

      {/* File list */}
      {book.files && book.files.length > 0 ? (
        <Stack spacing={0.5}>
          {book.files.map((f) => (
            <Tooltip
              key={f.id ?? f.file_path}
              title={f.file_path}
              placement="bottom-start"
              componentsProps={{ tooltip: { sx: { maxWidth: 600 } } }}
            >
              <Box
                sx={{
                  p: 0.75,
                  borderRadius: 1,
                  bgcolor: 'action.hover',
                  cursor: 'default',
                  minWidth: 0,
                }}
              >
                <Typography
                  variant="caption"
                  sx={{ fontFamily: 'monospace', fontSize: '0.65rem', display: 'block' }}
                  noWrap
                >
                  {truncatePath(f.file_path)}
                </Typography>
                <Stack direction="row" spacing={0.5} sx={{ mt: 0.25 }}>
                  {f.format && (
                    <Typography variant="caption" color="text.secondary">
                      {f.format.toUpperCase()}
                    </Typography>
                  )}
                  {f.bitrate != null && (
                    <Typography variant="caption" color="text.secondary">
                      {f.bitrate}kbps
                    </Typography>
                  )}
                  {f.file_size != null && (
                    <Typography variant="caption" color="text.secondary">
                      {formatBytes(f.file_size)}
                    </Typography>
                  )}
                  {f.duration != null && (
                    <Typography variant="caption" color="text.secondary">
                      {formatDuration(f.duration)}
                    </Typography>
                  )}
                </Stack>
              </Box>
            </Tooltip>
          ))}
        </Stack>
      ) : (
        <Typography variant="caption" color="text.disabled" fontStyle="italic">
          No file records
        </Typography>
      )}
    </Box>
  );
}

interface FileInfoCompareProps {
  bookA: DedupBookDetail;
  bookB: DedupBookDetail;
}

export function FileInfoCompare({ bookA, bookB }: FileInfoCompareProps) {
  return (
    <Stack
      direction="row"
      spacing={2}
      divider={<Divider orientation="vertical" flexItem />}
      sx={{ alignItems: 'flex-start' }}
      data-testid="file-info-compare"
    >
      <BookFilesColumn book={bookA} label="Book A" />
      <BookFilesColumn book={bookB} label="Book B" />
    </Stack>
  );
}
