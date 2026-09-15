import React from 'react';
import { Box, Button } from '@mui/material';
import { styled } from '@mui/material/styles';
import { ComposerPrimitive } from '@assistant-ui/react';

const Textarea = styled(ComposerPrimitive.Input)(({ theme }) => ({
  width: '100%',
  minHeight: 80,
  resize: 'vertical',
  padding: theme.spacing(1.5),
  fontFamily: theme.typography.fontFamily,
  fontSize: '1rem',
  lineHeight: 1.5,
  borderRadius: 8,
  border: `1px solid ${theme.palette.border?.neutralDefault || '#D8D8DF'}`,
  backgroundColor: theme.palette.background.paper,
  color: theme.palette.text.primary,
  outline: 'none',
  '&:focus': { borderColor: theme.palette.primary.main },
}));

/**
 * Inline editor shown in place of a user message while it is being edited.
 * Sending rewinds the conversation to this point and re-runs the model.
 */
const EditComposer = () => (
  <ComposerPrimitive.Root asChild>
    <Box component="form" sx={{ px: 6, py: 2, display: 'flex', flexDirection: 'column', gap: 1 }}>
      <Textarea aria-label="Edit message" />
      <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end' }}>
        <ComposerPrimitive.Cancel asChild>
          <Button variant="text" size="small">Cancel</Button>
        </ComposerPrimitive.Cancel>
        <ComposerPrimitive.Send asChild>
          <Button variant="contained" size="small">Save &amp; resend</Button>
        </ComposerPrimitive.Send>
      </Box>
    </Box>
  </ComposerPrimitive.Root>
);

export default EditComposer;
