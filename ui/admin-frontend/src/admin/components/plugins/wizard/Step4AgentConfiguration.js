import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Alert,
  Card,
  CardContent,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  CircularProgress,
} from '@mui/material';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import { PrimaryButton, SecondaryOutlineButton } from '../../../styles/sharedStyles';
import AgentFormFields from '../../agents/AgentFormFields';
import apiClient from '../../../utils/apiClient';
import pluginService from '../../../services/pluginService';

const Step4AgentConfiguration = ({
  pluginId,
  pluginData,
  onComplete,
  onSkip,
  loading,
  disabled,
}) => {
  const [showConfirmDialog, setShowConfirmDialog] = useState(true);
  const [configureAgent, setConfigureAgent] = useState(false);
  const [loadingData, setLoadingData] = useState(false);
  const [error, setError] = useState('');

  // Form data
  const [formData, setFormData] = useState({
    name: `${pluginData.name} Agent`,
    description: pluginData.description || '',
    pluginId: pluginId,
    appId: '',
    config: pluginData.config || {},
    groupIds: [],
    isActive: true,
  });

  // Dropdown options
  const [apps, setApps] = useState([]);
  const [groups, setGroups] = useState([]);

  // Plugin config schema
  const [configSchema, setConfigSchema] = useState(null);
  const [configJson, setConfigJson] = useState(JSON.stringify(pluginData.config || {}, null, 2));

  useEffect(() => {
    if (configureAgent) {
      loadDropdownData();
      loadPluginConfigSchema();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [configureAgent, pluginId]);

  const loadDropdownData = async () => {
    try {
      setLoadingData(true);
      // Load apps
      const appsResponse = await apiClient.get('/apps', {
        params: { page: 1, page_size: 100 },
      });
      setApps(appsResponse.data.data || []);

      // Load groups
      const groupsResponse = await apiClient.get('/groups', {
        params: { page: 1, page_size: 100 },
      });
      setGroups(groupsResponse.data.data || []);
    } catch (err) {
      console.error('Error loading dropdown data:', err);
      setError('Failed to load form options');
    } finally {
      setLoadingData(false);
    }
  };

  const loadPluginConfigSchema = async () => {
    try {
      const plugin = await pluginService.getPlugin(pluginId);
      if (plugin.manifest?.configSchema) {
        setConfigSchema(plugin.manifest.configSchema);
      }
    } catch (err) {
      console.error('Error loading plugin config schema:', err);
    }
  };

  const handleFieldChange = (field, value) => {
    setFormData(prev => ({ ...prev, [field]: value }));
  };

  const handleConfigChange = (value) => {
    setConfigJson(value);
    try {
      const parsed = JSON.parse(value);
      setFormData(prev => ({ ...prev, config: parsed }));
    } catch (err) {
      // Invalid JSON, don't update config
    }
  };

  const handleConfirmYes = () => {
    setShowConfirmDialog(false);
    setConfigureAgent(true);
  };

  const handleConfirmSkip = () => {
    setShowConfirmDialog(false);
    onSkip();
  };

  const handleSubmit = () => {
    // Validate JSON config
    try {
      JSON.parse(configJson);
    } catch (err) {
      setError('Invalid JSON in configuration');
      return;
    }

    // Validate required fields
    if (!formData.name || !formData.appId) {
      setError('Please fill in all required fields');
      return;
    }

    setError('');
    onComplete(formData);
  };

  const handleCancel = () => {
    onSkip();
  };

  // Show confirmation dialog first
  if (showConfirmDialog) {
    return (
      <Dialog open={true} maxWidth="sm" fullWidth>
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <SmartToyIcon color="primary" />
            <Typography variant="headingLarge">Agent Plugin Detected</Typography>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Typography variant="bodyLargeDefault" gutterBottom>
            This is an Agent Plugin. Would you like to configure it now to make it available in the AI Portal as a Chat option? (You can skip this step and configure the agent later from the Agents page.)
          </Typography>          
        </DialogContent>
        <DialogActions sx={{ p: 3, gap: 2 }}>
          <SecondaryOutlineButton onClick={handleConfirmSkip}>
            Skip for Now
          </SecondaryOutlineButton>
          <PrimaryButton onClick={handleConfirmYes}>
            Yes, Configure Agent
          </PrimaryButton>
        </DialogActions>
      </Dialog>
    );
  }

  // Show agent configuration form
  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 2 }}>
        <SmartToyIcon fontSize="large" color="primary" />
        <Box>
          <Typography variant="h6">Agent Configuration</Typography>
          <Typography variant="body2" color="textSecondary">
            Configure your agent to make it available in the AI Portal
          </Typography>
        </Box>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {error}
        </Alert>
      )}

      {loadingData ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
          <CircularProgress />
        </Box>
      ) : (
        <Card>
          <CardContent>
            <AgentFormFields
              formData={formData}
              onFieldChange={handleFieldChange}
              plugins={[]} // Plugin is already selected
              apps={apps}
              groups={groups}
              configJson={configJson}
              onConfigChange={handleConfigChange}
              configSchema={configSchema}
              showPluginField={false} // Don't show plugin field since it's pre-selected
              disabled={disabled || loading}
            />

            {/* Action Buttons */}
            <Box sx={{ display: 'flex', gap: 2, justifyContent: 'flex-end', mt: 4 }}>
              <SecondaryOutlineButton onClick={handleCancel} disabled={disabled || loading}>
                Skip
              </SecondaryOutlineButton>
              <PrimaryButton onClick={handleSubmit} disabled={disabled || loading}>
                {loading ? <CircularProgress size={24} /> : 'Create Agent'}
              </PrimaryButton>
            </Box>
          </CardContent>
        </Card>
      )}
    </Box>
  );
};

export default Step4AgentConfiguration;
