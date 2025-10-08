import React, { useState, useEffect } from 'react';
import {
  TextField,
  Box,
  Typography,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormControlLabel,
  Switch,
  Button,
  Grid,
} from '@mui/material';
import { PrimaryButton, SecondaryOutlineButton } from '../../../styles/sharedStyles';
import pluginService from '../../../services/pluginService';

const Step1BasicInfo = ({ data, onComplete, onBack, loading, disabled }) => {
  const [formData, setFormData] = useState({
    name: data.name || '',
    slug: data.slug || '',
    description: data.description || '',
    pluginType: data.pluginType || 'gateway',
    command: data.command || '',
    hookType: data.hookType || '',
    isActive: data.isActive !== undefined ? data.isActive : true,
    namespace: data.namespace || '',
  });

  const [errors, setErrors] = useState({});

  useEffect(() => {
    // Auto-generate slug from name if name changes and slug is empty
    if (formData.name && !formData.slug) {
      const slug = formData.name
        .toLowerCase()
        .replace(/[^a-z0-9\s-]/g, '')
        .replace(/\s+/g, '-')
        .replace(/-+/g, '-')
        .trim();
      setFormData(prev => ({ ...prev, slug }));
    }
  }, [formData.name]);

  const handleInputChange = (field) => (event) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
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

    if (!formData.slug.trim()) {
      newErrors.slug = 'Plugin slug is required';
    }

    if (!formData.command.trim()) {
      newErrors.command = 'Plugin command is required';
    }

    // Hook type is required for gateway plugins
    if (formData.pluginType === 'gateway' && !formData.hookType) {
      newErrors.hookType = 'Hook type is required for gateway plugins';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleNext = () => {
    if (validateForm()) {
      // Auto-set hook type for AI Studio plugins
      const finalData = { ...formData };
      if (finalData.pluginType === 'ai_studio') {
        finalData.hookType = 'studio_ui';
      } else if (finalData.pluginType === 'agent') {
        finalData.hookType = 'agent';
      }

      onComplete(finalData);
    }
  };

  const availableHookTypes = pluginService.getAvailableHookTypes();

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Basic Plugin Information
      </Typography>

      <Typography variant="body2" color="textSecondary" sx={{ mb: 3 }}>
        Enter the basic details for your plugin. The system will validate the command and load additional metadata in the next steps.
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

        <Grid item xs={12} md={6}>
          <TextField
            fullWidth
            label="Slug"
            value={formData.slug}
            onChange={handleInputChange('slug')}
            error={!!errors.slug}
            helperText={errors.slug || 'Unique identifier (auto-generated from name)'}
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

        <Grid item xs={12} md={6}>
          <FormControl fullWidth required>
            <InputLabel>Plugin Type</InputLabel>
            <Select
              value={formData.pluginType}
              label="Plugin Type"
              onChange={handleInputChange('pluginType')}
              disabled={disabled}
            >
              <MenuItem value="gateway">Gateway Plugin</MenuItem>
              <MenuItem value="ai_studio">AI Studio Plugin</MenuItem>
              <MenuItem value="agent">AI Studio Agent</MenuItem>
            </Select>
          </FormControl>
        </Grid>

        {formData.pluginType === 'gateway' && (
          <Grid item xs={12} md={6}>
            <FormControl fullWidth required error={!!errors.hookType}>
              <InputLabel>Hook Type</InputLabel>
              <Select
                value={formData.hookType}
                label="Hook Type"
                onChange={handleInputChange('hookType')}
                disabled={disabled}
              >
                {availableHookTypes.map((hookType) => (
                  <MenuItem key={hookType.value} value={hookType.value}>
                    {hookType.label}
                  </MenuItem>
                ))}
              </Select>
              {errors.hookType && (
                <Typography variant="caption" color="error" sx={{ mt: 0.5, ml: 1.5 }}>
                  {errors.hookType}
                </Typography>
              )}
            </FormControl>
          </Grid>
        )}

        {formData.pluginType === 'ai_studio' && (
          <Grid item xs={12} md={6}>
            <Typography variant="body2" color="textSecondary" sx={{ mt: 2 }}>
              AI Studio plugins automatically use the "studio_ui" hook type for UI extensions.
            </Typography>
          </Grid>
        )}

        {formData.pluginType === 'agent' && (
          <Grid item xs={12} md={6}>
            <Typography variant="body2" color="textSecondary" sx={{ mt: 2 }}>
              Agent plugins automatically use the "agent" hook type for agentic workflows.
            </Typography>
          </Grid>
        )}

        <Grid item xs={12}>
          <TextField
            fullWidth
            label="Command"
            value={formData.command}
            onChange={handleInputChange('command')}
            error={!!errors.command}
            helperText={
              errors.command ||
              'Plugin command - use oci:// for OCI artifacts, grpc:// for external services, or local path for binaries'
            }
            placeholder="e.g., oci://registry.com/my-plugin:latest or /path/to/plugin-binary"
            disabled={disabled}
            required
          />
        </Grid>

        <Grid item xs={12}>
          <FormControlLabel
            control={
              <Switch
                checked={formData.isActive}
                onChange={handleInputChange('isActive')}
                disabled={disabled}
              />
            }
            label="Active"
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