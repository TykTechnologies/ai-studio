import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Box,
  Divider,
  Card,
  CardContent,
  Chip,
  Alert,
  CircularProgress,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@mui/material';
import {
  ExpandMore as ExpandMoreIcon,
  Settings as SettingsIcon,
} from '@mui/icons-material';
import { getLLMPluginConfig, updateLLMPluginConfig } from '../../services/llmService';
import LLMPluginConfigSection from './LLMPluginConfigSection';

const PluginConfigDialog = ({
  open,
  onClose,
  plugin,
  llmId,
  onConfigSaved,
}) => {
  const [baseConfig, setBaseConfig] = useState({});
  const [overrideConfig, setOverrideConfig] = useState({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [baseConfigExpanded, setBaseConfigExpanded] = useState(false);

  // Fetch plugin configuration when dialog opens
  useEffect(() => {
    if (open && plugin && llmId) {
      fetchPluginConfig();
    }
  }, [open, plugin?.id, llmId]);

  const fetchPluginConfig = async () => {
    setLoading(true);
    setError(null);

    try {
      // Fetch current LLM plugin configuration override
      const llmPluginConfig = await getLLMPluginConfig(llmId, plugin.id);
      setOverrideConfig(llmPluginConfig || {});

      // Set base config from plugin data
      setBaseConfig(plugin.config || {});
    } catch (err) {
      console.error('Error fetching plugin config:', err);
      setError(err.message);
      setOverrideConfig({});
      setBaseConfig(plugin.config || {});
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);

    try {
      await updateLLMPluginConfig(llmId, plugin.id, overrideConfig);

      if (onConfigSaved) {
        onConfigSaved(plugin.id, overrideConfig);
      }

      onClose();
    } catch (err) {
      console.error('Error saving plugin config:', err);
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleOverrideConfigChange = (newConfig) => {
    setOverrideConfig(newConfig);
  };

  const handleClose = () => {
    if (!saving) {
      onClose();
    }
  };

  // Create preview of merged configuration
  const mergedConfig = { ...baseConfig, ...overrideConfig };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      scroll="paper"
    >
      <DialogTitle>
        <Box display="flex" alignItems="center" gap={1}>
          <SettingsIcon color="primary" />
          <Typography variant="h6">
            Configure Plugin for LLM
          </Typography>
        </Box>
        <Typography variant="body2" color="textSecondary">
          {plugin?.name} configuration for this specific LLM
        </Typography>
      </DialogTitle>

      <DialogContent dividers>
        {loading ? (
          <Box display="flex" justifyContent="center" alignItems="center" py={4}>
            <CircularProgress />
            <Typography variant="body2" sx={{ ml: 2 }}>
              Loading plugin configuration...
            </Typography>
          </Box>
        ) : error ? (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        ) : (
          <Box>
            {/* Plugin Information */}
            <Card variant="outlined" sx={{ mb: 2 }}>
              <CardContent>
                <Box display="flex" alignItems="center" gap={1} mb={1}>
                  <Typography variant="h6">{plugin?.name}</Typography>
                  <Chip
                    label={plugin?.hookType}
                    size="small"
                    color="primary"
                    variant="outlined"
                  />
                </Box>
                <Typography variant="body2" color="textSecondary">
                  {plugin?.description || 'No description available'}
                </Typography>
              </CardContent>
            </Card>

            {/* Base Configuration (Read-only) */}
            <Accordion
              expanded={baseConfigExpanded}
              onChange={(e, expanded) => setBaseConfigExpanded(expanded)}
              sx={{ mb: 2 }}
            >
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant="subtitle1">
                  Base Plugin Configuration (Read-only)
                </Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Typography variant="body2" color="textSecondary" paragraph>
                  This is the default configuration for this plugin.
                  Changes here would affect all LLMs using this plugin.
                </Typography>
                <Box
                  component="pre"
                  sx={{
                    backgroundColor: 'grey.50',
                    p: 2,
                    borderRadius: 1,
                    overflow: 'auto',
                    fontSize: '0.875rem',
                    fontFamily: 'monospace',
                  }}
                >
                  {JSON.stringify(baseConfig, null, 2)}
                </Box>
              </AccordionDetails>
            </Accordion>

            {/* LLM-Specific Configuration Override */}
            <Card variant="outlined">
              <CardContent>
                <Typography variant="subtitle1" gutterBottom>
                  LLM-Specific Configuration Override
                </Typography>
                <Typography variant="body2" color="textSecondary" paragraph>
                  Configure plugin settings specific to this LLM. These values will override
                  the base configuration when this plugin executes for this LLM.
                </Typography>

                <LLMPluginConfigSection
                  plugin={plugin}
                  llmId={llmId}
                  configOverride={overrideConfig}
                  onConfigChange={handleOverrideConfigChange}
                />
              </CardContent>
            </Card>

            {/* Configuration Preview */}
            {Object.keys(mergedConfig).length > 0 && (
              <Accordion sx={{ mt: 2 }}>
                <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                  <Typography variant="subtitle1">
                    Merged Configuration Preview
                  </Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <Typography variant="body2" color="textSecondary" paragraph>
                    This is the final configuration that will be passed to the plugin
                    when it executes for this LLM (base + overrides).
                  </Typography>
                  <Box
                    component="pre"
                    sx={{
                      backgroundColor: 'grey.100',
                      p: 2,
                      borderRadius: 1,
                      overflow: 'auto',
                      fontSize: '0.875rem',
                      fontFamily: 'monospace',
                    }}
                  >
                    {JSON.stringify(mergedConfig, null, 2)}
                  </Box>
                </AccordionDetails>
              </Accordion>
            )}
          </Box>
        )}
      </DialogContent>

      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          Cancel
        </Button>
        <Button
          onClick={handleSave}
          variant="contained"
          disabled={saving || loading}
          startIcon={saving ? <CircularProgress size={16} /> : null}
        >
          {saving ? 'Saving...' : 'Save Configuration'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default PluginConfigDialog;