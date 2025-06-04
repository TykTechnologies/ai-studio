import React, { memo } from 'react';
import { Snackbar, Alert } from '@mui/material';
import PropTypes from 'prop-types';

const AlertSnackbar = memo(({ open, message, severity, onClose }) => {
  return (
    <Snackbar
      open={open}
      autoHideDuration={6000}
      onClose={onClose}
      anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
    >
      <Alert
        onClose={onClose}
        severity={severity}
        sx={{ width: "100%" }}
      >
        {message}
      </Alert>
    </Snackbar>
  );
});

AlertSnackbar.propTypes = {
  open: PropTypes.bool.isRequired,
  message: PropTypes.string.isRequired,
  severity: PropTypes.oneOf(['success', 'info', 'warning', 'error']),
  onClose: PropTypes.func.isRequired
};

AlertSnackbar.defaultProps = {
  severity: 'success'
};

AlertSnackbar.displayName = 'AlertSnackbar';

export default AlertSnackbar;