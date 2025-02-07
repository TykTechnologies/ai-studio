import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Stepper,
  Step,
  StepLabel,
  Typography,
  TextField,
  List,
  ListItem,
  ListItemText,
  Box,
  CircularProgress,
  Alert,
} from '@mui/material';
import apiClient from '../../utils/apiClient';

const STEPS = {
  SELECT_PROVIDER: 0,
  CONFIGURE_PROVIDER: 1,
  SELECT_API: 2,
  CONFIGURE_TOOL: 3,
};

const ImportOpenAPIWizard = ({ open, onClose, onImport }) => {
  const [activeStep, setActiveStep] = useState(STEPS.SELECT_PROVIDER);
  const [providers, setProviders] = useState([]);
  const [selectedProvider, setSelectedProvider] = useState(null);
  const [providerConfig, setProviderConfig] = useState({
    url: '',
    token: '',
  });
  const [apis, setApis] = useState([]);
  const [selectedAPI, setSelectedAPI] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [toolConfig, setToolConfig] = useState({
    name: '',
    description: '',
    privacy_score: 50,
    auth_schema_name: '',
    auth_key: '',
  });

  useEffect(() => {
    if (open) {
      fetchProviders();
    }
  }, [open]);

  const fetchProviders = async () => {
    try {
      setLoading(true);
      const response = await apiClient.get('/providers');
      setProviders(response.data.data);
      setError('');
    } catch (error) {
      setError('Failed to fetch providers');
      console.error('Error fetching providers:', error);
    } finally {
      setLoading(false);
    }
  };

  const configureProvider = async () => {
    try {
      setLoading(true);
      await apiClient.post(`/providers/${selectedProvider.id}/configure`, {
        config: providerConfig,
      });
      const response = await apiClient.get(`/providers/${selectedProvider.id}/specs`);
      setApis(response.data.data);
      setError('');
      setActiveStep(STEPS.SELECT_API);
    } catch (error) {
      setError('Failed to configure provider: ' + error.response?.data?.errors?.[0]?.detail || error.message);
      console.error('Error configuring provider:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleNext = async () => {
    setError('');

    switch (activeStep) {
      case STEPS.SELECT_PROVIDER:
        if (!selectedProvider) {
          setError('Please select a provider');
          return;
        }
        setActiveStep(STEPS.CONFIGURE_PROVIDER);
        break;

      case STEPS.CONFIGURE_PROVIDER:
        if (!providerConfig.url || !providerConfig.token) {
          setError('Please fill in all fields');
          return;
        }
        await configureProvider();
        break;

      case STEPS.SELECT_API:
        if (!selectedAPI) {
          setError('Please select an API');
          return;
        }
        setToolConfig(prev => ({
          ...prev,
          name: selectedAPI.name,
          description: selectedAPI.description,
        }));
        setActiveStep(STEPS.CONFIGURE_TOOL);
        break;

      case STEPS.CONFIGURE_TOOL:
        if (!toolConfig.name || !toolConfig.description) {
          setError('Please fill in all required fields');
          return;
        }
        onImport({
          ...toolConfig,
          oas_spec: selectedAPI.spec,
          tool_type: 'REST',
        });
        onClose();
        break;
    }
  };

  const handleBack = () => {
    setActiveStep((prevStep) => prevStep - 1);
    setError('');
  };

  const renderStepContent = () => {
    switch (activeStep) {
      case STEPS.SELECT_PROVIDER:
        return (
          <Box>
            <Typography variant="h6" gutterBottom>
              Select API Provider
            </Typography>
            <List>
              {providers.map((provider) => (
                <ListItem
                  key={provider.id}
                  button
                  selected={selectedProvider?.id === provider.id}
                  onClick={() => setSelectedProvider(provider)}
                >
                  <ListItemText
                    primary={provider.name}
                    secondary={provider.description}
                  />
                </ListItem>
              ))}
            </List>
          </Box>
        );

      case STEPS.CONFIGURE_PROVIDER:
        return (
          <Box>
            <Typography variant="h6" gutterBottom>
              Configure {selectedProvider?.name}
            </Typography>
            <TextField
              fullWidth
              label="URL"
              value={providerConfig.url}
              onChange={(e) =>
                setProviderConfig((prev) => ({ ...prev, url: e.target.value }))
              }
              margin="normal"
              helperText="Enter the provider URL (e.g., http://localhost:9000)"
            />
            <TextField
              fullWidth
              label="Access Token"
              type="password"
              value={providerConfig.token}
              onChange={(e) =>
                setProviderConfig((prev) => ({ ...prev, token: e.target.value }))
              }
              margin="normal"
              helperText="Enter your access token"
            />
          </Box>
        );

      case STEPS.SELECT_API:
        return (
          <Box>
            <Typography variant="h6" gutterBottom>
              Select API
            </Typography>
            <List>
              {apis.map((api) => (
                <ListItem
                  key={api.id}
                  button
                  selected={selectedAPI?.id === api.id}
                  onClick={() => setSelectedAPI(api)}
                >
                  <ListItemText
                    primary={api.name}
                    secondary={api.description}
                  />
                </ListItem>
              ))}
            </List>
          </Box>
        );

      case STEPS.CONFIGURE_TOOL:
        return (
          <Box>
            <Typography variant="h6" gutterBottom>
              Configure Tool
            </Typography>
            <TextField
              fullWidth
              label="Name"
              value={toolConfig.name}
              onChange={(e) =>
                setToolConfig((prev) => ({ ...prev, name: e.target.value }))
              }
              margin="normal"
              required
            />
            <TextField
              fullWidth
              label="Description"
              value={toolConfig.description}
              onChange={(e) =>
                setToolConfig((prev) => ({ ...prev, description: e.target.value }))
              }
              margin="normal"
              multiline
              rows={4}
              required
            />
            <TextField
              fullWidth
              label="Privacy Score"
              type="number"
              value={toolConfig.privacy_score}
              onChange={(e) =>
                setToolConfig((prev) => ({
                  ...prev,
                  privacy_score: parseInt(e.target.value) || 0,
                }))
              }
              margin="normal"
              inputProps={{ min: 0, max: 100 }}
            />
            <TextField
              fullWidth
              label="Auth Schema Name"
              value={toolConfig.auth_schema_name}
              onChange={(e) =>
                setToolConfig((prev) => ({
                  ...prev,
                  auth_schema_name: e.target.value,
                }))
              }
              margin="normal"
              helperText="Name of the security scheme in OpenAPI spec"
            />
            <TextField
              fullWidth
              label="Auth Key"
              type="password"
              value={toolConfig.auth_key}
              onChange={(e) =>
                setToolConfig((prev) => ({ ...prev, auth_key: e.target.value }))
              }
              margin="normal"
              helperText="API key or token for authentication"
            />
          </Box>
        );
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
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

        {loading ? (
          <Box display="flex" justifyContent="center" my={4}>
            <CircularProgress />
          </Box>
        ) : (
          <>
            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}
            {renderStepContent()}
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        {activeStep > 0 && <Button onClick={handleBack}>Back</Button>}
        <Button
          variant="contained"
          onClick={handleNext}
          disabled={loading}
        >
          {activeStep === STEPS.CONFIGURE_TOOL ? 'Create Tool' : 'Next'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default ImportOpenAPIWizard;
