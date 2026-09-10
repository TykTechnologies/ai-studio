import React, { useState } from 'react';
import { Box, Chip, IconButton, Stack, TextField } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';

/**
 * Chip-list editor for string_list fields (mirrors the LLM form's allowed
 * models input). Emits a string[]; Enter or the add button appends.
 */
const StringListInput = ({ id, label, value = [], onChange, error, helperText, required, disabled }) => {
  const [draft, setDraft] = useState('');
  const items = Array.isArray(value) ? value : [];

  const add = () => {
    const next = draft.trim();
    if (!next || items.includes(next)) {
      setDraft('');
      return;
    }
    onChange([...items, next]);
    setDraft('');
  };

  const remove = (item) => onChange(items.filter((i) => i !== item));

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
        <TextField
          id={id}
          fullWidth
          label={label}
          value={draft}
          required={required && items.length === 0}
          disabled={disabled}
          error={Boolean(error)}
          helperText={helperText}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              add();
            }
          }}
          inputProps={{ 'aria-label': label }}
        />
        <IconButton aria-label={`Add ${label}`} onClick={add} disabled={disabled || !draft.trim()} sx={{ mt: 1 }}>
          <AddIcon />
        </IconButton>
      </Box>
      {items.length > 0 && (
        <Stack direction="row" spacing={1} sx={{ mt: 1, flexWrap: 'wrap', gap: 0.5 }}>
          {items.map((item) => (
            <Chip key={item} label={item} onDelete={disabled ? undefined : () => remove(item)} size="small" />
          ))}
        </Stack>
      )}
    </Box>
  );
};

export default StringListInput;
