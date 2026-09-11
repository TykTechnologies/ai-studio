import React, { useEffect, useState } from 'react';
import { Dialog, DialogActions, DialogContent, DialogTitle, TextField, Typography } from '@mui/material';
import { PrimaryButton, SecondaryOutlineButton } from '../../styles/sharedStyles';

/**
 * Asks for the name of a role copy. System roles cannot be edited, so this
 * is how they are customised.
 */
const CloneRoleDialog = ({ open, role, onConfirm, onCancel, busy = false }) => {
  const sourceName = role?.attributes?.name || role?.name || '';
  const [name, setName] = useState('');

  useEffect(() => {
    if (open) setName(sourceName ? `Copy of ${sourceName}` : '');
  }, [open, sourceName]);

  return (
    <Dialog open={open} onClose={onCancel} fullWidth maxWidth="xs">
      <DialogTitle>Clone role</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          The copy starts with the same permissions as {sourceName || 'the role'} and can be edited freely.
        </Typography>
        <TextField
          autoFocus
          fullWidth
          size="small"
          label="New role name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          inputProps={{ 'data-testid': 'clone-role-name' }}
        />
      </DialogContent>
      <DialogActions>
        <SecondaryOutlineButton onClick={onCancel} disabled={busy}>Cancel</SecondaryOutlineButton>
        <PrimaryButton onClick={() => onConfirm(name.trim())} disabled={busy || !name.trim()}>
          Clone
        </PrimaryButton>
      </DialogActions>
    </Dialog>
  );
};

export default CloneRoleDialog;
