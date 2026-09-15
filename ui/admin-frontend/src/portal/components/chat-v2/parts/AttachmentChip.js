import React from 'react';
import { Chip } from '@mui/material';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import { AttachmentPrimitive, useAuiState } from '@assistant-ui/react';

/** A file attached to the composer (removable) or to a sent message. */
const AttachmentChip = ({ removable = true }) => {
  const name = useAuiState((s) => s.attachment.name);
  const status = useAuiState((s) => s.attachment.status?.type);
  const chip = (
    <Chip
      icon={<AttachFileIcon />}
      label={name}
      size="small"
      variant="outlined"
      color={status === 'incomplete' ? 'error' : 'default'}
      sx={{ maxWidth: 240 }}
    />
  );
  if (!removable) {
    return <AttachmentPrimitive.Root>{chip}</AttachmentPrimitive.Root>;
  }
  return (
    <AttachmentPrimitive.Root>
      <AttachmentPrimitive.Remove asChild>
        <Chip
          icon={<AttachFileIcon />}
          label={name}
          size="small"
          variant="outlined"
          onDelete={() => {}}
          sx={{ maxWidth: 240 }}
        />
      </AttachmentPrimitive.Remove>
    </AttachmentPrimitive.Root>
  );
};

export default AttachmentChip;
