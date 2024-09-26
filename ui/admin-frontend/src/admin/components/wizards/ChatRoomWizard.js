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
  Box,
} from "@mui/material";
import apiClient from "../../utils/apiClient";

import {
  StyledDialog,
  StyledDialogTitle,
  StyledDialogContent,
  StyledButton,
} from "../../styles/sharedStyles";

import { getVendorCodes, getVendorName } from "../../utils/vendorLogos";
import modelPresets from "../../utils/modelPresets";

import { getVendorLogo } from "../../utils/vendorLogos";

const ChatRoomWizard = ({ open, onClose, fetchData }) => {
  const stepHelpText = {
    0: "Configure the LLM vendor details. Select the vendor, set a name for the LLM, and specify the privacy level. For some vendors, you'll need to provide an API endpoint and key.",
    1: "Set up call settings for the LLM. Choose a model preset and customize the system prompt that guides the AI's behavior.",
    2: "Create a group and catalogue for organizing your chat rooms. The group will contain the catalogue, which in turn will contain the LLM.",
    3: "Name your chat room. This is the name that users will see when accessing the chat.",
    4: "Review and confirm your chat room creation.",
  };

  const StepInfoPanel = ({ step }) => (
    <Box
      sx={{
        mt: 2,
        mb: 2,
        p: 2,
        backgroundColor: "rgba(0, 0, 0, 0.03)",
        borderRadius: 1,
      }}
    >
      <Typography variant="body2" color="textSecondary">
        {stepHelpText[step]}
      </Typography>
    </Box>
  );

  const [activeStep, setActiveStep] = useState(0);
  const [error, setError] = useState(null);
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
    chatRoomName: "My Chat Room",
  });

  const [createdChatRoomId, setCreatedChatRoomId] = useState(null);

  const steps = [
    "LLM Vendor Details",
    "Call Settings",
    "Group & Catalogue Details",
    "Chat Room Details",
    "Finish",
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

      // Step 7: Create Chat Room
      const chatRoomResponse = await apiClient.post("/chats", {
        data: {
          type: "Chat",
          attributes: {
            name: formData.chatRoomName,
            llm_settings_id: parseInt(llmSettingsId, 10),
            llm_id: parseInt(llmId, 10),
            group_ids: [parseInt(groupId, 10)],
          },
        },
      });

      const chatRoomId = chatRoomResponse.data.data.id;
      setCreatedChatRoomId(chatRoomId);
      setActiveStep((prevStep) => prevStep + 1);

      await fetchData();
    } catch (error) {
      console.error("Error creating chat room:", error);
      setError("Failed to create chat room. Please try again.");
    }
  };

  const renderStepContent = (step) => {
    return (
      <>
        <StepInfoPanel step={step} />
        {(() => {
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
                          <Box sx={{ display: "flex", alignItems: "center" }}>
                            <img
                              src={getVendorLogo(vendorCode)}
                              alt={getVendorName(vendorCode)}
                              style={{
                                width: 24,
                                height: 24,
                                marginRight: 8,
                                objectFit: "contain",
                              }}
                              onError={(e) => {
                                e.target.onerror = null;
                                e.target.src =
                                  process.env.PUBLIC_URL +
                                  "/images/placeholder-logo.png";
                              }}
                            />
                            {getVendorName(vendorCode)}
                          </Box>
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
                    label="Group Name"
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
                <TextField
                  fullWidth
                  margin="normal"
                  label="Chat Room Name"
                  name="chatRoomName"
                  value={formData.chatRoomName}
                  onChange={handleInputChange}
                  required
                />
              );
            case 4:
              return (
                <Box sx={{ textAlign: "center" }}>
                  <Typography variant="h6" gutterBottom>
                    Chat Room Created Successfully!
                  </Typography>
                  <Button
                    variant="contained"
                    color="primary"
                    onClick={() => {
                      window.open(
                        `/portal/chat/${createdChatRoomId}`,
                        "_blank",
                      );
                      onClose();
                    }}
                  >
                    Take me to this Chat Room
                  </Button>
                </Box>
              );
            default:
              return null;
          }
        })()}
      </>
    );
  };

  return (
    <StyledDialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <StyledDialogTitle>Create Chat Room</StyledDialogTitle>
      <StyledDialogContent>
        <Stepper activeStep={activeStep} alternativeLabel sx={{ mt: 3 }}>
          {steps.map((label) => (
            <Step key={label}>
              <StepLabel>{label}</StepLabel>
            </Step>
          ))}
        </Stepper>
        <Typography sx={{ mt: 2, mb: 1 }}>
          {activeStep === steps.length - 1
            ? "Finish"
            : `Step ${activeStep + 1}`}
        </Typography>
        {renderStepContent(activeStep)}
      </StyledDialogContent>
      <DialogActions>
        {activeStep !== steps.length - 1 && (
          <>
            <Button onClick={onClose}>Cancel</Button>
            <Button disabled={activeStep === 0} onClick={handleBack}>
              Back
            </Button>
          </>
        )}
        {activeStep === steps.length - 1 ? (
          <StyledButton onClick={onClose} variant="contained" color="primary">
            Close
          </StyledButton>
        ) : activeStep === steps.length - 2 ? (
          <StyledButton
            onClick={handleSubmit}
            variant="contained"
            color="primary"
          >
            Create Chat Room
          </StyledButton>
        ) : (
          <StyledButton
            onClick={handleNext}
            variant="contained"
            color="primary"
          >
            Next
          </StyledButton>
        )}
      </DialogActions>
    </StyledDialog>
  );
};

export default ChatRoomWizard;
