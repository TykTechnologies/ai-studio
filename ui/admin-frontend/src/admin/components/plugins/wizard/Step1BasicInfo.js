import React, { useState } from 'react';
import {
  TextField,
  Box,
  Typography,
  Grid,
} from '@mui/material';
import { PrimaryButton, SecondaryOutlineButton } from '../../../styles/sharedStyles';
import usePluginPolicy from '../../../hooks/usePluginPolicy';
import {
  allowedPluginSources,
  pluginSourceError,
  pluginCommandHelp,
  pluginCommandPlaceholder,
} from '../../../services/pluginService';

const Step1BasicInfo = ({ data, onComplete, onBack, loading, disabled }) => {
  const { policy } = usePluginPolicy();
  const allowedSources = allowedPluginSources(policy);
  const [formData, setFormData] = useState({
    name: data.name || '',
    description: data.description || '',
    command: data.command || '',
  });

  const [errors, setErrors] = useState({});

  const handleInputChange = (field) => (event) => {
    const value = event.target.value;
    setFormData(prev => ({ ...prev, [field]: value }));

    // Clear error when user starts typing
    if (errors[field]) {
      setErrors(prev => ({ ...prev, [field]: null }));
    }
  };

  const validateForm = () => {
    const newErrors = {};

    if (!formData.name.trim()) {
      newErrors.name = 'Plugin name is required';
    }

    if (!formData.command.trim()) {
      newErrors.command = 'Plugin command is required';
    } else {
      const sourceError = pluginSourceError(formData.command, allowedSources);
      if (sourceError) {
        newErrors.command = sourceError;
      }
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleNext = () => {
    if (validateForm()) {
      const dataToPass = {
        name: formData.name,
        description: formData.description,
        command: formData.command,
      };
      console.log('Step1 passing data:', dataToPass);
      onComplete(dataToPass);
    }
  };

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Basic Plugin Information
      </Typography>

      <Typography variant="body2" color="textSecondary" sx={{ mb: 3 }}>
        Enter the basic details for your plugin. The system will load the plugin and detect its capabilities automatically.
      </Typography>

      <Grid container spacing={3}>
        <Grid item xs={12} md={6}>
          <TextField
            fullWidth
            label="Plugin Name"
            value={formData.name}
            onChange={handleInputChange('name')}
            error={!!errors.name}
            helperText={errors.name || 'Display name for the plugin'}
            disabled={disabled}
            required
          />
        </Grid>

        <Grid item xs={12}>
          <TextField
            fullWidth
            label="Description"
            value={formData.description}
            onChange={handleInputChange('description')}
            multiline
            rows={2}
            helperText="Optional description of what this plugin does"
            disabled={disabled}
          />
        </Grid>

        <Grid item xs={12}>
          <TextField
            fullWidth
            label="Command"
            value={formData.command}
            onChange={handleInputChange('command')}
            error={!!errors.command}
            helperText={
              errors.command ||
              pluginCommandHelp(allowedSources, Boolean(policy?.oci_require_signature))
            }
            placeholder={pluginCommandPlaceholder(allowedSources)}
            disabled={disabled}
            required
          />
        </Grid>
      </Grid>

      <Box sx={{ mt: 4, display: 'flex', justifyContent: 'space-between' }}>
        <SecondaryOutlineButton onClick={onBack} disabled={disabled}>
          Cancel
        </SecondaryOutlineButton>

        <PrimaryButton
          onClick={handleNext}
          disabled={disabled || loading}
        >
          {loading ? 'Loading...' : 'Load Plugin'}
        </PrimaryButton>
      </Box>
    </Box>
  );
};

export default Step1BasicInfo;
