import React, { useEffect, useState } from 'react';
import { Snackbar, Alert } from '@mui/material';
import { subscribePermissionDenied } from '../../utils/permissionDeniedBus';
import { permissionLabel } from '../../rbac/permissions';

/**
 * One snackbar for every denied mutation, mounted once in the admin layout.
 */
const PermissionDeniedToaster = () => {
  const [error, setError] = useState(null);

  useEffect(() => subscribePermissionDenied((err) => setError(err)), []);

  const label = error?.requiredPermission ? permissionLabel(error.requiredPermission) : '';
  const message = label
    ? `You don't have permission to do that (requires ${label}).`
    : "You don't have permission to do that.";

  return (
    <Snackbar
      open={Boolean(error)}
      autoHideDuration={6000}
      onClose={() => setError(null)}
      anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
    >
      <Alert severity="warning" onClose={() => setError(null)} data-testid="permission-denied-toast">
        {message}
      </Alert>
    </Snackbar>
  );
};

export default PermissionDeniedToaster;
