import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Stepper,
  Step,
  StepLabel,
  Alert,
  CircularProgress,
  Snackbar,
} from '@mui/material';
import { useNavigate } from 'react-router-dom';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
} from '../../styles/sharedStyles';
import pluginService from '../../services/pluginService';
import Step1BasicInfo from './wizard/Step1BasicInfo';
import Step2ScopeApproval from './wizard/Step2ScopeApproval';
import Step3Configuration from './wizard/Step3Configuration';

const steps = ['Basic Info', 'Scope Approval', 'Configuration'];

const WORKFLOW_STATES = {
  CREATED: 'created',
  LOADING: 'loading',
  SCOPES_PENDING: 'scopes_pending',
  SCOPES_DENIED: 'scopes_denied',
  READY: 'ready',
};

const PluginCreationWizard = () => {
  const navigate = useNavigate();

  const [activeStep, setActiveStep] = useState(0);
  const [workflowState, setWorkflowState] = useState(WORKFLOW_STATES.CREATED);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });

  // Plugin data from each step
  const [pluginData, setPluginData] = useState({
    name: '',
    slug: '',
    description: '',
    command: '',
    hookType: '',
    hookTypes: [],
    manifestHookTypes: [],
    hookTypesCustomized: false,
    isActive: true,
    namespace: '',
    config: {},
  });

  // Metadata loaded from backend
  const [metadataLoading, setMetadataLoading] = useState(false);
  const [metadata, setMetadata] = useState({
    configSchema: null,
    manifest: null,
    scopes: [],
    status: 'created',
  });

  // Plugin ID (once created)
  const [pluginId, setPluginId] = useState(null);

  // Handle step completion and navigation
  const handleNext = () => {
    setActiveStep(prev => prev + 1);
  };

  const handleBack = () => {
    setActiveStep(prev => prev - 1);
  };

  // Step 1: Handle basic info completion and plugin creation
  const handleBasicInfoComplete = async (basicInfo) => {
    setPluginData(prev => ({ ...prev, ...basicInfo }));
    setLoading(true);
    setError(null);

    try {
      // Create the plugin first
      const createdPlugin = await pluginService.createPlugin(basicInfo);
      setPluginId(createdPlugin.id);

      // Load metadata for all plugin types (config schema + manifest)
      setMetadataLoading(true);
      setWorkflowState(WORKFLOW_STATES.LOADING);

      try {
        // Call the validate-and-load endpoint to get metadata
        const response = await pluginService.validateAndLoadPlugin(createdPlugin.id, {
          command: basicInfo.command,
        });

        const attrs = response.data.attributes;

        setMetadata({
          configSchema: attrs.config_schema,
          manifest: attrs.manifest,
          scopes: attrs.scopes || [],
          status: attrs.status,
        });

        // Update hook types from manifest if available
        if (attrs.hook_types && attrs.hook_types.length > 0) {
          setPluginData(prev => ({
            ...prev,
            hookType: attrs.primary_hook || attrs.hook_types[0],
            hookTypes: attrs.hook_types,
            manifestHookTypes: attrs.hook_types,
            hookTypesCustomized: false,
          }));
        }

        setWorkflowState(attrs.status);

        // If there are scopes (UI/Agent plugins), go to approval step
        if (attrs.scopes && attrs.scopes.length > 0) {
          handleNext(); // Go to scope approval step (activeStep becomes 1)
        } else {
          // No scopes, skip to configuration (activeStep becomes 1 since scope step is hidden)
          handleNext();
          setWorkflowState(WORKFLOW_STATES.READY);
        }
      } catch (metadataErr) {
        // If metadata loading fails, log but continue to configuration step
        console.warn('Failed to load plugin metadata:', metadataErr);
        setMetadata({
          configSchema: null,
          manifest: null,
          scopes: [],
          status: 'ready',
        });
        setWorkflowState(WORKFLOW_STATES.READY);
        handleNext(); // Go to configuration step (activeStep becomes 1)
      } finally {
        setMetadataLoading(false);
      }
    } catch (err) {
      console.error('Error creating plugin:', err);
      setError(err.message || 'Failed to create plugin');
      setWorkflowState(WORKFLOW_STATES.CREATED);
    } finally {
      setLoading(false);
      setMetadataLoading(false);
    }
  };

  // Step 2: Handle scope approval
  const handleScopeApproval = async (approved) => {
    if (!approved) {
      // User declined scopes - delete the plugin and exit
      try {
        if (pluginId) {
          await pluginService.deletePlugin(pluginId);
        }
        setSnackbar({
          open: true,
          message: 'Plugin creation cancelled due to scope denial.',
          severity: 'info',
        });
        setTimeout(() => navigate('/admin/plugins'), 2000);
        return;
      } catch (err) {
        console.error('Error cleaning up plugin after scope denial:', err);
      }
    }

    setLoading(true);
    try {
      // Call approve-scopes endpoint
      await pluginService.approvePluginScopes(pluginId, approved);
      setWorkflowState(WORKFLOW_STATES.READY);
      handleNext(); // Go to configuration step
    } catch (err) {
      console.error('Error approving scopes:', err);
      setError(err.message || 'Failed to approve scopes');
    } finally {
      setLoading(false);
    }
  };

  // Step 3: Handle final configuration and completion
  const handleConfigurationComplete = async (configData) => {
    setLoading(true);
    try {
      // Update plugin with final configuration and hook types
      await pluginService.updatePlugin(pluginId, {
        config: configData.config,
        hookType: configData.hookTypes?.[0] || pluginData.hookType,
        hookTypes: configData.hookTypes || pluginData.hookTypes,
        hookTypesCustomized: configData.hookTypesCustomized || false,
        isActive: true,
      });

      setSnackbar({
        open: true,
        message: 'Plugin created successfully!',
        severity: 'success',
      });

      setTimeout(() => navigate(`/admin/plugins/${pluginId}`), 2000);
    } catch (err) {
      console.error('Error updating plugin configuration:', err);
      setError(err.message || 'Failed to save plugin configuration');
    } finally {
      setLoading(false);
    }
  };

  const handleCloseSnackbar = () => {
    setSnackbar({ ...snackbar, open: false });
  };

  // Determine which steps to show based on whether plugin has scopes
  const hasScopes = metadata.scopes && metadata.scopes.length > 0;
  const shouldShowScopeStep = hasScopes;

  // Adjust step labels based on whether scopes are present
  const getStepLabels = () => {
    if (!shouldShowScopeStep) {
      return ['Basic Info', 'Configuration'];
    }
    return ['Basic Info', 'Scope Approval', 'Configuration'];
  };

  const stepLabels = getStepLabels();
  const totalSteps = stepLabels.length;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          Add Plugin
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate('/admin/plugins')}
          color="inherit"
        >
          Back to plugins
        </SecondaryLinkButton>
      </TitleBox>

      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mb: 3 }}>
          Create a new plugin by following these steps. Plugins with UI or Agent capabilities will require scope approval for security.
        </Typography>

        {/* Progress Stepper */}
        <Box sx={{ mb: 4 }}>
          <Stepper activeStep={activeStep} alternativeLabel>
            {stepLabels.map((label, index) => (
              <Step key={label}>
                <StepLabel>{label}</StepLabel>
              </Step>
            ))}
          </Stepper>
        </Box>

        {/* Error Display */}
        {error && (
          <Alert severity="error" sx={{ mb: 3 }}>
            {error}
          </Alert>
        )}

        {/* Loading State */}
        {(loading || metadataLoading) && (
          <Box display="flex" alignItems="center" justifyContent="center" sx={{ mb: 3 }}>
            <CircularProgress size={24} sx={{ mr: 2 }} />
            <Typography variant="body2" color="textSecondary">
              {metadataLoading ? 'Loading plugin metadata...' : 'Processing...'}
            </Typography>
          </Box>
        )}

        <ContentBox>
          {/* Step Content */}
          {activeStep === 0 && (
            <Step1BasicInfo
              data={pluginData}
              onComplete={handleBasicInfoComplete}
              onBack={() => navigate('/admin/plugins')}
              loading={loading || metadataLoading}
              disabled={loading || metadataLoading}
            />
          )}

          {activeStep === 1 && shouldShowScopeStep && (
            <Step2ScopeApproval
              scopes={metadata.scopes}
              manifest={metadata.manifest}
              pluginData={pluginData}
              onApprove={handleScopeApproval}
              onBack={handleBack}
              loading={loading}
              disabled={loading}
            />
          )}

          {activeStep === (shouldShowScopeStep ? 2 : 1) && (
            <Step3Configuration
              pluginId={pluginId}
              pluginData={pluginData}
              configSchema={metadata.configSchema}
              onComplete={handleConfigurationComplete}
              loading={loading}
              disabled={loading}
            />
          )}
        </ContentBox>
      </Box>

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

export default PluginCreationWizard;