import React, { useMemo, useState } from 'react';
import { Box, Collapse, IconButton, Tooltip, Typography } from '@mui/material';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import ReplayIcon from '@mui/icons-material/Replay';
import PsychologyIcon from '@mui/icons-material/Psychology';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { ActionBarPrimitive, MessagePrimitive, useAuiState } from '@assistant-ui/react';
import MarkdownText from './MarkdownText';
import ToolCallCard from './parts/ToolCallCard';
import ContextBlock from './parts/ContextBlock';
import StatusChip from './parts/StatusChip';
import ErrorCard from './parts/ErrorCard';
import { useToolUiRegistry } from './toolUiRegistry';

const Avatar = () => (
  <Box
    sx={{
      width: 35,
      height: 35,
      borderRadius: '50%',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      flexShrink: 0,
      bgcolor: 'background.surfaceDefaultBubble',
    }}
  >
    <Typography variant="bodyLargeDefault" color="text.defaultSubdued">AI</Typography>
  </Box>
);

const Reasoning = ({ text }) => {
  const [open, setOpen] = useState(false);
  if (!text) return null;
  return (
    <Box sx={{ my: 1, opacity: 0.85 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, cursor: 'pointer' }} onClick={() => setOpen((v) => !v)}>
        <PsychologyIcon fontSize="small" color="disabled" />
        <Typography variant="bodySmallDefault" color="text.secondary" fontWeight={500}>Thinking</Typography>
        <ExpandMoreIcon fontSize="small" sx={{ transform: open ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s' }} />
      </Box>
      <Collapse in={open}>
        <Typography variant="bodySmallDefault" color="text.secondary" sx={{ fontStyle: 'italic', whiteSpace: 'pre-wrap', mt: 0.5 }}>
          {text}
        </Typography>
      </Collapse>
    </Box>
  );
};

const MessageError = () => {
  const error = useAuiState((s) => {
    const st = s.message.status;
    return st?.type === 'incomplete' && st.reason === 'error' ? st.error : null;
  });
  if (!error) return null;
  const text = typeof error === 'string' ? error : error?.message || JSON.stringify(error);
  return <ErrorCard data={{ code: 'internal', message: text }} />;
};

const dataComponents = {
  by_name: { status: StatusChip, context: ContextBlock, error: ErrorCard },
  Fallback: () => null,
};

/** An assistant turn: markdown text, reasoning, tool cards, context/status/error parts. */
const AssistantMessage = () => {
  const registry = useToolUiRegistry();
  const components = useMemo(
    () => ({
      Text: MarkdownText,
      Reasoning,
      tools: { by_name: registry, Fallback: ToolCallCard },
      data: dataComponents,
    }),
    [registry],
  );

  return (
    <MessagePrimitive.Root asChild>
      <Box sx={{ display: 'flex', gap: 2, py: 2, alignItems: 'flex-start' }} data-role="assistant">
        <Avatar />
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <MessagePrimitive.Parts components={components} />
          <MessageError />
          <ActionBarPrimitive.Root hideWhenRunning autohide="not-last" asChild>
            <Box sx={{ display: 'flex', gap: 0.5, mt: 0.5, opacity: 0.7 }}>
              <ActionBarPrimitive.Copy asChild>
                <Tooltip title="Copy">
                  <IconButton size="small" aria-label="Copy message"><ContentCopyIcon fontSize="small" /></IconButton>
                </Tooltip>
              </ActionBarPrimitive.Copy>
              <ActionBarPrimitive.Reload asChild>
                <Tooltip title="Regenerate">
                  <IconButton size="small" aria-label="Regenerate response"><ReplayIcon fontSize="small" /></IconButton>
                </Tooltip>
              </ActionBarPrimitive.Reload>
            </Box>
          </ActionBarPrimitive.Root>
        </Box>
      </Box>
    </MessagePrimitive.Root>
  );
};

export default AssistantMessage;
