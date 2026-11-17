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
import { listAll } from '../../utils/listAll';
import { P } from '../../rbac/permissions';
import WorkspaceSelect from '../workspaces/WorkspaceSelect';
import { useCanMove, workspaceFieldError } from '../workspaces/workspaceFields';
import NotFound from '../../pages/NotFound';

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
    workspaceId: null, // Enterprise: the Workspace it lives in (landing rule when null)
  });
  // The stored Workspace of the agent being edited: a different workspaceId
  // on save is a move.
  const [originalWorkspaceId, setOriginalWorkspaceId] = useState(null);
  const [workspaceError, setWorkspaceError] = useState('');
  const canMove = useCanMove(P.AGENTS_WRITE);
  // An agent in another Workspace (or deleted) answers 404: the form shows
  // the standard not-found panel instead of an empty form.
  const [notFound, setNotFound] = useState(false);

  // Dropdown options
  const [plugins, setPlugins] = useState([]);
  const [apps, setApps] = useState([]);
  const [appsLoaded, setAppsLoaded] = useState(false);
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
      const appsResponse = await listAll(apiClient, '/apps');
      setApps(appsResponse.data.data || []);
      setAppsLoaded(true);

      // Load groups
      const groupsResponse = await listAll(apiClient, '/groups');
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
        workspaceId: agent.workspaceId ?? null,
      });
      setOriginalWorkspaceId(agent.workspaceId ?? null);

      setConfigJson(JSON.stringify(agent.config || {}, null, 2));

      // Load config schema for the plugin
      if (agent.pluginId) {
        loadPluginConfigSchema(agent.pluginId);
      }
    } catch (err) {
      console.error('Error loading agent:', err);
      if (err.status === 404) {
        setNotFound(true);
        return;
      }
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

    // workspaceId is sent only to choose a Workspace (create) or to move the
    // agent (edit).
    const { workspaceId, ...fields } = formData;
    const sendWorkspace =
      workspaceId !== null &&
      workspaceId !== undefined &&
      (!isEditMode || Number(workspaceId) !== Number(originalWorkspaceId));
    const payload = { ...fields, workspaceId: sendWorkspace ? Number(workspaceId) : undefined };

    try {
      if (isEditMode) {
        await agentService.updateAgent(id, payload);
      } else {
        await agentService.createAgent(payload);
      }
      navigate('/admin/agents');
    } catch (err) {
      console.error('Error saving agent:', err);
      const wsError = workspaceFieldError(err);
      if (wsError) {
        setWorkspaceError(wsError);
        setError(wsError);
        return;
      }
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = () => {
    navigate('/admin/agents');
  };

  if (notFound) return <NotFound />;

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
                workspaceId={formData.workspaceId ?? null}
                appsLoaded={appsLoaded}
              />

              {/* Workspace (Enterprise): where it is created; on edit, the move control. */}
              <Box sx={{ mt: 3 }}>
                <WorkspaceSelect
                  permission={P.AGENTS_WRITE}
                  value={formData.workspaceId}
                  onChange={(ws) => {
                    setFormData((prev) => ({ ...prev, workspaceId: ws }));
                    setWorkspaceError('');
                  }}
                  disabled={saving || (isEditMode && !canMove)}
                  error={workspaceError}
                  helperText={
                    isEditMode
                      ? 'Moving it to another Workspace needs write in both.'
                      : 'The Workspace it is created in.'
                  }
                />
              </Box>

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
