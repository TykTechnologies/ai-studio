import React, { useState, useEffect } from 'react';
import {
  TextField,
  Box,
  Typography,
  Grid,
  Snackbar,
  Alert,
  AlertTitle,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormControlLabel,
  Switch,
  AccordionSummary,
  AccordionDetails,
  Button,
  CircularProgress,
  Chip,
  Checkbox,
  ListItemText,
  FormHelperText,
} from '@mui/material';
import { useNavigate, useParams, Link } from 'react-router-dom';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import LockIcon from '@mui/icons-material/Lock';
import StarIcon from '@mui/icons-material/Star';
import EditIcon from '@mui/icons-material/Edit';
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
} from '../../styles/sharedStyles';
import pluginService, { PluginService } from '../../services/pluginService';
import EdgeAvailabilitySection from '../common/EdgeAvailabilitySection';
import PluginConfigurationSection from './PluginConfigurationSection';
import ScopeReviewSection from './ScopeReviewSection';

const PluginForm = ({ mode = 'create' }) => {
  const { id } = useParams();
  const navigate = useNavigate();
  const isEdit = mode === 'edit' && id;
  
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    command: '',
    config: {},
    hookType: '',                   // Primary hook (required)
    hookTypes: [],                  // Additional hooks
    manifestHookTypes: [],          // From manifest (read-only)
    hookTypesCustomized: false,     // User overrode manifest
    isActive: true,
    namespace: '',
    ociReference: '',
    loadImmediately: false,
  });
  
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success',
  });

  // Configuration as JSON string for editing
  const [configJson, setConfigJson] = useState('{}');
  const [configError, setConfigError] = useState(null);

  // Accordion expansion state
  const [accordionExpanded, setAccordionExpanded] = useState(false);

  // Command change detection state
  const [originalCommand, setOriginalCommand] = useState('');
  const [requiresReapproval, setRequiresReapproval] = useState(false);
  const [showCommandChangeWarning, setShowCommandChangeWarning] = useState(false);
  const [showScopeApproval, setShowScopeApproval] = useState(false);
  const [extractedScopes, setExtractedScopes] = useState([]);
  const [scopeLoading, setScopeLoading] = useState(false);

  useEffect(() => {
    if (isEdit) {
      fetchPlugin();
    }
  }, [id, isEdit]);

  const fetchPlugin = async () => {
    try {
      const plugin = await pluginService.getPlugin(id);
      if (plugin) {
        setFormData({
          name: plugin.name,
          description: plugin.description,
          command: plugin.command,
          config: plugin.config || {},
          hookType: plugin.hookType || '',
          hookTypes: plugin.hookTypes || [],
          manifestHookTypes: [], // Will be populated if we load metadata
          hookTypesCustomized: plugin.hookTypesCustomized || false,
          isActive: plugin.isActive,
          namespace: plugin.namespace === 'global' ? '' : plugin.namespace,
          ociReference: plugin.ociReference || '',
          loadImmediately: false,
        });
        setConfigJson(JSON.stringify(plugin.config || {}, null, 2));

        // Store original command for change detection
        setOriginalCommand(plugin.command);
      }
    } catch (error) {
      console.error('Error fetching plugin:', error);
      setSnackbar({
        open: true,
        message: 'Failed to fetch plugin details',
        severity: 'error',
      });
    }
  };

  const handleInputChange = (field) => (event) => {
    const { name, value } = event.target;
    const fieldName = name || field;

    setFormData(prev => ({
      ...prev,
      [fieldName]: value
    }));

    // Command change detection for edit mode
    // Check if plugin has UI or Agent hooks (which need scope approval)
    if (fieldName === 'command' && isEdit && value !== originalCommand) {
      const hasUIOrAgent = formData.hookType === 'studio_ui' || formData.hookType === 'agent' ||
                          formData.hookTypes.includes('studio_ui') || formData.hookTypes.includes('agent');
      if (hasUIOrAgent) {
        setRequiresReapproval(true);
        setShowCommandChangeWarning(true);
        setShowScopeApproval(false);
        // Reset any previous scope approval state
        setExtractedScopes([]);
      }
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFormData({ ...formData, [name]: value });
  };

  const handleSwitchChange = (field) => (event) => {
    setFormData(prev => ({
      ...prev,
      [field]: event.target.checked
    }));
  };

  const handleConfigChange = (configData) => {
    setConfigError(null);

    if (typeof configData === 'string') {
      // Handle raw JSON string (from JSON editor)
      const value = configData;
      setConfigJson(value);

      try {
        const parsed = JSON.parse(value);
        setFormData(prev => ({ ...prev, config: parsed }));
      } catch (err) {
        setConfigError('Invalid JSON format');
      }
    } else {
      // Handle parsed config object (from schema form)
      setFormData(prev => ({ ...prev, config: configData }));
      setConfigJson(JSON.stringify(configData || {}, null, 2));
    }
  };

  const handleAccordionChange = (event, isExpanded) => {
    setAccordionExpanded(isExpanded);
  };

  const handleNamespaceChange = (namespaces) => {
    // Convert array to comma-delimited string, or empty string for global
    const namespaceString = Array.isArray(namespaces) ? namespaces.join(', ') : namespaces;
    setFormData(prev => ({
      ...prev,
      namespace: namespaceString
    }));
  };

  // Command change handlers
  const handleLoadNewScopes = async () => {
    setScopeLoading(true);
    try {
      // Call validate-and-load API to get new scopes and hook types
      const response = await pluginService.validateAndLoadPlugin(id, {
        command: formData.command,
      });

      const attrs = response.data.attributes;
      setExtractedScopes(attrs.scopes || []);

      // Update hook types from manifest
      if (attrs.hook_types && attrs.hook_types.length > 0) {
        setFormData(prev => ({
          ...prev,
          hookType: attrs.primary_hook || attrs.hook_types[0],
          hookTypes: attrs.hook_types,
          manifestHookTypes: attrs.hook_types,
          hookTypesCustomized: false,
        }));
      }

      setShowScopeApproval(true);
      setShowCommandChangeWarning(false);
    } catch (error) {
      console.error('Error loading new scopes:', error);
      setSnackbar({
        open: true,
        message: error.message || 'Failed to load plugin metadata',
        severity: 'error',
      });
    } finally {
      setScopeLoading(false);
    }
  };

  const handleScopeApproval = async (approved) => {
    if (approved) {
      setScopeLoading(true);
      try {
        await pluginService.approvePluginScopes(id, true);
        setRequiresReapproval(false);
        setShowScopeApproval(false);
        setSnackbar({
          open: true,
          message: 'Plugin scopes approved successfully',
          severity: 'success',
        });
      } catch (error) {
        console.error('Error approving scopes:', error);
        setSnackbar({
          open: true,
          message: error.message || 'Failed to approve scopes',
          severity: 'error',
        });
      } finally {
        setScopeLoading(false);
      }
    } else {
      // User declined - revert command or navigate away
      setFormData(prev => ({ ...prev, command: originalCommand }));
      setRequiresReapproval(false);
      setShowScopeApproval(false);
      setShowCommandChangeWarning(false);
      setSnackbar({
        open: true,
        message: 'Command reverted to original value',
        severity: 'info',
      });
    }
  };

  const validateForm = () => {
    const newErrors = {};
    if (!formData.name.trim()) newErrors.name = 'Plugin name is required';

    // Validate command (auto-detect OCI vs local from prefix)
    if (!formData.command.trim()) {
      newErrors.command = 'Plugin command is required';
    } else if (formData.command.startsWith('oci://') && formData.ociReference && !formData.ociReference.trim()) {
      newErrors.command = 'OCI reference cannot be empty for OCI plugins';
    }

    // Hook type is always required
    if (!formData.hookType) {
      newErrors.hookType = 'Primary hook type is required';
    }
    if (configError) newErrors.config = configError;

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (event) => {
    event.preventDefault();

    if (!validateForm()) {
      return;
    }

    try {
      // Prepare submission data with hook types (camelCase for service layer)
      const submissionData = {
        name: formData.name,
        description: formData.description,
        command: formData.command,
        hookType: formData.hookType,
        hookTypes: formData.hookTypes.length > 0 ? formData.hookTypes : [formData.hookType],
        hookTypesCustomized: formData.hookTypesCustomized,
        config: formData.config,
        isActive: formData.isActive,
        namespace: formData.namespace,
        ociReference: formData.ociReference,
      };

      if (isEdit) {
        await pluginService.updatePlugin(id, submissionData);
      } else {
        await pluginService.createPlugin(submissionData);
      }

      setSnackbar({
        open: true,
        message: isEdit ? 'Plugin updated successfully' : 'Plugin created successfully',
        severity: 'success',
      });

      setTimeout(() => navigate('/admin/plugins'), 2000);
    } catch (error) {
      console.error('Error saving plugin:', error);
      setSnackbar({
        open: true,
        message: 'Failed to save plugin. Please try again.',
        severity: 'error',
      });
    }
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === 'clickaway') {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const availableHookTypes = pluginService.getAvailableHookTypes();

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          {isEdit ? 'Edit plugin' : 'Add plugin'}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/plugins"
          color="inherit"
        >
          Back to plugins
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          Plugins extend the microgateway with custom logic at various points in the request lifecycle.
          Each plugin is executed at a specific hook point (pre-auth, auth, post-auth, or response processing).
        </Typography>  
      </Box>
      <ContentBox>

        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Plugin Name"
                name="name"
                value={formData.name}
                onChange={handleChange}
                error={!!errors.name}
                helperText={errors.name || 'Display name for the plugin'}
                required
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Description"
                name="description"
                value={formData.description}
                onChange={handleChange}
                multiline
                rows={3}
                helperText="Optional description of what this plugin does"
              />
            </Grid>
            {/* Primary Hook Type - Required */}
            <Grid item xs={12}>
              <FormControl fullWidth required error={!!errors.hookType}>
                <InputLabel>Primary Hook Type</InputLabel>
                <Select
                  name="hookType"
                  value={formData.hookType}
                  label="Primary Hook Type"
                  onChange={handleChange}
                  disabled={!formData.hookTypesCustomized && formData.manifestHookTypes.length > 0}
                >
                  {availableHookTypes.map((hookType) => (
                    <MenuItem key={hookType.value} value={hookType.value}>
                      <Box>
                        <Typography variant="body1">{hookType.label}</Typography>
                        <Typography variant="caption" color="textSecondary">
                          {PluginService.HOOK_TYPE_DESCRIPTIONS[hookType.value]}
                        </Typography>
                      </Box>
                    </MenuItem>
                  ))}
                </Select>
                {errors.hookType && (
                  <Typography variant="caption" color="error" sx={{ mt: 0.5, ml: 1.5 }}>
                    {errors.hookType}
                  </Typography>
                )}
                <FormHelperText>
                  {formData.manifestHookTypes.length > 0 && !formData.hookTypesCustomized
                    ? "From plugin manifest (click Customize to edit)"
                    : "The primary capability of this plugin"}
                </FormHelperText>
              </FormControl>
            </Grid>

            {/* Additional Hook Types */}
            <Grid item xs={12}>
              {!formData.hookTypesCustomized && formData.manifestHookTypes.length > 0 ? (
                <Box>
                  <Typography variant="body2" color="textSecondary" gutterBottom>
                    Plugin Capabilities (from manifest)
                  </Typography>
                  <Box display="flex" gap={1} flexWrap="wrap" mb={2}>
                    {formData.manifestHookTypes.map(hook => (
                      <Chip
                        key={hook}
                        label={PluginService.HOOK_TYPE_LABELS[hook] || hook}
                        color={hook === formData.hookType ? "primary" : "default"}
                        icon={hook === formData.hookType ? <StarIcon /> : <LockIcon />}
                        size="small"
                      />
                    ))}
                  </Box>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={<EditIcon />}
                    onClick={() => {
                      setFormData(prev => ({
                        ...prev,
                        hookTypesCustomized: true,
                        hookTypes: [...prev.manifestHookTypes],
                      }));
                    }}
                  >
                    Customize Hook Types (Advanced)
                  </Button>
                </Box>
              ) : (
                <Box>
                  {formData.manifestHookTypes.length > 0 && (
                    <Alert severity="warning" sx={{ mb: 2 }}>
                      <AlertTitle>Customizing Hook Types</AlertTitle>
                      You are customizing hook types. Removing hooks declared in the manifest may cause the plugin to malfunction.
                      <Button
                        size="small"
                        onClick={() => {
                          setFormData(prev => ({
                            ...prev,
                            hookTypes: [...prev.manifestHookTypes],
                            hookType: prev.manifestHookTypes[0] || '',
                            hookTypesCustomized: false,
                          }));
                        }}
                        sx={{ ml: 2 }}
                      >
                        Reset to Manifest
                      </Button>
                    </Alert>
                  )}
                  <FormControl fullWidth>
                    <InputLabel>Additional Hook Types</InputLabel>
                    <Select
                      multiple
                      value={formData.hookTypes}
                      onChange={(e) => {
                        const selectedHooks = e.target.value;
                        setFormData(prev => ({
                          ...prev,
                          hookTypes: selectedHooks,
                          // Ensure primary hook is in the list
                          hookType: selectedHooks.includes(prev.hookType)
                            ? prev.hookType
                            : (selectedHooks[0] || ''),
                        }));
                      }}
                      renderValue={(selected) => (
                        <Box display="flex" gap={0.5} flexWrap="wrap">
                          {selected.map(hook => (
                            <Chip
                              key={hook}
                              label={PluginService.HOOK_TYPE_LABELS[hook] || hook}
                              size="small"
                            />
                          ))}
                        </Box>
                      )}
                    >
                      {availableHookTypes.map((hookType) => (
                        <MenuItem key={hookType.value} value={hookType.value}>
                          <Checkbox checked={formData.hookTypes.includes(hookType.value)} />
                          <ListItemText
                            primary={hookType.label}
                            secondary={PluginService.HOOK_TYPE_DESCRIPTIONS[hookType.value]}
                          />
                        </MenuItem>
                      ))}
                    </Select>
                    <FormHelperText>
                      Select all hook types this plugin implements
                    </FormHelperText>
                  </FormControl>
                </Box>
              )}
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Command"
                name="command"
                value={formData.command}
                onChange={handleChange}
                error={!!errors.command}
                helperText={
                  errors.command ||
                  'Plugin command - use oci:// for OCI artifacts, grpc:// for external services, or local path for binaries'
                }
                placeholder="e.g., oci://registry.com/my-plugin:latest or /path/to/plugin-binary"
                required
              />
            </Grid>
            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={formData.isActive}
                    onChange={handleSwitchChange('isActive')}
                    name="isActive"
                    color="primary"
                  />
                }
                label="Active"
              />
            </Grid>
          </Grid>

          {/* Edge Availability Section */}
          <EdgeAvailabilitySection
            value={formData.namespace}
            onChange={handleNamespaceChange}
            defaultExpanded={false}
          />

          {/* Command Change Warning */}
          {showCommandChangeWarning && (
            <Alert severity="warning" sx={{ mb: 3 }}>
              <Box display="flex" alignItems="center" justifyContent="space-between">
                <Box>
                  <Typography variant="body2" fontWeight="medium">
                    Command Changed - Scope Re-approval Required
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    AI Studio plugins require security approval when the command changes.
                    Click "Load New Scopes" to review the new permissions.
                  </Typography>
                </Box>
                <Button
                  onClick={handleLoadNewScopes}
                  variant="outlined"
                  size="small"
                  disabled={scopeLoading}
                  startIcon={scopeLoading ? <CircularProgress size={16} /> : null}
                >
                  {scopeLoading ? 'Loading...' : 'Load New Scopes'}
                </Button>
              </Box>
            </Alert>
          )}

          {/* Scope Approval Section */}
          {showScopeApproval && (
            <Box sx={{ mb: 3, p: 3, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
              <ScopeReviewSection
                scopes={extractedScopes}
                onApprove={() => handleScopeApproval(true)}
                onDeny={() => handleScopeApproval(false)}
                loading={scopeLoading}
                disabled={scopeLoading}
              />
            </Box>
          )}

          {/* Configuration Section */}
          <StyledAccordion
            expanded={accordionExpanded}
            onChange={handleAccordionChange}
          >
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Plugin Configuration</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <PluginConfigurationSection
                pluginId={isEdit ? id : null}
                config={formData.config}
                onConfigChange={handleConfigChange}
                configError={configError}
                isEdit={isEdit}
              />
            </AccordionDetails>
          </StyledAccordion>

          <Box mt={4}>
            <PrimaryButton variant="contained" type="submit">
              {isEdit ? 'Update plugin' : 'Add plugin'}
            </PrimaryButton>
          </Box>
        </Box>
      </ContentBox>
      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={handleCloseSnackbar}
          severity={snackbar.severity}
          sx={{ width: '100%' }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default PluginForm;