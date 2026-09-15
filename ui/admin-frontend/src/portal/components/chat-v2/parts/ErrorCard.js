import React from 'react';
import { Alert, AlertTitle, Typography } from '@mui/material';

const TITLES = {
  llm_config: 'LLM Configuration Error',
  api: 'API Error',
  connection: 'Connection Error',
  auth: 'Authentication Error',
  filter: 'Blocked by policy',
  tool: 'Tool Error',
  session: 'Session Error',
  internal: 'Error',
};

const SEVERITY = {
  llm_config: 'warning',
  connection: 'warning',
  filter: 'warning',
};

/**
 * Renders an `error` data part from the backend. The code comes from the
 * server (chat_session.ClassifyError) so no string sniffing happens here.
 * Errors are always shown, regardless of the system-message toggle.
 */
const ErrorCard = ({ data, message, severity }) => {
  const code = data?.code || 'internal';
  const text = data?.message || message || 'Something went wrong';
  return (
    <Alert severity={severity || SEVERITY[code] || 'error'} sx={{ my: 1 }} data-testid="error-card">
      <AlertTitle sx={{ fontWeight: 'bold' }}>{TITLES[code] || 'Error'}</AlertTitle>
      <Typography variant="body2">{text}</Typography>
      {data?.detail && (
        <Typography variant="body2" color="text.secondary" sx={{ fontSize: '0.85em', mt: 0.5 }}>
          Details: {data.detail}
        </Typography>
      )}
    </Alert>
  );
};

export default ErrorCard;
