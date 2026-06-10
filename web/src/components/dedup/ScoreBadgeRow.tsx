// file: web/src/components/dedup/ScoreBadgeRow.tsx
// version: 1.0.0
// guid: c2b3d4e5-f6a7-8901-bcde-cb2345678901
// last-edited: 2026-06-10

// ScoreBadgeRow renders a compact row of band + score chips for a candidate.
// Used inside the candidate table and the comparison drawer header.

import { Box, Chip, Stack, Tooltip, Typography } from '@mui/material';
import type { DedupBand } from '../../services/api';
import { BAND_CONFIG } from './BandFilterBar';

interface ScoreBadgeRowProps {
  band?: DedupBand | string;
  score?: number;
  layer?: string;
  similarity?: number;
}

const LAYER_COLORS: Record<string, 'error' | 'primary' | 'secondary' | 'default'> = {
  exact: 'error',
  embedding: 'primary',
  llm: 'secondary',
};

export function ScoreBadgeRow({ band, score, layer, similarity }: ScoreBadgeRowProps) {
  const bandCfg = band ? BAND_CONFIG[band as DedupBand] : null;

  return (
    <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap>
      {bandCfg && (
        <Tooltip title={bandCfg.description}>
          <Chip
            label={bandCfg.label}
            size="small"
            color={bandCfg.color}
            variant="filled"
          />
        </Tooltip>
      )}
      {band && !bandCfg && (
        <Chip label={String(band)} size="small" variant="outlined" />
      )}
      {score != null && (
        <Tooltip title={`Composite score: ${score.toFixed(1)} / 100`}>
          <Box
            component="span"
            sx={{
              display: 'inline-flex',
              alignItems: 'center',
              bgcolor: 'action.hover',
              borderRadius: 1,
              px: 0.75,
              py: 0.25,
            }}
          >
            <Typography variant="caption" fontWeight={600}>
              {score.toFixed(0)}
            </Typography>
          </Box>
        </Tooltip>
      )}
      {layer && (
        <Chip
          label={layer}
          size="small"
          color={LAYER_COLORS[layer] ?? 'default'}
          variant="outlined"
        />
      )}
      {similarity != null && score == null && (
        <Typography variant="caption" color="text.secondary">
          {(similarity * 100).toFixed(1)}%
        </Typography>
      )}
    </Stack>
  );
}
