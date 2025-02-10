import React from 'react';
import {
  Box,
  Typography,
  TextField,
  Alert,
  IconButton,
  Tooltip
} from '@mui/material';
import InfoIcon from '@mui/icons-material/Info';

const ConfigureProvider = ({
  provider,
  config,
  onConfigChange,
  loading,
  error
}) => {
  const handleChange = (field) => (event) => {
    onConfigChange({
      ...config,
      [field]: event.target.value
    });
  };

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Configure {provider?.name}
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <TextField
        fullWidth
        label="URL"
        value={config.url}
        onChange={handleChange('url')}
        margin="normal"
        helperText={provider?.name === "Tyk Dashboard" 
          ? "Enter your Tyk Dashboard URL (e.g., http://localhost:3000)" 
          : "Enter the provider URL"}
        autoComplete="off"
      />

      <Box sx={{ display: 'flex', alignItems: 'flex-start' }}>
        <TextField
          fullWidth
          label="Access Token or Secret Reference"
          value={config.token}
          onChange={handleChange('token')}
          margin="normal"
          helperText={provider?.name === "Tyk Dashboard" 
            ? "Enter your Tyk Dashboard API token or use a secret reference (e.g., $SECRET/DashboardKey)" 
            : "Enter your access token or use a secret reference (e.g., $SECRET/MySecret)"}
          autoComplete="off"
        />
        <Tooltip title="You can use a secret reference in the format $SECRET/SecretName to securely store and use your access token" placement="top">
          <IconButton sx={{ mt: 2.5, ml: 1 }}>
            <InfoIcon color="info" />
          </IconButton>
        </Tooltip>
      </Box>
    </Box>
  );
};

export default ConfigureProvider;
