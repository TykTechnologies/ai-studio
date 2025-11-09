import React, { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Typography,
  TextField,
  CircularProgress,
  Alert,
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
import SchemaFormRenderer from '../plugins/SchemaFormRenderer';

const LLMPluginConfigSection = ({
  plugin,
  llmId,
  configOverride,
  onConfigChange,
}) => {
  const [configSchema, setConfigSchema] = useState(null);
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [schemaError, setSchemaError] = useState(null);
  const [viewMode, setViewMode] = useState('auto'); // 'auto', 'form', 'json'
  const [configJson, setConfigJson] = useState('{}');

  // Retry logic state
  const [retryCount, setRetryCount] = useState(0);
  const [retryTimeoutId, setRetryTimeoutId] = useState(null);
  const MAX_RETRIES = 3;
  const RETRY_DELAYS = [1000, 2000, 5000]; // 1s, 2s, 5s delays

  // Initialize JSON representation of config
  useEffect(() => {
    setConfigJson(JSON.stringify(configOverride || {}, null, 2));
  }, [configOverride]);

  const fetchConfigSchema = useCallback(async (isRetry = false) => {
    if (!plugin?.id) return;

    // Clear any existing retry timeout
    if (retryTimeoutId) {
      clearTimeout(retryTimeoutId);
      setRetryTimeoutId(null);
    }

    // If this is not a retry, reset retry count
    if (!isRetry) {
      setRetryCount(0);
    }

    setSchemaLoading(true);
    setSchemaError(null);

    try {
      const schema = await pluginService.getPluginConfigSchema(plugin.id);
      setConfigSchema(schema);
      setRetryCount(0); // Reset retry count on success

      // Auto-switch to form view if schema is available
      if (schema && viewMode === 'auto') {
        setViewMode('form');
      }
    } catch (error) {
      console.warn(`Failed to fetch plugin schema (attempt ${retryCount + 1}):`, error);

      // Check if we should retry
      if (retryCount < MAX_RETRIES) {
        const delay = RETRY_DELAYS[retryCount] || RETRY_DELAYS[RETRY_DELAYS.length - 1];
        const newRetryCount = retryCount + 1;

        console.log(`Retrying schema fetch in ${delay}ms (attempt ${newRetryCount}/${MAX_RETRIES})`);

        setRetryCount(newRetryCount);
        setSchemaError(`Failed to load schema. Retrying in ${delay / 1000}s... (${newRetryCount}/${MAX_RETRIES})`);

        const timeoutId = setTimeout(() => {
          fetchConfigSchema(true); // isRetry = true
        }, delay);

        setRetryTimeoutId(timeoutId);
      } else {
        // Max retries exceeded
        console.error('Max retries exceeded for plugin schema fetch');
        setSchemaError(`Failed to load plugin schema after ${MAX_RETRIES} attempts. Please try refreshing manually.`);
        setConfigSchema(null);
        setRetryCount(0);
      }
    } finally {
      if (retryCount >= MAX_RETRIES || !isRetry) {
        setSchemaLoading(false);
      }
      // Keep loading state true during retries
    }
  }, [plugin?.id, viewMode, retryCount, retryTimeoutId]);

  // Fetch config schema when component mounts
  useEffect(() => {
    if (plugin?.id && !configSchema && !schemaLoading) {
      fetchConfigSchema();
    }
  }, [plugin?.id, configSchema, schemaLoading, fetchConfigSchema]);

  // Cleanup retry timeout on unmount
  useEffect(() => {
    return () => {
      if (retryTimeoutId) {
        clearTimeout(retryTimeoutId);
      }
    };
  }, [retryTimeoutId]);

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

  const handleRefreshSchema = async () => {
    if (!plugin?.id) return;

    // Clear any existing retry timeout and reset retry state
    if (retryTimeoutId) {
      clearTimeout(retryTimeoutId);
      setRetryTimeoutId(null);
    }
    setRetryCount(0);

    // Clear current schema state
    setConfigSchema(null);
    setSchemaError(null);
    setSchemaLoading(true);

    try {
      // Call the refresh API which invalidates cache and fetches fresh
      const schema = await pluginService.refreshPluginConfigSchema(plugin.id);
      setConfigSchema(schema);

      // Auto-switch to form view if schema is available
      if (schema && viewMode === 'auto') {
        setViewMode('form');
      }
    } catch (error) {
      console.warn('Failed to refresh plugin schema:', error);
      setSchemaError(`Refresh failed: ${error.message}`);
      setConfigSchema(null);
    } finally {
      setSchemaLoading(false);
    }
  };

  // Check if we have a real schema (not just the default fallback)
  const isRealSchema = configSchema &&
    !configSchema.description?.includes('default - manager not available') &&
    !configSchema.description?.includes('fallback -') &&
    configSchema.properties &&
    Object.keys(configSchema.properties).length > 0;

  // Determine what to show based on schema availability and view mode
  const showSchemaForm = isRealSchema && !schemaError && (viewMode === 'form' || viewMode === 'auto');
  const showJsonEditor = !showSchemaForm || viewMode === 'json';

  return (
    <Box>
      <Typography variant="body2" color="textSecondary" paragraph>
        Configure plugin settings specific to this LLM. These values will override
        the base plugin configuration when this plugin executes for this LLM.
        Leave empty to use the base configuration.
      </Typography>

      {/* Schema Status & Controls */}
      <Box display="flex" alignItems="center" gap={1} mb={2}>
        {schemaLoading && (
          <Box display="flex" alignItems="center" gap={1}>
            <CircularProgress size={16} />
            <Typography variant="caption" color="textSecondary">
              Loading configuration schema...
            </Typography>
          </Box>
        )}

        {isRealSchema && !schemaLoading && (
          <Box display="flex" alignItems="center" gap={1}>
            {/* View Mode Toggle */}
            <ToggleButtonGroup
              value={viewMode}
              exclusive
              onChange={handleViewModeChange}
              size="small"
            >
              <ToggleButton value="form" disabled={!isRealSchema}>
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

        {schemaError && (
          <Alert severity={retryCount > 0 ? "info" : "warning"} sx={{ width: '100%' }}>
            <Box display="flex" alignItems="center" justifyContent="space-between">
              <Typography variant="body2">
                {retryCount > 0 ? schemaError : 'Could not load configuration schema. Using JSON editor.'}
              </Typography>
              {!schemaLoading && (
                <IconButton size="small" onClick={handleRefreshSchema}>
                  <RefreshIcon fontSize="small" />
                </IconButton>
              )}
            </Box>
          </Alert>
        )}
      </Box>

      {/* Configuration Content */}
      {showSchemaForm && (
        <SchemaFormRenderer
          schema={configSchema}
          formData={configOverride || {}}
          onChange={handleFormChange}
          onError={(error) => console.error('Schema form error:', error)}
        />
      )}

      {showJsonEditor && (
        <TextField
          fullWidth
          label="LLM Plugin Configuration Override (JSON)"
          value={configJson}
          onChange={handleJsonChange}
          multiline
          rows={6}
          helperText="JSON object with configuration overrides for this LLM"
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

export default LLMPluginConfigSection;