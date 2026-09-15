import React from 'react';
import { Box, Typography } from '@mui/material';
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined';
import { useChatUi } from '../ChatUiContext';

/**
 * Renders a `status` data part (governance filters running, tool added,
 * guideline violation…) as the small system line v1 showed. Hidden when
 * system messages are switched off.
 */
const StatusChip = ({ data }) => {
  const { showSystemMessages } = useChatUi();
  if (!showSystemMessages || !data?.text) return null;
  const warn = data.level === 'warn';
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        my: 0.75,
        px: 1.5,
        py: 0.75,
        borderRadius: '8px',
        bgcolor: warn ? 'background.surfaceWarningDefault' : '#E0F7F6',
        border: '1px solid',
        borderColor: warn ? 'border.warningDefaultSubdued' : '#e9ecef',
        fontFamily: 'monospace',
      }}
      data-testid="status-chip"
    >
      <SmartToyOutlinedIcon sx={{ fontSize: '1rem', color: '#666' }} />
      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
        {data.text}
      </Typography>
    </Box>
  );
};

export default StatusChip;
