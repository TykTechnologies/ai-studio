import React, { useState, useEffect } from 'react';
import {
  TextField,
  Box,
  Typography,
  Grid,
  Snackbar,
  Alert,
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
} from '@mui/material';
import { useNavigate, useParams, Link } from 'react-router-dom';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
} from '../../styles/sharedStyles';
import pluginService from '../../services/pluginService';
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
    hookType: '',
    isActive: true,
    namespace: '',
    pluginType: 'gateway',
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
          hookType: plugin.hookType,
          isActive: plugin.isActive,
          namespace: plugin.namespace === 'global' ? '' : plugin.namespace,
          pluginType: plugin.pluginType || 'gateway',
          ociReference: plugin.ociReference || '',
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
    if (fieldName === 'command' && isEdit && value !== originalCommand && formData.pluginType === 'ai_studio') {
      setRequiresReapproval(true);
      setShowCommandChangeWarning(true);
      setShowScopeApproval(false);
      // Reset any previous scope approval state
      setExtractedScopes([]);
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
      // Call validate-and-load API to get new scopes
      const response = await pluginService.validateAndLoadPlugin(id, {
        command: formData.command,
      });

      setExtractedScopes(response.data.attributes.scopes || []);
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

    // Hook type is required for gateway plugins, auto-set for AI Studio plugins
    if (formData.pluginType === 'gateway' && !formData.hookType) {
      newErrors.hookType = 'Hook type is required for gateway plugins';
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
      // Prepare form data with auto-set hook type for AI Studio plugins
      const submissionData = { ...formData };
      if (submissionData.pluginType === 'ai_studio') {
        submissionData.hookType = 'studio_ui'; // Auto-set for AI Studio plugins
      }

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
            <Grid item xs={12}>
              <FormControl fullWidth required>
                <InputLabel>Plugin Type</InputLabel>
                <Select
                  name="pluginType"
                  value={formData.pluginType}
                  label="Plugin Type"
                  onChange={handleChange}
                >
                  <MenuItem value="gateway">Gateway Plugin</MenuItem>
                  <MenuItem value="ai_studio">AI Studio Plugin</MenuItem>
                </Select>
              </FormControl>
            </Grid>
            {formData.pluginType === 'gateway' && (
              <Grid item xs={12}>
                <FormControl fullWidth required error={!!errors.hookType}>
                  <InputLabel>Hook Type</InputLabel>
                  <Select
                    name="hookType"
                    value={formData.hookType}
                    label="Hook Type"
                    onChange={handleChange}
                  >
                    {availableHookTypes.map((hookType) => (
                      <MenuItem key={hookType.value} value={hookType.value}>
                        {hookType.label}
                      </MenuItem>
                    ))}
                  </Select>
                  {errors.hookType && (
                    <Typography variant="caption" color="error" sx={{ mt: 0.5, ml: 1.5 }}>
                      {errors.hookType}
                    </Typography>
                  )}
                </FormControl>
              </Grid>
            )}
            {formData.pluginType === 'ai_studio' && (
              <>
                <Grid item xs={12}>
                  <Typography variant="body2" color="textSecondary">
                    AI Studio plugins automatically use the "studio_ui" hook type for UI extensions.
                  </Typography>
                </Grid>
                <Grid item xs={12}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={formData.loadImmediately}
                        onChange={handleSwitchChange('loadImmediately')}
                        name="loadImmediately"
                      />
                    }
                    label="Load Immediately"
                  />
                  <Typography variant="caption" display="block" color="textSecondary" sx={{ mt: 0.5 }}>
                    Automatically load plugin and fetch manifest after creation (recommended for development)
                  </Typography>
                </Grid>
              </>
            )}
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