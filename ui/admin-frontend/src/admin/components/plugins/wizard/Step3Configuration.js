import React, { useState } from 'react';
import {
  Box,
  Typography,
  Alert,
  Chip,
  Button,
  AlertTitle,
  Select,
  MenuItem,
  Checkbox,
  ListItemText,
  FormControl,
  InputLabel,
  Divider,
} from '@mui/material';
import LockIcon from '@mui/icons-material/Lock';
import EditIcon from '@mui/icons-material/Edit';
import { PrimaryButton, SecondaryOutlineButton } from '../../../styles/sharedStyles';
import PluginConfigurationSection from '../PluginConfigurationSection';
import pluginService, { PluginService } from '../../../services/pluginService';
import { useNavigate } from 'react-router-dom';

const Step3Configuration = ({ pluginId, pluginData, configSchema, onComplete, onBack, loading, disabled }) => {
  const navigate = useNavigate();
  const [config, setConfig] = useState({});
  const [configError, setConfigError] = useState(null);
  const [hookTypesCustomized, setHookTypesCustomized] = useState(false);
  const [customHookTypes, setCustomHookTypes] = useState(pluginData.hookTypes || []);

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

  const handleCancel = async () => {
    if (window.confirm('Cancel plugin creation? This will delete the plugin record.')) {
      try {
        if (pluginId) {
          await pluginService.deletePlugin(pluginId);
        }
        navigate('/admin/plugins');
      } catch (err) {
        console.error('Error deleting plugin:', err);
      }
    }
  };

  const handleComplete = () => {
    if (configError) {
      return;
    }
    onComplete({
      config: config,
      hookTypes: hookTypesCustomized ? customHookTypes : pluginData.hookTypes,
      hookTypesCustomized: hookTypesCustomized,
    });
  };

  const handleHookTypesChange = (event) => {
    setCustomHookTypes(event.target.value);
  };

  const handleResetToManifest = () => {
    setCustomHookTypes(pluginData.hookTypes || []);
    setHookTypesCustomized(false);
  };

  const getPluginCategory = (data) => {
    const hooks = data.hookTypes || [];
    if (hooks.includes(PluginService.HOOK_TYPES.STUDIO_UI)) {
      return 'UI Plugin';
    }
    if (hooks.includes(PluginService.HOOK_TYPES.AGENT)) {
      return 'Agent Plugin';
    }
    return 'Gateway Plugin';
  };

  const availableHookTypes = pluginService.getAvailableHookTypes();
  const canProceed = !configError;

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Plugin Configuration
      </Typography>

      <Typography variant="body2" color="textSecondary" sx={{ mb: 3 }}>
        Review the plugin capabilities and complete the configuration to finalize the setup.
      </Typography>

      {/* Success Alert */}
      <Alert severity="success" sx={{ mb: 3 }}>
        <Typography variant="body2">
          Plugin <strong>{pluginData.name}</strong> has been loaded successfully
          {pluginData.hookTypes && pluginData.hookTypes.length > 0 && ' and security permissions have been approved'}.
          Complete the configuration to finalize the setup.
        </Typography>
      </Alert>

      {/* Capabilities Section */}
      <Box mb={4}>
        <Typography variant="h6" gutterBottom>
          Plugin Capabilities
        </Typography>

        {/* Category Badge */}
        <Box mb={2}>
          <Typography variant="body2" color="textSecondary" gutterBottom>
            Category
          </Typography>
          <Chip label={getPluginCategory(pluginData)} color="primary" sx={{ fontWeight: 'bold' }} />
        </Box>

        {/* Hook Types - From Manifest */}
        {pluginData.hookTypes && pluginData.hookTypes.length > 0 && (
          <Box mb={2}>
            <Typography variant="body2" color="textSecondary" gutterBottom>
              Hook Types (from manifest)
            </Typography>

            {!hookTypesCustomized ? (
              <Box>
                <Box display="flex" gap={1} flexWrap="wrap" mb={1}>
                  {pluginData.hookTypes.map(hook => (
                    <Chip
                      key={hook}
                      label={PluginService.HOOK_TYPE_LABELS[hook] || hook}
                      icon={<LockIcon />}
                      size="small"
                    />
                  ))}
                </Box>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<EditIcon />}
                  onClick={() => setHookTypesCustomized(true)}
                  disabled={disabled}
                >
                  Customize Hook Types (Advanced)
                </Button>
              </Box>
            ) : (
              <Box>
                <Alert severity="warning" sx={{ mb: 2 }}>
                  <AlertTitle>Customizing Hook Types</AlertTitle>
                  Removing hooks declared in the manifest may cause the plugin to malfunction.
                  <Button size="small" onClick={handleResetToManifest} disabled={disabled}>
                    Reset to Manifest
                  </Button>
                </Alert>
                <FormControl fullWidth>
                  <InputLabel>Hook Types</InputLabel>
                  <Select
                    multiple
                    value={customHookTypes}
                    onChange={handleHookTypesChange}
                    renderValue={(selected) => (
                      <Box display="flex" gap={0.5} flexWrap="wrap">
                        {selected.map(hook => (
                          <Chip key={hook} label={PluginService.HOOK_TYPE_LABELS[hook]} size="small" />
                        ))}
                      </Box>
                    )}
                    disabled={disabled}
                  >
                    {availableHookTypes.map(hookType => (
                      <MenuItem key={hookType.value} value={hookType.value}>
                        <Checkbox checked={customHookTypes.includes(hookType.value)} />
                        <ListItemText primary={hookType.label} />
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Box>
            )}
          </Box>
        )}
      </Box>

      <Divider sx={{ my: 3 }} />

      {/* Configuration Section */}
      <Box>
        <Typography variant="h6" gutterBottom>
          Configuration
        </Typography>

        <Typography variant="body2" color="textSecondary" sx={{ mb: 3 }}>
          Configure your plugin with the required settings. You can use the form below if a configuration schema is available,
          or enter JSON directly in the editor.
        </Typography>

        <PluginConfigurationSection
          pluginId={pluginId}
          config={config}
          onConfigChange={handleConfigChange}
          isEdit={true}
          configError={configError}
        />
      </Box>

      {/* Action Buttons */}
      <Box sx={{ mt: 4, display: 'flex', justifyContent: 'space-between' }}>
        <SecondaryOutlineButton onClick={handleCancel} disabled={disabled || loading}>
          Cancel
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
