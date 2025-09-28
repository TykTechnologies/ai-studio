import React, { useState } from 'react';
import {
  Box,
  Typography,
  Alert,
} from '@mui/material';
import { PrimaryButton, SecondaryOutlineButton } from '../../../styles/sharedStyles';
import PluginConfigurationSection from '../PluginConfigurationSection';

const Step3Configuration = ({ pluginId, pluginData, configSchema, onComplete, onBack, loading, disabled }) => {
  const [config, setConfig] = useState({});
  const [configError, setConfigError] = useState(null);

  const handleConfigChange = (configData) => {
    setConfigError(null);

    if (typeof configData === 'string') {
      // Handle raw JSON string (from JSON editor)
      try {
        const parsed = JSON.parse(configData);
        setConfig(parsed);
      } catch (err) {
        setConfigError('Invalid JSON format');
        return;
      }
    } else {
      // Handle parsed config object (from schema form)
      setConfig(configData || {});
    }
  };

  const handleComplete = () => {
    if (configError) {
      return;
    }
    onComplete(config);
  };

  const canProceed = !configError;

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Plugin Configuration
      </Typography>

      <Typography variant="body2" color="textSecondary" sx={{ mb: 3 }}>
        Configure your plugin with the required settings. You can use the form below if a configuration schema is available,
        or enter JSON directly in the editor.
      </Typography>

      {/* Success Alert */}
      <Alert severity="success" sx={{ mb: 3 }}>
        <Typography variant="body2">
          Plugin <strong>{pluginData.name}</strong> has been created successfully and security permissions have been approved.
          Complete the configuration to finalize the setup.
        </Typography>
      </Alert>

      {/* Configuration Section */}
      <PluginConfigurationSection
        pluginId={pluginId}
        config={config}
        onConfigChange={handleConfigChange}
        isEdit={true} // Enable schema loading
        configError={configError}
      />

      {/* Action Buttons */}
      <Box sx={{ mt: 4, display: 'flex', justifyContent: 'space-between' }}>
        <SecondaryOutlineButton onClick={onBack} disabled={disabled || loading}>
          Back
        </SecondaryOutlineButton>

        <PrimaryButton
          onClick={handleComplete}
          disabled={disabled || loading || !canProceed}
        >
          {loading ? 'Saving...' : 'Complete Setup'}
        </PrimaryButton>
      </Box>
    </Box>
  );
};

export default Step3Configuration;