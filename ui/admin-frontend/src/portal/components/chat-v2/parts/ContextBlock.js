import React, { useState } from 'react';
import { Box, Collapse, Typography } from '@mui/material';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import { useChatUi } from '../ChatUiContext';

const LABELS = {
  rag: 'Retrieved context',
  tool_docs: 'Tool documentation',
  file: 'Uploaded file',
};

/**
 * Renders a `context` data part (RAG hits, uploaded files, tool docs) as a
 * collapsed monospace block, matching the v1 [CONTEXT] presentation. Hidden
 * when system messages are switched off.
 */
const ContextBlock = ({ data }) => {
  const { showSystemMessages } = useChatUi();
  const [open, setOpen] = useState(false);
  const text = data?.text || '';
  if (!showSystemMessages || !text.trim()) return null;

  return (
    <Box
      sx={{
        bgcolor: '#F5F5F5',
        border: '1px solid #e9ecef',
        borderRadius: '10px',
        p: 1.5,
        my: 1,
        color: '#666',
        fontFamily: 'monospace',
        cursor: 'pointer',
      }}
      onClick={() => setOpen((v) => !v)}
      data-testid="context-block"
    >
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography variant="caption" sx={{ fontWeight: 'bold', color: '#666' }}>
          {LABELS[data?.source] || 'CONTEXT'}
        </Typography>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography variant="caption">{open ? 'Click to collapse' : 'Click to show context'}</Typography>
          <KeyboardArrowDownIcon sx={{ transform: open ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s' }} />
        </Box>
      </Box>
      <Collapse in={open}>
        <Box component="pre" sx={{ mt: 1, whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: '0.8rem', m: 0 }}>
          {text}
        </Box>
      </Collapse>
    </Box>
  );
};

export default ContextBlock;
