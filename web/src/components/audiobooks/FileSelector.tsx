// file: web/src/components/audiobooks/FileSelector.tsx
// version: 1.0.0
// guid: 8f2a3b4c-5d6e-7f8a-9b0c-1d2e3f4a5b6c

import { Box, Chip, MenuItem, Select, type SelectChangeEvent } from '@mui/material';
import type { BookSegment } from '../../services/api';

interface FileSelectorProps {
  segments: BookSegment[];
  selectedId: string | null;
  onSelect: (id: string | null) => void;
}

/**
 * Truncate a filename to maxLen characters, keeping the extension visible.
 */
function truncateBasename(filePath: string, maxLen = 26): string {
  const basename = filePath.split('/').pop() || filePath;
  if (basename.length <= maxLen) return basename;
  const extIdx = basename.lastIndexOf('.');
  const ext = extIdx >= 0 ? basename.slice(extIdx) : '';
  const nameOnly = extIdx >= 0 ? basename.slice(0, extIdx) : basename;
  const available = maxLen - ext.length - 1; // 1 for ellipsis char
  if (available <= 3) return basename.slice(0, maxLen - 1) + '\u2026';
  return nameOnly.slice(0, available) + '\u2026' + ext;
}

/**
 * Build chip label: zero-padded track number + truncated basename.
 */
function chipLabel(seg: BookSegment, totalDigits: number): string {
  const trackStr = seg.track_number != null
    ? String(seg.track_number).padStart(totalDigits, '0') + ' '
    : '';
  return trackStr + truncateBasename(seg.file_path);
}

export const FileSelector = ({ segments, selectedId, onSelect }: FileSelectorProps) => {
  // Hide for single-file books
  if (segments.length <= 1) return null;

  const totalDigits = Math.max(2, String(segments.length).length);

  // > 20 segments: dropdown only
  if (segments.length > 20) {
    const handleChange = (event: SelectChangeEvent<string>) => {
      const val = event.target.value;
      onSelect(val === '__all__' ? null : val);
    };

    return (
      <Box sx={{ mb: 2 }}>
        <Select
          size="small"
          value={selectedId ?? '__all__'}
          onChange={handleChange}
          sx={{ minWidth: 300 }}
          displayEmpty
        >
          <MenuItem value="__all__">All Files</MenuItem>
          {segments.map((seg) => (
            <MenuItem key={seg.id} value={seg.id}>
              {chipLabel(seg, totalDigits)}
            </MenuItem>
          ))}
        </Select>
      </Box>
    );
  }

  // 2-20 segments: horizontal scrollable chip strip
  return (
    <Box
      sx={{
        mb: 2,
        display: 'flex',
        flexWrap: 'nowrap',
        overflowX: 'auto',
        gap: 1,
        pb: 0.5,
        '&::-webkit-scrollbar': { height: 4 },
        '&::-webkit-scrollbar-thumb': { bgcolor: 'divider', borderRadius: 2 },
      }}
    >
      <Chip
        label="All Files"
        variant={selectedId === null ? 'filled' : 'outlined'}
        color={selectedId === null ? 'primary' : 'default'}
        onClick={() => onSelect(null)}
      />
      {segments.map((seg) => (
        <Chip
          key={seg.id}
          label={chipLabel(seg, totalDigits)}
          variant={selectedId === seg.id ? 'filled' : 'outlined'}
          color={selectedId === seg.id ? 'primary' : 'default'}
          onClick={() => onSelect(selectedId === seg.id ? null : seg.id)}
          sx={{ flexShrink: 0 }}
        />
      ))}
    </Box>
  );
};
