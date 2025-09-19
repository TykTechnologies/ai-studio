import React, { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Typography,
  TextField,
  CircularProgress,
  Alert,
  Chip,
  ToggleButtonGroup,
  ToggleButton,
  IconButton,
  Tooltip,
} from '@mui/material';
import {
  Refresh as RefreshIcon,
  Code as CodeIcon,
  ViewList as FormIcon,
} from '@mui/icons-material';
import pluginService from '../../services/pluginService';
import SchemaFormRenderer from './SchemaFormRenderer';

const PluginConfigurationSection = ({
  pluginId,
  config,
  onConfigChange,
  isEdit,
  configError,
}) => {
  const [configSchema, setConfigSchema] = useState(null);
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [schemaError, setSchemaError] = useState(null);
  const [viewMode, setViewMode] = useState('auto'); // 'auto', 'form', 'json'
  const [configJson, setConfigJson] = useState('{}');

  // Initialize JSON representation of config
  useEffect(() => {
    setConfigJson(JSON.stringify(config || {}, null, 2));
  }, [config]);

  const fetchConfigSchema = useCallback(async () => {
    if (!pluginId) return;

    setSchemaLoading(true);
    setSchemaError(null);

    try {
      const schema = await pluginService.getPluginConfigSchema(pluginId);
      setConfigSchema(schema);

      // Auto-switch to form view if schema is available
      if (schema && viewMode === 'auto') {
        setViewMode('form');
      }
    } catch (error) {
      console.warn('Failed to fetch plugin schema:', error);
      setSchemaError(error.message);
      setConfigSchema(null);
    } finally {
      setSchemaLoading(false);
    }
  }, [pluginId]);

  // Fetch config schema when component mounts (if in edit mode)
  useEffect(() => {
    if (isEdit && pluginId && !configSchema && !schemaLoading) {
      fetchConfigSchema();
    }
  }, [isEdit, pluginId, configSchema, schemaLoading, fetchConfigSchema]);

  const handleViewModeChange = (event, newMode) => {
    if (newMode !== null) {
      setViewMode(newMode);
    }
  };

  const handleJsonChange = (event) => {
    const value = event.target.value;
    setConfigJson(value);

    // Pass the raw string to parent for validation
    onConfigChange(value);
  };

  const handleFormChange = (formData) => {
    // Update both the form config and JSON representation
    onConfigChange(formData);
    setConfigJson(JSON.stringify(formData || {}, null, 2));
  };

  const handleRefreshSchema = () => {
    fetchConfigSchema();
  };

  // Determine what to show based on schema availability and view mode
  const showSchemaForm = configSchema && !schemaError && (viewMode === 'form' || viewMode === 'auto');
  const showJsonEditor = !showSchemaForm || viewMode === 'json';

  return (
    <Box>
      <Typography variant="body2" color="textSecondary" paragraph>
        {isEdit && !schemaLoading && configSchema ?
          'Configure this plugin using the generated form below, or switch to JSON editor for advanced options.' :
          'Optional JSON configuration that will be passed to the plugin. This can include any plugin-specific settings or parameters.'
        }
      </Typography>

      {/* Schema Status & Controls */}
      {isEdit && (
        <Box display="flex" alignItems="center" gap={1} mb={2}>
          {schemaLoading && (
            <Box display="flex" alignItems="center" gap={1}>
              <CircularProgress size={16} />
              <Typography variant="caption" color="textSecondary">
                Loading configuration schema...
              </Typography>
            </Box>
          )}

          {configSchema && !schemaLoading && (
            <Box display="flex" alignItems="center" gap={1}>
              <Chip
                size="small"
                label="Schema Available"
                color="success"
                variant="outlined"
              />

              {/* View Mode Toggle */}
              <ToggleButtonGroup
                value={viewMode}
                exclusive
                onChange={handleViewModeChange}
                size="small"
              >
                <ToggleButton value="form" disabled={!configSchema}>
                  <Tooltip title="Form View">
                    <FormIcon fontSize="small" />
                  </Tooltip>
                </ToggleButton>
                <ToggleButton value="json">
                  <Tooltip title="JSON Editor">
                    <CodeIcon fontSize="small" />
                  </Tooltip>
                </ToggleButton>
              </ToggleButtonGroup>

              {/* Refresh Schema Button */}
              <Tooltip title="Refresh Schema">
                <IconButton size="small" onClick={handleRefreshSchema} disabled={schemaLoading}>
                  <RefreshIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </Box>
          )}

          {schemaError && !schemaLoading && (
            <Alert severity="warning" sx={{ width: '100%' }}>
              <Box display="flex" alignItems="center" justifyContent="space-between">
                <Typography variant="body2">
                  Could not load configuration schema. Using JSON editor.
                </Typography>
                <IconButton size="small" onClick={handleRefreshSchema}>
                  <RefreshIcon fontSize="small" />
                </IconButton>
              </Box>
            </Alert>
          )}
        </Box>
      )}

      {/* Configuration Content */}
      {showSchemaForm && (
        <SchemaFormRenderer
          schema={configSchema}
          formData={config || {}}
          onChange={handleFormChange}
          onError={(error) => console.error('Schema form error:', error)}
        />
      )}

      {showJsonEditor && (
        <TextField
          fullWidth
          label="Configuration JSON"
          value={configJson}
          onChange={handleJsonChange}
          multiline
          rows={8}
          error={!!configError}
          helperText={configError || 'Valid JSON configuration object'}
          sx={{ fontFamily: 'monospace' }}
        />
      )}

      {/* Schema Details (for debugging) */}
      {configSchema && process.env.NODE_ENV === 'development' && (
        <Box mt={2}>
          <Typography variant="caption" color="textSecondary">
            Schema loaded: {configSchema.title || 'Untitled'}
            {configSchema.description && ` - ${configSchema.description}`}
          </Typography>
        </Box>
      )}
    </Box>
  );
};

export default PluginConfigurationSection;