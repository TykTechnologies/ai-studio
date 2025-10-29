import React, { useState, useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  Box,
  Typography,
  CircularProgress,
  Alert,
  Card,
  CardContent,
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
import AgentFormFields from './AgentFormFields';

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
              <AgentFormFields
                formData={formData}
                onFieldChange={handleChange}
                plugins={plugins}
                apps={apps}
                groups={groups}
                configJson={configJson}
                onConfigChange={handleConfigChange}
                configSchema={configSchema}
                showPluginField={true}
                disabled={saving}
              />

              {/* Actions */}
              <Box sx={{ display: 'flex', gap: 2, justifyContent: 'flex-end', mt: 4 }}>
                <SecondaryOutlineButton onClick={handleCancel} disabled={saving}>
                  Cancel
                </SecondaryOutlineButton>
                <PrimaryButton type="submit" disabled={saving}>
                  {saving ? <CircularProgress size={24} /> : isEditMode ? 'Update' : 'Create'}
                </PrimaryButton>
              </Box>
            </form>
          </CardContent>
        </Card>
      </ContentBox>
    </>
  );
};

export default AgentForm;
