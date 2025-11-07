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
import { useNavigate, useLocation } from 'react-router-dom';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
} from '../../styles/sharedStyles';
import pluginService from '../../services/pluginService';
import agentService from '../../services/agentService';
import pluginLoaderService from '../../services/pluginLoaderService';
import Step1BasicInfo from './wizard/Step1BasicInfo';
import Step2ScopeApproval from './wizard/Step2ScopeApproval';
import Step3Configuration from './wizard/Step3Configuration';
import Step4AgentConfiguration from './wizard/Step4AgentConfiguration';

const WORKFLOW_STATES = {
  CREATED: 'created',
  LOADING: 'loading',
  SCOPES_PENDING: 'scopes_pending',
  SCOPES_DENIED: 'scopes_denied',
  READY: 'ready',
};

const PluginCreationWizard = () => {
  const navigate = useNavigate();
  const location = useLocation();

  const [activeStep, setActiveStep] = useState(0);
  const [workflowState, setWorkflowState] = useState(WORKFLOW_STATES.CREATED);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });

  // Check if we're coming from marketplace
  const fromMarketplace = location.state?.fromMarketplace || false;
  const marketplaceData = location.state?.marketplaceData || null;

  // Plugin data from each step
  const [pluginData, setPluginData] = useState({
    name: marketplaceData?.name || '',
    slug: '',
    description: marketplaceData?.description || '',
    command: marketplaceData?.oci_reference || '',
    hookType: marketplaceData?.hook_type || '',
    hookTypes: marketplaceData?.hook_types || [],
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

  // Agent ID (if agent is created)
  const [agentId, setAgentId] = useState(null);

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

        // Always go to manifest/scope approval step
        // All plugins must show their manifest for user review (hook types, capabilities, permissions)
        handleNext(); // Go to scope approval step (activeStep becomes 1)
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
      // Determine hook types to use (customized or from manifest)
      const finalHookTypes = configData.hookTypes || pluginData.hookTypes || [];
      const finalPrimaryHook = pluginData.hookType || finalHookTypes[0];

      // Only include hookType in update if we have a valid value
      const updatePayload = {
        config: configData.config,
        hookTypes: finalHookTypes,
        hookTypesCustomized: configData.hookTypesCustomized || false,
        isActive: true,
      };

      // Only include hookType if it's not empty/pending
      if (finalPrimaryHook && finalPrimaryHook !== 'pending') {
        updatePayload.hookType = finalPrimaryHook;
      }

      await pluginService.updatePlugin(pluginId, updatePayload);

      // Update pluginData with the final config for Step 4
      setPluginData(prev => ({
        ...prev,
        config: configData.config,
        hookTypes: configData.hookTypes || prev.hookTypes,
      }));

      // Check if this is an agent plugin
      const hookTypes = configData.hookTypes || pluginData.hookTypes || [];
      const isAgentPlugin = hookTypes.includes('agent');

      if (isAgentPlugin) {
        // Go to agent configuration step
        handleNext();
      } else {
        // Complete without agent configuration
        // For UI plugins, reload the plugin to register UI components
        const supportsUI = hookTypes.includes('studio_ui');
        if (supportsUI) {
          try {
            console.log('Reloading UI plugin to register components...');
            await pluginService.reloadPlugin(pluginId);
            console.log('Plugin reloaded, refreshing UI loader...');
            await pluginLoaderService.refresh();
            console.log('Plugin loader refreshed - UI plugin is now available');
          } catch (refreshErr) {
            console.warn('Failed to reload/refresh plugin:', refreshErr);
          }
        }

        setSnackbar({
          open: true,
          message: 'Plugin created successfully!',
          severity: 'success',
        });

        setTimeout(() => navigate(`/admin/plugins/${pluginId}`), 2000);
      }
    } catch (err) {
      console.error('Error updating plugin configuration:', err);
      setError(err.message || 'Failed to save plugin configuration');
    } finally {
      setLoading(false);
    }
  };

  // Step 4: Handle agent configuration
  const handleAgentConfiguration = async (agentData) => {
    setLoading(true);
    try {
      const createdAgent = await agentService.createAgent(agentData);
      setAgentId(createdAgent.id);

      // For UI plugins, reload the plugin to register UI components
      const hookTypes = pluginData.hookTypes || [];
      const supportsUI = hookTypes.includes('studio_ui');
      if (supportsUI) {
        try {
          console.log('Reloading UI plugin to register components...');
          await pluginService.reloadPlugin(pluginId);
          console.log('Plugin reloaded, refreshing UI loader...');
          await pluginLoaderService.refresh();
          console.log('Plugin loader refreshed - UI plugin is now available');
        } catch (refreshErr) {
          console.warn('Failed to reload/refresh plugin:', refreshErr);
        }
      }

      setSnackbar({
        open: true,
        message: 'Plugin and agent created successfully!',
        severity: 'success',
      });

      // Show completion step with options
      handleNext();
    } catch (err) {
      console.error('Error creating agent:', err);
      setError(err.message || 'Failed to create agent');
    } finally {
      setLoading(false);
    }
  };

  // Step 4: Skip agent configuration
  const handleSkipAgentConfiguration = async () => {
    // For UI plugins, reload the plugin to register UI components
    const hookTypes = pluginData.hookTypes || [];
    const supportsUI = hookTypes.includes('studio_ui');
    if (supportsUI) {
      try {
        console.log('Reloading UI plugin to register components...');
        await pluginService.reloadPlugin(pluginId);
        console.log('Plugin reloaded, refreshing UI loader...');
        await pluginLoaderService.refresh();
        console.log('Plugin loader refreshed - UI plugin is now available');
      } catch (refreshErr) {
        console.warn('Failed to reload/refresh plugin:', refreshErr);
      }
    }

    setSnackbar({
      open: true,
      message: 'Plugin created successfully!',
      severity: 'success',
    });

    setTimeout(() => navigate(`/admin/plugins/${pluginId}`), 2000);
  };

  const handleCloseSnackbar = () => {
    setSnackbar({ ...snackbar, open: false });
  };

  // Determine which steps to show based on plugin characteristics
  // Always show scope/manifest step for all plugins (they need to see hook types and permissions)
  const shouldShowScopeStep = true;

  // Check if plugin has agent hook type for step 4
  const hookTypes = pluginData.hookTypes || [];
  const isAgentPlugin = hookTypes.includes('agent');
  const shouldShowAgentStep = isAgentPlugin;

  // Adjust step labels based on which steps are present
  const getStepLabels = () => {
    const labels = ['Basic Info'];

    if (shouldShowScopeStep) {
      labels.push('Scope Approval');
    }

    labels.push('Configuration');

    if (shouldShowAgentStep) {
      labels.push('Agent Setup');
    }

    return labels;
  };

  const stepLabels = getStepLabels();
  const totalSteps = stepLabels.length;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          {fromMarketplace ? 'Install Plugin from Marketplace' : 'Add Plugin'}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate(fromMarketplace ? '/admin/marketplace' : '/admin/plugins')}
          color="inherit"
        >
          {fromMarketplace ? 'Back to marketplace' : 'Back to plugins'}
        </SecondaryLinkButton>
      </TitleBox>

      <Box sx={{ p: 3 }}>
        {fromMarketplace && marketplaceData && (
          <Alert severity="info" sx={{ mb: 3 }}>
            Installing <strong>{marketplaceData.name}</strong> v{marketplaceData.version} from the marketplace.
            The plugin details have been pre-filled for you.
          </Alert>
        )}
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

          {/* Step 4: Agent Configuration (only for agent plugins) */}
          {activeStep === (shouldShowScopeStep ? 3 : 2) && shouldShowAgentStep && (
            <Step4AgentConfiguration
              pluginId={pluginId}
              pluginData={pluginData}
              onComplete={handleAgentConfiguration}
              onSkip={handleSkipAgentConfiguration}
              loading={loading}
              disabled={loading}
            />
          )}

          {/* Completion Step (after agent creation) */}
          {activeStep === (shouldShowScopeStep ? 4 : 3) && agentId && (
            <Box>
              <Alert
                severity="success"
                sx={{
                  mb: 3,
                  '& .MuiAlert-message': {
                    width: '100%',
                    textAlign: 'center',
                  },
                  display: 'flex',
                  justifyContent: 'center',
                }}
              >
                <Box sx={{ width: '100%' }}>
                  <Typography variant="h6" gutterBottom>
                    Success!
                  </Typography>
                  <Typography variant="body2">
                    Your plugin and agent have been created successfully.
                  </Typography>
                </Box>
              </Alert>

              <Box sx={{ textAlign: 'center', py: 2 }}>
                <Typography variant="body1" sx={{ mb: 3 }}>
                  What would you like to do next?
                </Typography>

                <Box sx={{ display: 'flex', gap: 2, justifyContent: 'center', flexWrap: 'wrap' }}>
                  <SecondaryLinkButton onClick={() => navigate(`/admin/plugins/${pluginId}`)}>
                    View Plugin Details
                  </SecondaryLinkButton>
                  <PrimaryButton onClick={() => navigate(`/chat/agent/${agentId}`)}>
                    Try Agent Now
                  </PrimaryButton>
                </Box>
              </Box>
            </Box>
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