import { useState, useCallback } from 'react';

export const useSnackbarState = () => {
  const [snackbarState, setSnackbarState] = useState({
    open: false,
    message: '',
    severity: 'success'
  });

  const showSnackbar = useCallback((message, severity = 'success') => {
    setSnackbarState({
      open: true,
      message,
      severity
    });
  }, []);

  const hideSnackbar = useCallback((event, reason) => {
    if (reason === 'clickaway') {
      return;
    }
    setSnackbarState(prev => ({ ...prev, open: false }));
  }, []);

  return {
    snackbarState,
    showSnackbar,
    hideSnackbar
  };
};