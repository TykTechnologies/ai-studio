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
  Box
} from "@mui/material";
import { PrimaryButton, SecondaryLinkButton } from "../../../styles/sharedStyles";

import SelectProvider from "./components/SelectProvider";
import SelectConnection from "./components/SelectConnection";
import SelectAPI from "./components/SelectAPI";
import ConfigureTool from "./components/ConfigureTool";
import DirectImportSpec from "./components/DirectImportSpec";
import { useTykImport } from "./hooks/useTykImport";
import { useToolCreation } from "./hooks/useToolCreation";
import yaml from "js-yaml";
import { detectFormat, extractOperations, extractAuthDetails, validateSpec } from "./utils/specUtils";
import { STEPS, STEP_SEQUENCES, STEP_LABELS, IMPORT_METHODS } from "./constants";

const emptyToolConfig = () => ({
  name: "",
  description: "",
  tool_type: "REST",
  oas_spec: "",
  // 25 is the top of the Public band, the same default the quick-start
  // uses; the create hook falls back to it too.
  privacy_score: 25,
  auth_schema_name: "",
  auth_key: "",
  auth_key_prefilled: false,
  operations: [],
});

/**
 * ImportOpenAPIWizard creates a tool from an OpenAPI document: pasted,
 * uploaded, fetched from a URL, or read from a Tyk Dashboard through a saved
 * Tyk connection (Enterprise).
 */
const ImportOpenAPIWizard = ({ open, onClose, onImport }) => {
  const [activeStep, setActiveStep] = useState(STEPS.SELECT_PROVIDER);
  const [selectedProvider, setSelectedProvider] = useState(null);
  const [connection, setConnection] = useState(null);
  const [selectedAPI, setSelectedAPI] = useState(null);
  const [directSpec, setDirectSpec] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [toolConfig, setToolConfig] = useState(emptyToolConfig());

  const tyk = useTykImport();

  const {
    createTool,
    loading: toolLoading,
    error: toolError
  } = useToolCreation();

  const sequence = selectedProvider ? STEP_SEQUENCES[selectedProvider.type] : [STEPS.SELECT_PROVIDER];

  const getSteps = () => sequence.map((step) => STEP_LABELS[step]);

  const getNextStep = (currentStep) => sequence[sequence.indexOf(currentStep) + 1];

  const selectProvider = (provider) => {
    setSelectedProvider(provider);
    setConnection(null);
    setSelectedAPI(null);
    tyk.reset();
  };

  const selectConnection = (c) => {
    setConnection(c);
    setSelectedAPI(null);
  };

  const handleNext = async () => {
    try {
      switch (activeStep) {
        case STEPS.SELECT_PROVIDER:
          if (!selectedProvider) {
            throw new Error("Please select a provider");
          }
          setActiveStep(getNextStep(STEPS.SELECT_PROVIDER));
          break;

        case STEPS.SELECT_CONNECTION:
          if (!connection) {
            throw new Error("Please choose a connection");
          }
          await tyk.loadAPIs(connection.id);
          setActiveStep(STEPS.SELECT_API);
          break;

        case STEPS.SELECT_API: {
          if (!selectedAPI) {
            throw new Error("Please select an API");
          }
          // Fetch the definition (credentials masked server-side) and read
          // what the tool form needs from it, like the direct import does.
          const doc = await tyk.loadDocument(connection.id, selectedAPI.api_id);
          const spec = JSON.stringify(doc.definition, null, 2);
          const operations = extractOperations(spec, "json");
          const securityDetails = extractAuthDetails(spec, "json");
          setSelectedAPI({ ...selectedAPI, security_details: securityDetails });
          setToolConfig({
            ...toolConfig,
            name: doc.name || selectedAPI.name || "",
            description: doc.definition?.info?.description || "",
            tool_type: "REST",
            oas_spec: spec,
            auth_schema_name: securityDetails.name || "",
            auth_key: "",
            auth_key_prefilled: false,
            operations,
          });
          setActiveStep(STEPS.CONFIGURE_TOOL);
          break;
        }

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
            } else if (directSpec.type === 'paste') {
              // For pasted content, use the spec directly
              spec = directSpec.spec;
              
              // Detect format and validate
              const format = detectFormat(spec);
              validateSpec(spec, format);
              
              // Extract operations and auth details
              directSpec.operations = extractOperations(spec, format);
              directSpec.security_details = extractAuthDetails(spec, format);
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

  const getPreviousStep = (currentStep) => sequence[sequence.indexOf(currentStep) - 1] ?? STEPS.SELECT_PROVIDER;

  const handleBack = () => {
    setActiveStep(getPreviousStep(activeStep));
  };

  const handleClose = () => {
    tyk.reset();
    setSelectedProvider(null);
    setActiveStep(STEPS.SELECT_PROVIDER);
    setConnection(null);
    setSelectedAPI(null);
    setDirectSpec(null);
    setError("");
    setToolConfig(emptyToolConfig());
    onClose();
  };

  const renderStepContent = () => {
    switch (activeStep) {
      case STEPS.SELECT_PROVIDER:
        return (
          <SelectProvider
            providers={IMPORT_METHODS}
            selectedProvider={selectedProvider}
            onSelect={selectProvider}
            loading={false}
            error=""
          />
        );

      case STEPS.SELECT_CONNECTION:
        return (
          <SelectConnection
            status={tyk.status}
            connections={tyk.connections}
            loading={tyk.loading}
            error={tyk.error}
            selected={connection}
            onSelect={selectConnection}
            reload={tyk.loadConnections}
          />
        );

      case STEPS.SELECT_API:
        return (
          <SelectAPI
            apis={tyk.apis}
            selectedAPI={selectedAPI}
            onSelect={setSelectedAPI}
            loading={tyk.loading}
            error={tyk.error}
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

  const getCurrentStepIndex = () => (selectedProvider ? sequence.indexOf(activeStep) : 0);

  // The connection step cannot advance while the integration is missing or
  // off; the step itself explains why.
  const tykBlocked =
    activeStep === STEPS.SELECT_CONNECTION && tyk.status && (!tyk.status.available || !tyk.status.enabled);

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle>Import OpenAPI Specification</DialogTitle>
      <DialogContent>
        <Stepper activeStep={getCurrentStepIndex()} sx={{ mb: 4 }}>
          {getSteps().map((label) => (
            <Step key={label}>
              <StepLabel>{label}</StepLabel>
            </Step>
          ))}
        </Stepper>

        {renderStepContent()}
      </DialogContent>
      <DialogActions sx={{ justifyContent: 'space-between', px: 3 }}>
        <Box>
          <SecondaryLinkButton onClick={handleClose}>Cancel</SecondaryLinkButton>
        </Box>
        <Box sx={{ display: 'flex', gap: 1 }}>
          {getCurrentStepIndex() > 0 && <Button onClick={handleBack}>Back</Button>}
          <PrimaryButton
            variant="contained"
            onClick={handleNext}
            disabled={tyk.loading || toolLoading || loading || Boolean(tykBlocked)}
            data-testid="wizard-next"
          >
            {activeStep === STEPS.CONFIGURE_TOOL ? 'Create Tool' : 'Next'}
          </PrimaryButton>
        </Box>
      </DialogActions>
    </Dialog>
  );
};

export default ImportOpenAPIWizard;
