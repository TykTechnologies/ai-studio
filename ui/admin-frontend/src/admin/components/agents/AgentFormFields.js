import React from 'react';
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
  Chip,
  OutlinedInput,
  Divider,
} from '@mui/material';

/**
 * Reusable form fields component for agent creation/editing
 * Can be used in both standalone AgentForm and wizard Step4AgentConfiguration
 */
const AgentFormFields = ({
  formData,
  onFieldChange,
  plugins = [],
  apps = [],
  groups = [],
  configJson = '{}',
  onConfigChange,
  configSchema = null,
  showPluginField = true,
  disabled = false,
}) => {
  const handleChange = (field, value) => {
    onFieldChange(field, value);
  };

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
      {/* Basic Information */}
      <Typography variant="headingMedium">Basic Information</Typography>

      <TextField
        label="Name"
        required
        fullWidth
        value={formData.name || ''}
        onChange={(e) => handleChange('name', e.target.value)}
        helperText="A descriptive name for this agent"
        disabled={disabled}
      />

      <TextField
        label="Description"
        fullWidth
        multiline
        rows={3}
        value={formData.description || ''}
        onChange={(e) => handleChange('description', e.target.value)}
        helperText="Optional description of what this agent does"
        disabled={disabled}
      />

      <Divider />

      {/* Agent Configuration */}
      <Typography variant="headingMedium">Agent Configuration</Typography>

      {showPluginField && (
        <FormControl fullWidth required>
          <InputLabel>Plugin</InputLabel>
          <Select
            value={formData.pluginId || ''}
            label="Plugin"
            onChange={(e) => handleChange('pluginId', e.target.value)}
            disabled={disabled}
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
      )}

      <FormControl fullWidth required>
        <InputLabel>App</InputLabel>
        <Select
          value={formData.appId || ''}
          label="App"
          onChange={(e) => handleChange('appId', e.target.value)}
          disabled={disabled}
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
        onChange={(e) => onConfigChange(e.target.value)}
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
        disabled={disabled}
      />

      <Divider />

      {/* Access Control */}
      <Typography variant="headingMedium">Access Control</Typography>

      <FormControl fullWidth>
        <InputLabel>Teams</InputLabel>
        <Select
          multiple
          value={formData.groupIds || []}
          onChange={(e) => handleChange('groupIds', e.target.value)}
          input={<OutlinedInput label="Teams" />}
          renderValue={(selected) => (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
              {selected.map((groupId) => {
                const group = groups.find(g => g.id === groupId);
                return (
                  <Chip
                    key={groupId}
                    label={group?.attributes?.name || `Team ${groupId}`}
                    size="small"
                  />
                );
              })}
            </Box>
          )}
          disabled={disabled}
        >
          {groups.map((group) => (
            <MenuItem key={group.id} value={group.id}>
              <Checkbox checked={(formData.groupIds || []).indexOf(group.id) > -1} />
              {group.attributes?.name || `Team ${group.id}`}
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
            checked={formData.isActive !== undefined ? formData.isActive : true}
            onChange={(e) => handleChange('isActive', e.target.checked)}
            disabled={disabled}
          />
        }
        label="Active"
      />
    </Box>
  );
};

export default AgentFormFields;
