import React, { useState } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Stepper,
  Step,
  StepLabel,
} from "@mui/material";

import SelectProvider from "./components/SelectProvider";
import ConfigureProvider from "./components/ConfigureProvider";
import SelectAPI from "./components/SelectAPI";
import ConfigureTool from "./components/ConfigureTool";
import DirectImportSpec from "./components/DirectImportSpec";
import { useProvider } from "./hooks/useProvider";
import { useToolCreation } from "./hooks/useToolCreation";
import yaml from "js-yaml";
import { detectFormat, extractOperations, extractAuthDetails, validateSpec } from "./utils/specUtils";
import { PROVIDER_TYPES, STEPS, STEP_SEQUENCES, STEP_LABELS } from "./constants";

const ImportOpenAPIWizard = ({ open, onClose, onImport }) => {
  const [activeStep, setActiveStep] = useState(STEPS.SELECT_PROVIDER);
  const [providerConfig, setProviderConfig] = useState({
    url: "",
    token: "",
  });
  const [selectedAPI, setSelectedAPI] = useState(null);
  const [directSpec, setDirectSpec] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [toolConfig, setToolConfig] = useState({
    name: "",
    description: "",
    tool_type: "REST",
    oas_spec: "",
    privacy_score: 50,
    auth_schema_name: "",
    auth_key: "",
    operations: [],
  });

  const {
    loading: providerLoading,
    error: providerError,
    providers,
    selectedProvider,
    apis,
    configureSelectedProvider,
    selectProvider,
    reset: resetProvider
  } = useProvider();

  const {
    createTool,
    loading: toolLoading,
    error: toolError
  } = useToolCreation();

  const getSteps = () => {
    if (!selectedProvider) return [STEP_LABELS[STEPS.SELECT_PROVIDER]];
    const sequence = STEP_SEQUENCES[selectedProvider.type] || [STEPS.SELECT_PROVIDER];
    return sequence.map(step => STEP_LABELS[step]);
  };

  const getNextStep = (currentStep) => {
    if (!selectedProvider) return currentStep;
    
    const sequence = STEP_SEQUENCES[selectedProvider.type];
    const currentIndex = sequence.indexOf(currentStep);
    return sequence[currentIndex + 1];
  };

  const handleNext = async () => {
    try {
      switch (activeStep) {
        case STEPS.SELECT_PROVIDER:
          if (!selectedProvider) {
            throw new Error("Please select a provider");
          }
          const nextStep = selectedProvider.type === PROVIDER_TYPES.TYK_DASHBOARD
            ? STEPS.CONFIGURE_PROVIDER
            : STEPS.DIRECT_IMPORT;
          setActiveStep(nextStep);
          break;

        case STEPS.CONFIGURE_PROVIDER:
          if (!providerConfig.url || !providerConfig.token) {
            throw new Error("Please fill in all fields");
          }
          await configureSelectedProvider(providerConfig);
          setActiveStep(getNextStep(STEPS.CONFIGURE_PROVIDER));
          break;

        case STEPS.SELECT_API:
          if (!selectedAPI) {
            throw new Error("Please select an API");
          }

          // Update tool config with API details
          const newConfig = {
            name: selectedAPI.name?.trim() || "",
            description: selectedAPI.description?.trim() || "",
            tool_type: "REST",
            oas_spec: selectedAPI.spec,
            privacy_score: toolConfig.privacy_score,
            auth_schema_name: selectedAPI.security_details?.name || "",
            auth_key: selectedAPI.auth_key || "",
            operations: selectedAPI.operations || [],
          };

          setToolConfig(newConfig);
          setActiveStep(STEPS.CONFIGURE_TOOL);
          break;

        case STEPS.DIRECT_IMPORT:
          if (!directSpec) {
            throw new Error("Please provide an OpenAPI specification");
          }

          try {
            setLoading(true);
            let spec;
            
            if (directSpec.type === 'url') {
              // Validate URL
              try {
                new URL(directSpec.spec);
              } catch (err) {
                throw new Error("Please enter a valid URL");
              }
              
              // Fetch spec from URL
              const response = await fetch(directSpec.spec);
              if (!response.ok) {
                throw new Error("Failed to fetch specification from URL");
              }
              spec = await response.text();
              
              // Detect format from URL and content
              const format = detectFormat(spec, directSpec.spec);
              validateSpec(spec, format);
              
              // Extract operations and auth details using the detected format
              directSpec.operations = extractOperations(spec, format);
              directSpec.security_details = extractAuthDetails(spec, format);
            } else if (directSpec.type === 'file') {
              // Validate file type
              if (!directSpec.file.name.match(/\.(json|yaml|yml)$/i)) {
                throw new Error("Please upload a JSON or YAML file");
              }
              
              // Read file content
              spec = await new Promise((resolve, reject) => {
                const reader = new FileReader();
                reader.onload = (e) => resolve(e.target.result);
                reader.onerror = () => reject(new Error("Failed to read file"));
                reader.readAsText(directSpec.file);
              });
            } else {
              throw new Error("Invalid specification source");
            }

            // For file uploads, format is already detected and operations/auth details are extracted in DirectImportSpec
            
            // Try to parse spec to get info based on format
            let parsedSpec;
            try {
              const format = directSpec.type === 'file' ? detectFormat(spec, directSpec.file.name) : detectFormat(spec, directSpec.spec);
              parsedSpec = format === 'yaml' ? yaml.load(spec) : JSON.parse(spec);
            } catch (error) {
              throw new Error("Failed to parse specification");
            }

            // Update tool config with all extracted data
            setToolConfig({
              ...toolConfig,
              name: parsedSpec.info?.title || "",
              description: parsedSpec.info?.description || "",
              oas_spec: spec,
              operations: directSpec.operations || [],
              auth_schema_name: directSpec.security_details?.name || "",
              auth_key: "",
            });
            setActiveStep(STEPS.CONFIGURE_TOOL);
          } catch (error) {
            setError(error.message || "Failed to load specification");
            throw error;
          } finally {
            setLoading(false);
          }
          break;

        case STEPS.CONFIGURE_TOOL:
          if (!toolConfig.name?.trim()) {
            throw new Error("Tool name is required");
          }
          if (!toolConfig.description?.trim()) {
            throw new Error("Tool description is required");
          }
          if (!toolConfig.oas_spec) {
            throw new Error("OpenAPI specification is required");
          }

          const result = await createTool(toolConfig);
          onImport(result.data);
          handleClose();
          break;
          
        default:
          throw new Error("Invalid step");
      }
    } catch (error) {
      console.error('Error in wizard step:', error);
    }
  };

  const getPreviousStep = (currentStep) => {
    if (!selectedProvider) return STEPS.SELECT_PROVIDER;
    
    const sequence = STEP_SEQUENCES[selectedProvider.type];
    const currentIndex = sequence.indexOf(currentStep);
    return sequence[currentIndex - 1] ?? STEPS.SELECT_PROVIDER;
  };

  const handleBack = () => {
    setActiveStep(getPreviousStep(activeStep));
  };

  const handleClose = () => {
    resetProvider();
    setActiveStep(STEPS.SELECT_PROVIDER);
    setProviderConfig({ url: "", token: "" });
    setSelectedAPI(null);
    setDirectSpec(null);
    setToolConfig({
      name: "",
      description: "",
      tool_type: "REST",
      oas_spec: "",
      privacy_score: 50,
      auth_schema_name: "",
      auth_key: "",
      operations: [],
    });
    onClose();
  };

  const renderStepContent = () => {
    console.log('activeStep:', activeStep, STEPS);
    switch (activeStep) {
      case STEPS.SELECT_PROVIDER:
        return (
          <SelectProvider
            providers={providers}
            selectedProvider={selectedProvider}
            onSelect={selectProvider}
            loading={providerLoading}
            error={providerError}
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

      case STEPS.DIRECT_IMPORT:
        return (
          <DirectImportSpec
            onSpecProvided={setDirectSpec}
            loading={loading}
            error={error}
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
            importMethod={selectedProvider?.type}
          />
        );

      default:
        return null;
    }
  };

  const getCurrentStepIndex = () => {
    if (!selectedProvider) return 0;
    const sequence = STEP_SEQUENCES[selectedProvider.type];
    return sequence.findIndex(step => step === activeStep);
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle>Import OpenAPI Specification</DialogTitle>
      <DialogContent>
        <Stepper activeStep={getCurrentStepIndex()} sx={{ mb: 4 }}>
          {getSteps().map((label, index) => (
            <Step key={label}>
              <StepLabel>{label}</StepLabel>
            </Step>
          ))}
        </Stepper>

        {renderStepContent()}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        {getCurrentStepIndex() > 0 && <Button onClick={handleBack}>Back</Button>}
        <Button
          variant="contained"
          onClick={handleNext}
          disabled={providerLoading || toolLoading || loading}
        >
          {activeStep === STEPS.CONFIGURE_TOOL ? 'Create Tool' : 'Next'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default ImportOpenAPIWizard;
