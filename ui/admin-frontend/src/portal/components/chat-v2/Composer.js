import React from 'react';
import { Box, IconButton, Tooltip } from '@mui/material';
import { styled } from '@mui/material/styles';
import SendIcon from '@mui/icons-material/Send';
import StopIcon from '@mui/icons-material/Stop';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import { ComposerPrimitive, ThreadPrimitive } from '@assistant-ui/react';
import AttachmentChip from './parts/AttachmentChip';

const Textarea = styled(ComposerPrimitive.Input)(({ theme }) => ({
  width: '100%',
  minHeight: 60,
  maxHeight: 260,
  resize: 'none',
  border: 'none',
  outline: 'none',
  padding: theme.spacing(1.5),
  paddingRight: theme.spacing(14),
  fontFamily: theme.typography.fontFamily,
  fontSize: '1rem',
  lineHeight: 1.5,
  backgroundColor: 'transparent',
  color: theme.palette.text.primary,
  '&::placeholder': { color: theme.palette.text.secondary },
}));

const attachmentComponents = { Attachment: () => <AttachmentChip /> };

/**
 * The message composer: gradient-bordered box (as v1), autosizing textarea
 * (Enter sends, Shift+Enter inserts a newline), file attachments via button
 * or drag-and-drop, send / stop buttons driven by the runtime.
 */
const Composer = ({ hideFileUpload = false, placeholder }) => (
  <ComposerPrimitive.Root asChild>
    <Box component="form" sx={{ mb: 2, mx: '1px', position: 'relative' }}>
      <ComposerPrimitive.AttachmentDropzone asChild disabled={hideFileUpload}>
        <Box
          sx={{
            // 1px gradient frame: a padded wrapper around an opaque inner box,
            // so the interior always paints above the gradient in every browser
            // (the masked pseudo-element approach hid the typed text).
            p: '1px',
            borderRadius: '8px',
            background: 'linear-gradient(163.33deg, #23E2C2 46.22%, #5900CB 161.35%)',
          }}
        >
        <Box sx={{ position: 'relative', borderRadius: '7px', bgcolor: 'background.paper', minHeight: 90 }}>
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, px: 1.5, pt: 1 }}>
            <ComposerPrimitive.Attachments components={attachmentComponents} />
          </Box>
          <Textarea
            placeholder={placeholder || 'Type your message here... (Enter to send, Shift+Enter for new line)'}
            aria-label="Message"
          />
          <Box sx={{ position: 'absolute', right: 8, bottom: 8, display: 'flex', alignItems: 'center', gap: 0.5 }}>
            {!hideFileUpload && (
              <ComposerPrimitive.AddAttachment asChild multiple>
                <Tooltip title="Attach files">
                  <IconButton size="small" aria-label="Attach files"><AttachFileIcon /></IconButton>
                </Tooltip>
              </ComposerPrimitive.AddAttachment>
            )}
            <ThreadPrimitive.If running={false}>
              <ComposerPrimitive.Send asChild>
                <IconButton size="small" aria-label="Send message" color="primary"><SendIcon /></IconButton>
              </ComposerPrimitive.Send>
            </ThreadPrimitive.If>
            <ThreadPrimitive.If running>
              <ComposerPrimitive.Cancel asChild>
                <Tooltip title="Stop generating">
                  <IconButton size="small" aria-label="Stop generating" color="error"><StopIcon /></IconButton>
                </Tooltip>
              </ComposerPrimitive.Cancel>
            </ThreadPrimitive.If>
          </Box>
        </Box>
        </Box>
      </ComposerPrimitive.AttachmentDropzone>
    </Box>
  </ComposerPrimitive.Root>
);

export default Composer;
