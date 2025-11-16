// file: web/src/components/audiobooks/AudiobookCard.tsx
// version: 1.0.0
// guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

import React from 'react';
import {
  Card,
  CardContent,
  CardMedia,
  Typography,
  Chip,
  Box,
  IconButton,
  Menu,
  MenuItem,
} from '@mui/material';
import {
  MoreVert as MoreVertIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';
import type { Audiobook } from '../../types';

interface AudiobookCardProps {
  audiobook: Audiobook;
  onEdit?: (audiobook: Audiobook) => void;
  onDelete?: (audiobook: Audiobook) => void;
  onClick?: (audiobook: Audiobook) => void;
}

export const AudiobookCard: React.FC<AudiobookCardProps> = ({
  audiobook,
  onEdit,
  onDelete,
  onClick,
}) => {
  const [anchorEl, setAnchorEl] = React.useState<null | HTMLElement>(null);
  const open = Boolean(anchorEl);

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handleEdit = (event: React.MouseEvent) => {
    event.stopPropagation();
    handleClose();
    onEdit?.(audiobook);
  };

  const handleDelete = (event: React.MouseEvent) => {
    event.stopPropagation();
    handleClose();
    onDelete?.(audiobook);
  };

  const handleCardClick = () => {
    onClick?.(audiobook);
  };

  // Placeholder cover art with first letter of title
  const getCoverPlaceholder = () => {
    return audiobook.title?.charAt(0).toUpperCase() || '?';
  };

  return (
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        cursor: onClick ? 'pointer' : 'default',
        transition: 'transform 0.2s, box-shadow 0.2s',
        '&:hover': {
          transform: onClick ? 'translateY(-4px)' : 'none',
          boxShadow: onClick ? 6 : 1,
        },
      }}
      onClick={handleCardClick}
    >
      <Box sx={{ position: 'relative' }}>
        {audiobook.cover_path ? (
          <CardMedia
            component="img"
            height="240"
            image={audiobook.cover_path}
            alt={audiobook.title || 'Audiobook cover'}
            sx={{ objectFit: 'cover' }}
          />
        ) : (
          <Box
            sx={{
              height: 240,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              bgcolor: 'primary.main',
              color: 'primary.contrastText',
            }}
          >
            <Typography variant="h1" component="div">
              {getCoverPlaceholder()}
            </Typography>
          </Box>
        )}
        <IconButton
          sx={{
            position: 'absolute',
            top: 8,
            right: 8,
            bgcolor: 'background.paper',
            '&:hover': { bgcolor: 'background.default' },
          }}
          onClick={handleMenuClick}
          size="small"
        >
          <MoreVertIcon />
        </IconButton>
      </Box>

      <CardContent sx={{ flexGrow: 1, display: 'flex', flexDirection: 'column' }}>
        <Typography
          gutterBottom
          variant="h6"
          component="h2"
          noWrap
          title={audiobook.title || 'Untitled'}
        >
          {audiobook.title || 'Untitled'}
        </Typography>

        <Typography
          variant="body2"
          color="text.secondary"
          noWrap
          title={audiobook.author || 'Unknown Author'}
        >
          {audiobook.author || 'Unknown Author'}
        </Typography>

        {audiobook.series && (
          <Typography variant="caption" color="text.secondary" noWrap sx={{ mt: 0.5 }}>
            {audiobook.series}
            {audiobook.series_number && ` #${audiobook.series_number}`}
          </Typography>
        )}

        {audiobook.narrator && (
          <Typography variant="caption" color="text.secondary" noWrap sx={{ mt: 0.5 }}>
            Narrated by: {audiobook.narrator}
          </Typography>
        )}

        <Box sx={{ mt: 'auto', pt: 1, display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
          {audiobook.genre && (
            <Chip label={audiobook.genre} size="small" variant="outlined" />
          )}
          {audiobook.language && (
            <Chip label={audiobook.language} size="small" variant="outlined" />
          )}
        </Box>
      </CardContent>

      <Menu anchorEl={anchorEl} open={open} onClose={handleClose}>
        {onEdit && (
          <MenuItem onClick={handleEdit}>
            <EditIcon sx={{ mr: 1 }} fontSize="small" />
            Edit
          </MenuItem>
        )}
        {onDelete && (
          <MenuItem onClick={handleDelete}>
            <DeleteIcon sx={{ mr: 1 }} fontSize="small" />
            Delete
          </MenuItem>
        )}
      </Menu>
    </Card>
  );
};
