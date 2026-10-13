import React, { useState } from 'react';
import { Alert, AlertTitle, Button, IconButton, Snackbar } from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { useOptionalSyncStatus } from '../../context/SyncStatusContext';
import PushConfigurationModal from '../edge-gateways/PushConfigurationModal';

/**
 * Shown right after saving something the edge gateways consume (an LLM,
 * filter, tool, data source, app, catalogue, plugin...) while edges are out
 * of date: the change is saved but not live on them until it is pushed
 * (#506). Which requests count comes from configSyncNotifier (the same list
 * that refreshes sync status); whether edges need a push comes from the sync
 * status itself. It sits bottom-right so it does not cover the form's own
 * bottom-centre "saved" message.
 */
const PushReminderToast = () => {
  const sync = useOptionalSyncStatus();
  const [pushOpen, setPushOpen] = useState(false);
  if (!sync) return null;
  const { pushReminder, dismissPushReminder } = sync;

  const handleClose = (event, reason) => {
    if (reason === 'clickaway') return;
    dismissPushReminder();
  };

  return (
    <>
      <Snackbar
        key={pushReminder?.id}
        open={!!pushReminder}
        autoHideDuration={12000}
        onClose={handleClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      >
        <Alert
          severity="info"
          variant="filled"
          data-testid="push-reminder"
          action={
            <>
              <Button
                color="inherit"
                size="small"
                onClick={() => {
                  dismissPushReminder();
                  setPushOpen(true);
                }}
              >
                Push Configuration
              </Button>
              <IconButton color="inherit" size="small" aria-label="Dismiss" onClick={dismissPushReminder}>
                <CloseIcon fontSize="small" />
              </IconButton>
            </>
          }
        >
          <AlertTitle>Saved, not live on edge gateways yet</AlertTitle>
          Edge gateways keep using the previous configuration until you push it.
        </Alert>
      </Snackbar>
      <PushConfigurationModal open={pushOpen} onClose={() => setPushOpen(false)} />
    </>
  );
};

export default PushReminderToast;
