import React, { useState, useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  Box,
  Typography,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Checkbox,
  FormControlLabel,
  CircularProgress,
  Alert,
  Chip,
  OutlinedInput,
  Card,
  CardContent,
  Divider,
} from '@mui/material';
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryOutlineButton,
} from '../../styles/sharedStyles';
import agentService from '../../services/agentService';
import pluginService from '../../services/pluginService';
import apiClient from '../../utils/apiClient';

const AgentForm = () => {
  const navigate = useNavigate();
  const { id } = useParams();
  const isEditMode = Boolean(id);

  const [loading, setLoading] = useState(isEditMode);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // Form data
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    pluginId: '',
    appId: '',
    config: {},
    groupIds: [],
    isActive: true,
    namespace: '',
  });

  // Dropdown options
  const [plugins, setPlugins] = useState([]);
  const [apps, setApps] = useState([]);
  const [groups, setGroups] = useState([]);

  // Plugin config schema
  const [configSchema, setConfigSchema] = useState(null);
  const [configJson, setConfigJson] = useState('{}');

  // Load initial data
  useEffect(() => {
    loadDropdownData();
    if (isEditMode) {
      loadAgent();
    }
  }, [id, isEditMode]);

  const loadDropdownData = async () => {
    try {
      // Load agent plugins - filter by hook_type instead of plugin_type
      const pluginResult = await pluginService.listPlugins(1, 100, '', true);
      const agentPlugins = pluginResult.data.filter(p =>
        p.hookType === 'agent' || p.hookTypes?.includes('agent')
      );
      setPlugins(agentPlugins);

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
    }
  };

  const loadAgent = async () => {
    try {
      setLoading(true);
      const agent = await agentService.getAgent(id);

      setFormData({
        name: agent.name,
        description: agent.description || '',
        pluginId: agent.pluginId,
        appId: agent.appId,
        config: agent.config || {},
        groupIds: agent.groups.map(g => g.id),
        isActive: agent.isActive,
        namespace: agent.namespace || '',
      });

      setConfigJson(JSON.stringify(agent.config || {}, null, 2));

      // Load config schema for the plugin
      if (agent.pluginId) {
        loadPluginConfigSchema(agent.pluginId);
      }
    } catch (err) {
      console.error('Error loading agent:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const loadPluginConfigSchema = async (pluginId) => {
    try {
      const plugin = await pluginService.getPlugin(pluginId);
      if (plugin.manifest?.configSchema) {
        setConfigSchema(plugin.manifest.configSchema);
      }
    } catch (err) {
      console.error('Error loading plugin config schema:', err);
    }
  };

  const handleChange = (field, value) => {
    setFormData(prev => ({ ...prev, [field]: value }));

    // Load config schema when plugin changes
    if (field === 'pluginId' && value) {
      loadPluginConfigSchema(value);
    }
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

  const handleSubmit = async (e) => {
    e.preventDefault();

    // Validate JSON config
    try {
      JSON.parse(configJson);
    } catch (err) {
      setError('Invalid JSON in configuration');
      return;
    }

    setSaving(true);
    setError('');

    try {
      if (isEditMode) {
        await agentService.updateAgent(id, formData);
      } else {
        await agentService.createAgent(formData);
      }
      navigate('/admin/agents');
    } catch (err) {
      console.error('Error saving agent:', err);
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = () => {
    navigate('/admin/agents');
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <>
      <TitleBox>
        <Typography variant="headingXLarge">
          {isEditMode ? 'Edit Agent' : 'Create Agent'}
        </Typography>
      </TitleBox>

      <ContentBox>
        {error && (
          <Alert severity="error" sx={{ mb: 3 }}>
            {error}
          </Alert>
        )}

        <Card>
          <CardContent>
            <form onSubmit={handleSubmit}>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                {/* Basic Information */}
                <Typography variant="headingMedium">Basic Information</Typography>

                <TextField
                  label="Name"
                  required
                  fullWidth
                  value={formData.name}
                  onChange={(e) => handleChange('name', e.target.value)}
                  helperText="A descriptive name for this agent"
                />

                <TextField
                  label="Description"
                  fullWidth
                  multiline
                  rows={3}
                  value={formData.description}
                  onChange={(e) => handleChange('description', e.target.value)}
                  helperText="Optional description of what this agent does"
                />

                <Divider />

                {/* Agent Configuration */}
                <Typography variant="headingMedium">Agent Configuration</Typography>

                <FormControl fullWidth required>
                  <InputLabel>Plugin</InputLabel>
                  <Select
                    value={formData.pluginId}
                    label="Plugin"
                    onChange={(e) => handleChange('pluginId', e.target.value)}
                  >
                    {plugins.map(plugin => (
                      <MenuItem key={plugin.id} value={plugin.id}>
                        {plugin.name}
                        {!plugin.isActive && ' (Inactive)'}
                      </MenuItem>
                    ))}
                  </Select>
                  {plugins.length === 0 && (
                    <Typography variant="bodySmallDefault" color="text.secondary" sx={{ mt: 1 }}>
                      No agent plugins available. Please install an agent plugin first.
                    </Typography>
                  )}
                </FormControl>

                <FormControl fullWidth required>
                  <InputLabel>App</InputLabel>
                  <Select
                    value={formData.appId}
                    label="App"
                    onChange={(e) => handleChange('appId', e.target.value)}
                  >
                    {apps.map(app => (
                      <MenuItem key={app.id} value={app.id}>
                        {app.attributes?.name || `App ${app.id}`}
                      </MenuItem>
                    ))}
                  </Select>
                  <Typography variant="bodySmallDefault" color="text.secondary" sx={{ mt: 1 }}>
                    The app provides LLMs, tools, and datasources for the agent
                  </Typography>
                </FormControl>

                {/* Plugin Configuration */}
                {configSchema && (
                  <Box>
                    <Typography variant="bodyMedium" gutterBottom>
                      Plugin Configuration
                    </Typography>
                    <Typography variant="bodySmallDefault" color="text.secondary" sx={{ mb: 2 }}>
                      {configSchema.description || 'Plugin-specific configuration'}
                    </Typography>
                  </Box>
                )}

                <TextField
                  label="Configuration (JSON)"
                  fullWidth
                  multiline
                  rows={8}
                  value={configJson}
                  onChange={(e) => handleConfigChange(e.target.value)}
                  helperText="Plugin-specific configuration in JSON format"
                  error={(() => {
                    try {
                      JSON.parse(configJson);
                      return false;
                    } catch {
                      return true;
                    }
                  })()}
                  sx={{ fontFamily: 'monospace' }}
                />

                <Divider />

                {/* Access Control */}
                <Typography variant="headingMedium">Access Control</Typography>

                <FormControl fullWidth>
                  <InputLabel>Groups</InputLabel>
                  <Select
                    multiple
                    value={formData.groupIds}
                    onChange={(e) => handleChange('groupIds', e.target.value)}
                    input={<OutlinedInput label="Groups" />}
                    renderValue={(selected) => (
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                        {selected.map((groupId) => {
                          const group = groups.find(g => g.id === groupId);
                          return (
                            <Chip
                              key={groupId}
                              label={group?.attributes?.name || `Group ${groupId}`}
                              size="small"
                            />
                          );
                        })}
                      </Box>
                    )}
                  >
                    {groups.map((group) => (
                      <MenuItem key={group.id} value={group.id}>
                        <Checkbox checked={formData.groupIds.indexOf(group.id) > -1} />
                        {group.attributes?.name || `Group ${group.id}`}
                      </MenuItem>
                    ))}
                  </Select>
                  <Typography variant="bodySmallDefault" color="text.secondary" sx={{ mt: 1 }}>
                    Leave empty to make agent available to all users
                  </Typography>
                </FormControl>

                <Divider />

                {/* Settings */}
                <Typography variant="headingMedium">Settings</Typography>

                <FormControlLabel
                  control={
                    <Checkbox
                      checked={formData.isActive}
                      onChange={(e) => handleChange('isActive', e.target.checked)}
                    />
                  }
                  label="Active"
                />

                <TextField
                  label="Namespace"
                  fullWidth
                  value={formData.namespace}
                  onChange={(e) => handleChange('namespace', e.target.value)}
                  helperText="Optional namespace for multi-tenancy"
                />

                {/* Actions */}
                <Box sx={{ display: 'flex', gap: 2, justifyContent: 'flex-end', mt: 2 }}>
                  <SecondaryOutlineButton onClick={handleCancel} disabled={saving}>
                    Cancel
                  </SecondaryOutlineButton>
                  <PrimaryButton type="submit" disabled={saving}>
                    {saving ? <CircularProgress size={24} /> : isEditMode ? 'Update' : 'Create'}
                  </PrimaryButton>
                </Box>
              </Box>
            </form>
          </CardContent>
        </Card>
      </ContentBox>
    </>
  );
};

export default AgentForm;
