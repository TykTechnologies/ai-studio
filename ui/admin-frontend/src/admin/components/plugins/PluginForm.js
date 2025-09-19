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

const PluginForm = ({ mode = 'create' }) => {
  const { id } = useParams();
  const navigate = useNavigate();
  const isEdit = mode === 'edit' && id;
  
  const [formData, setFormData] = useState({
    name: '',
    slug: '',
    description: '',
    command: '',
    checksum: '',
    config: {},
    hookType: '',
    isActive: true,
    namespace: '',
    pluginType: 'gateway',
    ociReference: '',
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
          slug: plugin.slug,
          description: plugin.description,
          command: plugin.command,
          checksum: plugin.checksum || '',
          config: plugin.config || {},
          hookType: plugin.hookType,
          isActive: plugin.isActive,
          namespace: plugin.namespace === 'global' ? '' : plugin.namespace,
          pluginType: plugin.pluginType || 'gateway',
          ociReference: plugin.ociReference || '',
        });
        setConfigJson(JSON.stringify(plugin.config || {}, null, 2));
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
    
    // Auto-generate slug from name if creating new plugin
    if (fieldName === 'name' && !isEdit && !formData.slug) {
      const slug = value
        .toLowerCase()
        .replace(/[^a-z0-9\s-]/g, '')
        .replace(/\s+/g, '-')
        .replace(/-+/g, '-')
        .trim();
      setFormData(prev => ({ ...prev, slug }));
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

  const handleConfigChange = (event) => {
    const value = event.target.value;
    setConfigJson(value);
    setConfigError(null);
    
    try {
      const parsed = JSON.parse(value);
      setFormData(prev => ({ ...prev, config: parsed }));
    } catch (err) {
      setConfigError('Invalid JSON format');
    }
  };

  const handleNamespaceChange = (namespaces) => {
    // Convert array to comma-delimited string, or empty string for global
    const namespaceString = Array.isArray(namespaces) ? namespaces.join(', ') : namespaces;
    setFormData(prev => ({
      ...prev,
      namespace: namespaceString
    }));
  };

  const validateForm = () => {
    const newErrors = {};
    if (!formData.name.trim()) newErrors.name = 'Plugin name is required';
    if (!formData.slug.trim()) newErrors.slug = 'Plugin slug is required';

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
                label="Slug"
                name="slug"
                value={formData.slug}
                onChange={handleChange}
                error={!!errors.slug}
                helperText={errors.slug || 'Unique identifier (auto-generated from name)'}
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
              <Grid item xs={12}>
                <Typography variant="body2" color="textSecondary">
                  AI Studio plugins automatically use the "studio_ui" hook type for UI extensions.
                </Typography>
              </Grid>
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
              <TextField
                fullWidth
                label="Checksum"
                name="checksum"
                value={formData.checksum}
                onChange={handleChange}
                helperText="Optional checksum for plugin integrity verification"
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

          {/* Configuration Section */}
          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Plugin Configuration</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="textSecondary" paragraph>
                Optional JSON configuration that will be passed to the plugin. This can include
                any plugin-specific settings or parameters.
              </Typography>
              
              <TextField
                fullWidth
                label="Configuration JSON"
                value={configJson}
                onChange={handleConfigChange}
                multiline
                rows={8}
                error={!!configError}
                helperText={configError || 'Valid JSON configuration object'}
                sx={{ fontFamily: 'monospace' }}
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