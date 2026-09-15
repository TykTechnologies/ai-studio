import React from 'react';
import { Box, IconButton, Tooltip, Typography } from '@mui/material';
import EditIcon from '@mui/icons-material/Edit';
import { ActionBarPrimitive, MessagePrimitive } from '@assistant-ui/react';
import ContextBlock from './parts/ContextBlock';
import AttachmentChip from './parts/AttachmentChip';
import { useChatUi } from './ChatUiContext';

const PlainText = ({ text }) => (
  <Typography sx={{ fontSize: '1rem', whiteSpace: 'pre-wrap', overflowWrap: 'break-word' }} component="div">
    {text}
  </Typography>
);

const components = {
  Text: PlainText,
  data: { by_name: { context: ContextBlock }, Fallback: () => null },
};

const attachmentComponents = { Attachment: () => <AttachmentChip removable={false} /> };

/** A user turn: right-aligned bubble with edit-on-hover, attachments and context parts. */
const UserMessage = () => {
  const { userName } = useChatUi();
  return (
    <MessagePrimitive.Root asChild>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'flex-end',
          alignItems: 'flex-start',
          gap: 1,
          py: 2,
          '&:hover .edit-button': { opacity: 1, visibility: 'visible' },
        }}
        data-role="user"
      >
        <ActionBarPrimitive.Root hideWhenRunning asChild>
          <Box sx={{ alignSelf: 'center' }}>
            <ActionBarPrimitive.Edit asChild>
              <Tooltip title="Edit message">
                <IconButton
                  size="small"
                  className="edit-button"
                  aria-label="Edit message"
                  sx={{ opacity: 0, visibility: 'hidden', transition: 'opacity 0.2s ease-in-out' }}
                >
                  <EditIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </ActionBarPrimitive.Edit>
          </Box>
        </ActionBarPrimitive.Root>
        <Box
          sx={{
            maxWidth: '75%',
            bgcolor: 'background.surfaceNeutralDisabled',
            border: '1px solid',
            borderColor: 'border.neutralDefault',
            borderRadius: '8px',
            p: 1.5,
            minWidth: 0,
          }}
        >
          <MessagePrimitive.Parts components={components} />
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 0.5 }}>
            <MessagePrimitive.Attachments components={attachmentComponents} />
          </Box>
        </Box>
        <Box
          sx={{
            width: 35,
            height: 35,
            borderRadius: '50%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
            bgcolor: 'background.surfaceBrandHovered',
          }}
        >
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            {userName?.charAt(0)?.toUpperCase() || 'U'}
          </Typography>
        </Box>
      </Box>
    </MessagePrimitive.Root>
  );
};

export default UserMessage;
