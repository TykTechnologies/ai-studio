import React, { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Stepper,
  Step,
  StepLabel,
} from '@mui/material';

import SelectProvider from './components/SelectProvider';
import ConfigureProvider from './components/ConfigureProvider';
import SelectAPI from './components/SelectAPI';
import ConfigureTool from './components/ConfigureTool';
import { useProvider } from './hooks/useProvider';
import { useToolCreation } from './hooks/useToolCreation';

const STEPS = {
  SELECT_PROVIDER: 0,
  CONFIGURE_PROVIDER: 1,
  SELECT_API: 2,
  CONFIGURE_TOOL: 3,
};

const ImportOpenAPIWizard = ({ open, onClose, onImport }) => {
  const [activeStep, setActiveStep] = useState(STEPS.SELECT_PROVIDER);
  const [providerConfig, setProviderConfig] = useState({
    url: '',
    token: ''
  });
  const [selectedAPI, setSelectedAPI] = useState(null);
  const [toolConfig, setToolConfig] = useState({
    name: '',
    description: '',
    tool_type: 'REST',
    oas_spec: '',
    privacy_score: 50,
    auth_schema_name: '',
    auth_key: '',
    operations: [],
  });

  const {
    loading: providerLoading,
    error: providerError,
    providers,
    selectedProvider,
    apis,
    fetchProviders,
    configureSelectedProvider,
    selectProvider,
    reset: resetProvider
  } = useProvider();

  const {
    createTool,
    loading: toolLoading,
    error: toolError
  } = useToolCreation();

  const handleNext = async () => {
    try {
      switch (activeStep) {
        case STEPS.SELECT_PROVIDER:
          if (!selectedProvider) {
            throw new Error('Please select a provider');
          }
          setActiveStep(STEPS.CONFIGURE_PROVIDER);
          break;

        case STEPS.CONFIGURE_PROVIDER:
          if (!providerConfig.url || !providerConfig.token) {
            throw new Error('Please fill in all fields');
          }
          await configureSelectedProvider(providerConfig);
          setActiveStep(STEPS.SELECT_API);
          break;

        case STEPS.SELECT_API:
          if (!selectedAPI) {
            throw new Error('Please select an API');
          }
          
          // Update tool config with API details
          const newConfig = {
            name: selectedAPI.name?.trim() || '',
            description: selectedAPI.description?.trim() || '',
            tool_type: 'REST',
            oas_spec: selectedAPI.spec,
            privacy_score: toolConfig.privacy_score,
            auth_schema_name: selectedAPI.security_details?.name || '',
            auth_key: selectedAPI.auth_key || '',
            operations: selectedAPI.operations || [],
          };

          setToolConfig(newConfig);
          setActiveStep(STEPS.CONFIGURE_TOOL);
          break;

        case STEPS.CONFIGURE_TOOL:
          if (!toolConfig.name?.trim()) {
            throw new Error('Tool name is required');
          }
          if (!toolConfig.description?.trim()) {
            throw new Error('Tool description is required');
          }
          if (!toolConfig.oas_spec) {
            throw new Error('OpenAPI specification is required');
          }

          const result = await createTool(toolConfig);
          onImport(result.data);
          handleClose();
          break;
      }
    } catch (error) {
      console.error('Error in wizard step:', error);
    }
  };

  const handleBack = () => {
    setActiveStep((prevStep) => prevStep - 1);
  };

  const handleClose = () => {
    resetProvider();
    setActiveStep(STEPS.SELECT_PROVIDER);
    setProviderConfig({ url: '', token: '' });
    setSelectedAPI(null);
    setToolConfig({
      name: '',
      description: '',
      tool_type: 'REST',
      oas_spec: '',
      privacy_score: 50,
      auth_schema_name: '',
      auth_key: '',
      operations: [],
    });
    onClose();
  };

  const renderStepContent = () => {
    switch (activeStep) {
      case STEPS.SELECT_PROVIDER:
        return (
          <SelectProvider
            providers={providers}
            selectedProvider={selectedProvider}
            onSelect={selectProvider}
            loading={providerLoading}
            error={providerError}
            onFetchProviders={fetchProviders}
          />
        );

      case STEPS.CONFIGURE_PROVIDER:
        return (
          <ConfigureProvider
            provider={selectedProvider}
            config={providerConfig}
            onConfigChange={setProviderConfig}
            loading={providerLoading}
            error={providerError}
          />
        );

      case STEPS.SELECT_API:
        return (
          <SelectAPI
            apis={apis}
            selectedAPI={selectedAPI}
            onSelect={setSelectedAPI}
            loading={providerLoading}
            error={providerError}
          />
        );

      case STEPS.CONFIGURE_TOOL:
        return (
          <ConfigureTool
            toolConfig={toolConfig}
            onConfigChange={setToolConfig}
            loading={toolLoading}
            error={toolError}
            selectedAPI={selectedAPI}
          />
        );

      default:
        return null;
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle>Import OpenAPI Specification</DialogTitle>
      <DialogContent>
        <Stepper activeStep={activeStep} sx={{ mb: 4 }}>
          <Step>
            <StepLabel>Select Provider</StepLabel>
          </Step>
          <Step>
            <StepLabel>Configure Provider</StepLabel>
          </Step>
          <Step>
            <StepLabel>Select API</StepLabel>
          </Step>
          <Step>
            <StepLabel>Configure Tool</StepLabel>
          </Step>
        </Stepper>

        {renderStepContent()}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        {activeStep > 0 && <Button onClick={handleBack}>Back</Button>}
        <Button
          variant="contained"
          onClick={handleNext}
          disabled={providerLoading || toolLoading}
        >
          {activeStep === STEPS.CONFIGURE_TOOL ? 'Create Tool' : 'Next'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default ImportOpenAPIWizard;
