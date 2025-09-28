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
    pluginType: 'gateway',
    command: '',
    hookType: '',
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

      // For AI Studio plugins, load metadata
      if (basicInfo.pluginType === 'ai_studio') {
        setMetadataLoading(true);
        setWorkflowState(WORKFLOW_STATES.LOADING);

        // Call the validate-and-load endpoint to get metadata
        const response = await pluginService.validateAndLoadPlugin(createdPlugin.id, {
          command: basicInfo.command,
        });

        setMetadata({
          configSchema: response.data.attributes.config_schema,
          manifest: response.data.attributes.manifest,
          scopes: response.data.attributes.scopes,
          status: response.data.attributes.status,
        });

        setWorkflowState(response.data.attributes.status);

        // If there are scopes, go to approval step
        if (response.data.attributes.scopes.length > 0) {
          handleNext(); // Go to scope approval step
        } else {
          // No scopes, skip to configuration
          setActiveStep(2);
          setWorkflowState(WORKFLOW_STATES.READY);
        }
      } else {
        // Gateway plugins skip scope approval
        setWorkflowState(WORKFLOW_STATES.READY);
        setActiveStep(2); // Go to configuration step
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
  const handleConfigurationComplete = async (config) => {
    setLoading(true);
    try {
      // Update plugin with final configuration
      await pluginService.updatePlugin(pluginId, {
        ...pluginData,
        config,
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

  // Determine which steps to show
  const isGatewayPlugin = pluginData.pluginType === 'gateway';
  const hasScopes = metadata.scopes && metadata.scopes.length > 0;
  const shouldShowScopeStep = !isGatewayPlugin && hasScopes;

  // Adjust step labels based on plugin type
  const getStepLabels = () => {
    if (isGatewayPlugin) {
      return ['Basic Info', 'Configuration'];
    }
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
          Create a new plugin by following these steps. AI Studio plugins will require scope approval for security.
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
              onBack={handleBack}
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