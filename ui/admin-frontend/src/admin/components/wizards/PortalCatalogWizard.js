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
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Typography,
} from "@mui/material";
import apiClient from "../../utils/apiClient";
import { getVendorCodes, getVendorName } from "../../utils/vendorLogos";
import modelPresets from "../../utils/modelPresets";

const PortalCatalogWizard = ({ open, onClose, fetchData }) => {
  const [activeStep, setActiveStep] = useState(0);
  const [formData, setFormData] = useState({
    vendor: "",
    vendorName: "My LLM",
    privacyLevel: 0,
    apiEndpoint: "",
    apiKey: "",
    modelPreset: "default",
    systemPrompt:
      "You are a helpful AI assistant. Answer the user's questions to the best of your ability.",
    groupName: "My Group",
    catalogueName: "My Catalog",
  });

  const [createdCatalogueId, setCreatedCatalogueId] = useState(null);

  const steps = [
    "LLM Vendor Details",
    "Call Settings",
    "Team & Catalogue Details",
    "Completion",
  ];

  const handleNext = () => {
    setActiveStep((prevActiveStep) => prevActiveStep + 1);
  };

  const handleBack = () => {
    setActiveStep((prevActiveStep) => prevActiveStep - 1);
  };

  const handleInputChange = (event) => {
    const { name, value } = event.target;
    setFormData((prevData) => ({
      ...prevData,
      [name]: value,
    }));

    if (name === "vendor") {
      setFormData((prevData) => ({
        ...prevData,
        vendorName: getVendorName(value),
        apiEndpoint:
          value === "openai" || value === "anthropic"
            ? ""
            : prevData.apiEndpoint,
      }));
    }
  };

  const handleSubmit = async () => {
    try {
      // Step 1: Create LLM
      const llmResponse = await apiClient.post("/llms", {
        data: {
          type: "LLM",
          attributes: {
            name: formData.vendorName,
            vendor: formData.vendor,
            privacy_score: formData.privacyLevel,
            api_endpoint: formData.apiEndpoint,
            api_key: formData.apiKey,
            active: true,
            short_description: "A helpful AI assistant",
            long_description:
              "This LLM is designed to assist users with various tasks and answer questions across a wide range of topics.",
          },
        },
      });

      const llmId = llmResponse.data.data.id;

      // Step 2: Create LLM Settings
      const preset = modelPresets[formData.modelPreset];
      const llmSettingsResponse = await apiClient.post("/llm-settings", {
        data: {
          type: "LLMSettings",
          attributes: {
            ...preset,
            system_prompt: formData.systemPrompt,
          },
        },
      });

      const llmSettingsId = llmSettingsResponse.data.data.id;

      // Step 3: Create Group
      const groupResponse = await apiClient.post("/groups", {
        data: {
          type: "Group",
          attributes: {
            name: formData.groupName,
          },
        },
      });

      const groupId = groupResponse.data.data.id;

      // Step 4: Create Catalogue
      const catalogueResponse = await apiClient.post("/catalogues", {
        data: {
          type: "Catalogue",
          attributes: {
            name: formData.catalogueName,
          },
        },
      });

      const catalogueId = catalogueResponse.data.data.id;

      // Step 5: Associate LLM with Catalogue
      await apiClient.post(`/catalogues/${catalogueId}/llms`, {
        data: { id: llmId, type: "LLM" },
      });

      // Step 6: Associate Catalogue with Group
      await apiClient.post(`/groups/${groupId}/catalogues`, {
        data: { id: catalogueId, type: "Catalogue" },
      });

      setCreatedCatalogueId(catalogueId);
      setActiveStep((prevStep) => prevStep + 1);
      await fetchData();
    } catch (error) {
      console.error("Error creating portal catalog:", error);
      // Handle error (e.g., show error message to user)
    }
  };

  const renderStepContent = (step) => {
    switch (step) {
      case 0:
        return (
          <>
            <FormControl fullWidth margin="normal">
              <InputLabel>Vendor</InputLabel>
              <Select
                name="vendor"
                value={formData.vendor}
                onChange={handleInputChange}
                required
              >
                {getVendorCodes().map((vendorCode) => (
                  <MenuItem key={vendorCode} value={vendorCode}>
                    {getVendorName(vendorCode)}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <TextField
              fullWidth
              margin="normal"
              label="Vendor Name"
              name="vendorName"
              value={formData.vendorName}
              onChange={handleInputChange}
              required
            />
            <TextField
              fullWidth
              margin="normal"
              label="Privacy Level"
              name="privacyLevel"
              type="number"
              value={formData.privacyLevel}
              onChange={handleInputChange}
              required
            />
            {formData.vendor !== "openai" &&
              formData.vendor !== "anthropic" && (
                <TextField
                  fullWidth
                  margin="normal"
                  label="API Endpoint"
                  name="apiEndpoint"
                  value={formData.apiEndpoint}
                  onChange={handleInputChange}
                  required
                />
              )}
            <TextField
              fullWidth
              margin="normal"
              label="API Key"
              name="apiKey"
              type="password"
              value={formData.apiKey}
              onChange={handleInputChange}
              required
            />
          </>
        );
      case 1:
        return (
          <>
            <FormControl fullWidth margin="normal">
              <InputLabel>Model Preset</InputLabel>
              <Select
                name="modelPreset"
                value={formData.modelPreset}
                onChange={handleInputChange}
                required
              >
                {Object.keys(modelPresets).map((preset) => (
                  <MenuItem key={preset} value={preset}>
                    {preset}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <TextField
              fullWidth
              margin="normal"
              label="System Prompt"
              name="systemPrompt"
              multiline
              rows={4}
              value={formData.systemPrompt}
              onChange={handleInputChange}
            />
          </>
        );
      case 2:
        return (
          <>
            <TextField
              fullWidth
              margin="normal"
              label="Team Name"
              name="groupName"
              value={formData.groupName}
              onChange={handleInputChange}
              required
            />
            <TextField
              fullWidth
              margin="normal"
              label="Catalogue Name"
              name="catalogueName"
              value={formData.catalogueName}
              onChange={handleInputChange}
              required
            />
          </>
        );
      case 3:
        return (
          <>
            <Typography variant="h6" gutterBottom>
              Portal Catalog Created Successfully!
            </Typography>
            <Typography variant="body1" paragraph>
              Your new portal catalog has been set up and is ready to use.
            </Typography>
            <Button
              variant="contained"
              color="primary"
              onClick={() => {
                window.open(`/catalogues/${createdCatalogueId}`, "_blank");
                onClose();
              }}
            >
              Take me to the Portal
            </Button>
          </>
        );
      default:
        return null;
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Create Portal Catalog</DialogTitle>
      <DialogContent>
        <Stepper activeStep={activeStep} alternativeLabel>
          {steps.map((label) => (
            <Step key={label}>
              <StepLabel>{label}</StepLabel>
            </Step>
          ))}
        </Stepper>
        <Typography sx={{ mt: 2, mb: 1 }}>
          {activeStep === steps.length - 1
            ? "Completion"
            : `Step ${activeStep + 1}`}
        </Typography>
        {renderStepContent(activeStep)}
      </DialogContent>
      <DialogActions>
        {activeStep !== steps.length - 1 && (
          <>
            <Button onClick={onClose}>Cancel</Button>
            <Button disabled={activeStep === 0} onClick={handleBack}>
              Back
            </Button>
            {activeStep === steps.length - 2 ? (
              <Button
                onClick={handleSubmit}
                variant="contained"
                color="primary"
              >
                Create Catalog
              </Button>
            ) : (
              <Button onClick={handleNext} variant="contained" color="primary">
                Next
              </Button>
            )}
          </>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default PortalCatalogWizard;
