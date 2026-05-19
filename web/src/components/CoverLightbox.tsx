// file: web/src/components/CoverLightbox.tsx
// version: 1.0.0
// guid: 7f8e9a0b-1c2d-3e4f-5a6b-7c8d9e0f1a2b

import { Modal, Box, IconButton } from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import MusicNoteIcon from '@mui/icons-material/MusicNote';

export interface CoverLightboxProps {
  open: boolean;
  src: string | null;
  onClose: () => void;
}

export function CoverLightbox({ open, src, onClose }: CoverLightboxProps) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backdropFilter: 'blur(4px)',
      }}
    >
      <Box
        sx={{
          position: 'relative',
          bgcolor: 'background.paper',
          borderRadius: 1,
          p: 2,
          maxWidth: '90vw',
          maxHeight: '90vh',
          overflow: 'auto',
        }}
      >
        {/* Close button */}
        <IconButton
          aria-label="Close"
          onClick={onClose}
          sx={{
            position: 'absolute',
            right: 8,
            top: 8,
            bgcolor: 'background.paper',
            '&:hover': { bgcolor: 'action.hover' },
            zIndex: 1,
          }}
        >
          <CloseIcon />
        </IconButton>

        {/* Image or placeholder */}
        {src ? (
          <img
            src={src}
            alt="Cover preview"
            style={{
              maxWidth: '100%',
              maxHeight: '100%',
              display: 'block',
            }}
          />
        ) : (
          <Box
            data-testid="cover-placeholder"
            sx={{
              width: 400,
              height: 500,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              bgcolor: 'action.disabledBackground',
              borderRadius: 1,
            }}
          >
            <MusicNoteIcon sx={{ fontSize: 80, opacity: 0.3 }} />
          </Box>
        )}
      </Box>
    </Modal>
  );
}
