// file: web/src/components/audiobooks/TagEditor.tsx
// version: 1.0.0
// guid: b3c4d5e6-f7a8-4b9c-0d1e-2f3a4b5c6d7e

import React, { useState, useRef } from 'react';
import {
  Chip,
  Autocomplete,
  TextField,
  Box,
  Popover,
  Button,
  Stack,
} from '@mui/material';
import { Add as AddIcon } from '@mui/icons-material';
import * as api from '../../services/api';

interface TagEditorProps {
  bookId: string;
  tags: string[];
  allTags?: string[];
  onTagsChange?: (tags: string[]) => void;
  readOnly?: boolean;
  compact?: boolean;
}

export const TagEditor: React.FC<TagEditorProps> = ({
  bookId,
  tags,
  allTags = [],
  onTagsChange,
  readOnly = false,
  compact = false,
}) => {
  const [inputValue, setInputValue] = useState('');
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const addButtonRef = useRef<HTMLButtonElement>(null);

  const availableSuggestions = allTags.filter(
    (t) => !tags.includes(t.toLowerCase())
  );

  const handleAddTag = async (tag: string) => {
    const normalized = tag.trim().toLowerCase();
    if (!normalized || tags.includes(normalized)) return;
    try {
      const updatedTags = await api.addBookUserTag(bookId, normalized);
      onTagsChange?.(updatedTags);
    } catch (_err) {
      console.error('Failed to add tag:', _err);
    }
    setInputValue('');
  };

  const handleRemoveTag = async (tag: string) => {
    try {
      const updatedTags = await api.removeBookUserTag(bookId, tag);
      onTagsChange?.(updatedTags);
    } catch (_err) {
      console.error('Failed to remove tag:', _err);
    }
  };

  const popoverOpen = Boolean(anchorEl);

  if (compact) {
    return (
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, alignItems: 'center' }}>
        {tags.map((tag) => (
          <Chip
            key={tag}
            label={tag}
            size="small"
            variant="outlined"
            onDelete={readOnly ? undefined : () => handleRemoveTag(tag)}
          />
        ))}
        {!readOnly && (
          <>
            <Button
              ref={addButtonRef}
              size="small"
              variant="text"
              sx={{ minWidth: 'auto', px: 0.5, fontSize: '0.75rem' }}
              onClick={(e) => setAnchorEl(e.currentTarget)}
            >
              <AddIcon sx={{ fontSize: 16 }} />
              Tag
            </Button>
            <Popover
              open={popoverOpen}
              anchorEl={anchorEl}
              onClose={() => {
                setAnchorEl(null);
                setInputValue('');
              }}
              anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            >
              <Box sx={{ p: 1, minWidth: 200 }}>
                <Autocomplete
                  freeSolo
                  size="small"
                  options={availableSuggestions}
                  inputValue={inputValue}
                  onInputChange={(_e, value) => setInputValue(value)}
                  onChange={(_e, value) => {
                    if (typeof value === 'string') {
                      handleAddTag(value);
                    }
                    setAnchorEl(null);
                  }}
                  renderInput={(params) => (
                    <TextField
                      {...params}
                      placeholder="Add tag..."
                      autoFocus
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' && inputValue.trim()) {
                          e.preventDefault();
                          handleAddTag(inputValue);
                          setAnchorEl(null);
                        }
                      }}
                    />
                  )}
                />
              </Box>
            </Popover>
          </>
        )}
      </Box>
    );
  }

  return (
    <Stack spacing={1}>
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
        {tags.map((tag) => (
          <Chip
            key={tag}
            label={tag}
            size="small"
            variant="outlined"
            onDelete={readOnly ? undefined : () => handleRemoveTag(tag)}
          />
        ))}
      </Box>
      {!readOnly && (
        <Autocomplete
          freeSolo
          size="small"
          options={availableSuggestions}
          inputValue={inputValue}
          onInputChange={(_e, value) => setInputValue(value)}
          onChange={(_e, value) => {
            if (typeof value === 'string') {
              handleAddTag(value);
            }
          }}
          renderInput={(params) => (
            <TextField
              {...params}
              placeholder="Add tag..."
              onKeyDown={(e) => {
                if (e.key === 'Enter' && inputValue.trim()) {
                  e.preventDefault();
                  handleAddTag(inputValue);
                }
              }}
            />
          )}
        />
      )}
    </Stack>
  );
};
