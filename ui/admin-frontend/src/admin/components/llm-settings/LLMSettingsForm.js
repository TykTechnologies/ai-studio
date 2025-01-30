import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
  Box,
  Typography,
  Grid,
  Snackbar,
  Alert,
  Tooltip,
  InputAdornment,
  IconButton,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  DialogActions,
  ListItemText,
  ListItemIcon,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
  StyledDialog,
  StyledDialogTitle,
  StyledDialogContent,
} from "../../styles/sharedStyles";
import {
  getVendorName,
  getVendorLogo,
  getVendorCodes,
} from "../../utils/vendorLogos";
import Decimal from "decimal.js";

const TooltipTextField = ({ tooltip, ...props }) => (
  <Tooltip title={tooltip} placement="top-start" arrow>
    <TextField
      {...props}
      InputProps={{
        ...props.InputProps,
        endAdornment: (
          <InputAdornment position="end">
            <IconButton edge="end" size="small">
              <HelpOutlineIcon fontSize="small" />
            </IconButton>
          </InputAdornment>
        ),
      }}
    />
  </Tooltip>
);

const modelPresets = {
  default: {
    model_name: "",
    temperature: 0.7,
    max_tokens: 1024,
    top_p: 1,
    top_k: 50,
    min_length: 0,
    max_length: 4096,
    repetition_penalty: 1,
  },
  "OpenAI GPT-3.5 Turbo": {
    model_name: "gpt-3.5-turbo",
    temperature: 0.7,
    max_tokens: 4096,
    top_p: 1,
    top_k: 0,
    min_length: 0,
    max_length: 16385,
    repetition_penalty: 1,
  },
  "OpenAI GPT-4": {
    model_name: "gpt-4",
    temperature: 0.7,
    max_tokens: 8192,
    top_p: 1,
    top_k: 0,
    min_length: 0,
    max_length: 128000,
    repetition_penalty: 1,
  },
  "OpenAI GPT-4 Turbo": {
    model_name: "gpt-4o-turbo",
    temperature: 0.7,
    max_tokens: 4096,
    top_p: 1,
    top_k: 0,
    min_length: 0,
    max_length: 128000,
    repetition_penalty: 1,
  },
  "OpenAI GPT-4o": {
    model_name: "gpt-4o",
    temperature: 0.7,
    max_tokens: 4096,
    top_p: 1,
    top_k: 0,
    min_length: 0,
    max_length: 128000,
    repetition_penalty: 1,
  },
  "Anthropic Claude 3 Haiku": {
    model_name: "claude-3-haiku-20240307",
    temperature: 0.7,
    max_tokens: 4096,
    top_p: 0.9,
    top_k: 0,
    min_length: 0,
    max_length: 4096,
    repetition_penalty: 1.1,
  },
  "Anthropic Claude 3 Opus": {
    model_name: "claude-3-opus-20240229",
    temperature: 0.7,
    max_tokens: 4096,
    top_p: 0.9,
    top_k: 0,
    min_length: 0,
    max_length: 4096,
    repetition_penalty: 1.1,
  },
  "Anthropic Claude 3 Sonnet": {
    model_name: "claude-3-sonnet-20240229",
    temperature: 0.7,
    max_tokens: 4096,
    top_p: 0.9,
    top_k: 0,
    min_length: 0,
    max_length: 4096,
    repetition_penalty: 1.1,
  },
  "Anthropic Claude 3.5 Sonnet": {
    model_name: "claude-3-5-sonnet-20240620",
    temperature: 0.7,
    max_tokens: 8192,
    top_p: 0.9,
    top_k: 0,
    min_length: 0,
    max_length: 200000,
    repetition_penalty: 1.1,
  },
  "Google Gemini": {
    model_name: "gemini-1.5-pro",
    temperature: 0.9,
    max_tokens: 8192,
    top_p: 1,
    top_k: 40,
    min_length: 0,
    max_length: 2097152,
    repetition_penalty: 1.1,
  },
  "Meta LLama2": {
    model_name: "llama-2-70b",
    temperature: 0.7,
    max_tokens: 4096,
    top_p: 0.9,
    top_k: 40,
    min_length: 0,
    max_length: 16000,
    repetition_penalty: 1.1,
  },
  "Meta LLama3": {
    model_name: "llama-3",
    temperature: 0.7,
    max_tokens: 4096,
    top_p: 0.95,
    top_k: 50,
    min_length: 0,
    max_length: 16000,
    repetition_penalty: 1.05,
  },
};

const LLMSettingsForm = () => {
  const [setting, setSetting] = useState(modelPresets.default);
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [selectedPreset, setSelectedPreset] = useState("default");
  const navigate = useNavigate();
  const { id } = useParams();
  const [openPriceModal, setOpenPriceModal] = useState(false);
  const [modelPrice, setModelPrice] = useState({
    model_name: "",
    vendor: "",
    cpit_million: 0.0, // Cost per million input tokens
    cpt_million: 0.0, // Cost per million output tokens
    currency: "USD",
  });

  const [vendors, setVendors] = useState([]);

  useEffect(() => {
    if (id) {
      fetchSetting();
    }
    fetchVendors();
  }, [id]);

  const fetchVendors = async () => {
    try {
      const response = await apiClient.get("/vendors/llm-drivers");
      setVendors(response.data.data || []);
    } catch (error) {
      console.error("Error fetching vendors", error);
    }
  };

  const fetchSetting = async () => {
    try {
      const response = await apiClient.get(`/llm-settings/${id}`);
      setSetting(response.data.data.attributes);
    } catch (error) {
      console.error("Error fetching LLM Call Settings", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch LLM Call Settings details",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setSetting((prevSetting) => ({
      ...prevSetting,
      [name]:
        name === "system_prompt"
          ? value
          : name === "model_name"
            ? value
            : parseFloat(value) || 0,
    }));
  };

  const handlePresetChange = (e) => {
    const preset = e.target.value;
    setSelectedPreset(preset);
    setSetting(modelPresets[preset]);
  };

  const validateForm = () => {
    const newErrors = {};
    if (!setting.model_name.trim())
      newErrors.model_name = "Model name is required";
    if (setting.temperature < 0 || setting.temperature > 1)
      newErrors.temperature = "Temperature must be between 0 and 1";
    if (setting.max_tokens < 1)
      newErrors.max_tokens = "Max tokens must be at least 1";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const checkModelPrice = async (modelName) => {
    try {
      const response = await apiClient.get("/model-prices");
      const existingPrice = response.data.data.find(
        (price) => price.attributes.model_name === modelName,
      );
      if (!existingPrice) {
        setModelPrice({ ...modelPrice, model_name: modelName });
        setOpenPriceModal(true);
      } else {
        saveSettings();
      }
    } catch (error) {
      console.error("Error checking model price", error);
      setSnackbar({
        open: true,
        message: "Failed to check model price. Please try again.",
        severity: "error",
      });
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    if (!id) {
      await checkModelPrice(setting.model_name);
    } else {
      saveSettings();
    }
  };

  const saveSettings = async () => {
    const settingData = {
      data: {
        type: "LLMSettings",
        attributes: setting,
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/llm-settings/${id}`, settingData);
      } else {
        await apiClient.post("/llm-settings", settingData);
      }

      setSnackbar({
        open: true,
        message: id
          ? "LLM Call Settings updated successfully"
          : "LLM Call Settings created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/llm-settings"), 2000);
    } catch (error) {
      console.error("Error saving LLM Call Settings", error);
      setSnackbar({
        open: true,
        message: "Failed to save LLM Call Settings. Please try again.",
        severity: "error",
      });
    }
  };

  const handleModelPriceChange = (e) => {
    const { name, value } = e.target;
    setModelPrice({
      ...modelPrice,
      [name]: name.includes("million") ? parseFloat(value) || 0 : value,
    });
  };

  const handleSaveModelPrice = async () => {
    try {
      // Use Decimal.js to handle the calculations
      const cpitMillions = new Decimal(modelPrice.cpit_million);
      const cptMillions = new Decimal(modelPrice.cpt_million);

      // Calculate per-token prices
      const cpit = cpitMillions.dividedBy(1000000);
      const cpt = cptMillions.dividedBy(1000000);

      // Convert to JSON-safe decimal strings that won't use scientific notation
      const payload = {
        data: {
          type: "ModelPrice",
          attributes: {
            model_name: modelPrice.model_name,
            vendor: modelPrice.vendor,
            cpit: Number(cpit.toFixed(10)),
            cpt: Number(cpt.toFixed(10)),
            currency: modelPrice.currency,
          },
        },
      };

      // Verify the actual payload being sent
      console.log("Final payload:", JSON.stringify(payload, null, 2));

      await apiClient.post("/model-prices", payload);
      setOpenPriceModal(false);
      saveSettings();
    } catch (error) {
      console.error("Error saving model price", error);
      setSnackbar({
        open: true,
        message: "Failed to save model price. Please try again.",
        severity: "error",
      });
    }
  };

  const renderVendorMenuItem = (vendorCode) => {
    const name = getVendorName(vendorCode);
    const logo = getVendorLogo(vendorCode);

    return (
      <MenuItem value={vendorCode} key={vendorCode}>
        <Box sx={{ display: "flex", alignItems: "center", width: "100%" }}>
          <ListItemIcon>
            <img
              src={logo}
              alt={name}
              style={{
                width: 24,
                height: 24,
                objectFit: "contain",
              }}
              onError={(e) => {
                e.target.onerror = null;
                e.target.src =
                  process.env.PUBLIC_URL + "/images/placeholder-logo.png";
              }}
            />
          </ListItemIcon>
          <ListItemText primary={name} />
        </Box>
      </MenuItem>
    );
  };

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5">
          {id ? "Edit LLM Call Settings" : "Add LLM Call Settings"}
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/llm-settings"
          color="inherit"
        >
          Back to LLM Call Settings
        </Button>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <FormControl fullWidth>
                <InputLabel>Model Preset</InputLabel>
                <Select
                  value={selectedPreset}
                  onChange={handlePresetChange}
                  label="Model Preset"
                >
                  <MenuItem value="default">Custom</MenuItem>
                  {Object.keys(modelPresets)
                    .filter((key) => key !== "default")
                    .map((key) => (
                      <MenuItem key={key} value={key}>
                        {key}
                      </MenuItem>
                    ))}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <TooltipTextField
                fullWidth
                label="Model Name"
                name="model_name"
                value={setting.model_name}
                onChange={handleChange}
                error={!!errors.model_name}
                helperText={errors.model_name}
                required
                tooltip="The name of the language model (e.g., 'gpt-3.5-turbo', 'text-davinci-003')"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <TooltipTextField
                fullWidth
                label="Temperature"
                name="temperature"
                type="number"
                inputProps={{ step: 0.1, min: 0, max: 1 }}
                value={setting.temperature}
                onChange={handleChange}
                error={!!errors.temperature}
                helperText={errors.temperature}
                tooltip="Controls randomness: 0 is deterministic, 1 is very random. Range: 0 to 1"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <TooltipTextField
                fullWidth
                label="Max Tokens"
                name="max_tokens"
                type="number"
                inputProps={{ min: 1 }}
                value={setting.max_tokens}
                onChange={handleChange}
                error={!!errors.max_tokens}
                helperText={errors.max_tokens}
                tooltip="The maximum number of tokens to generate in the response"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <TooltipTextField
                fullWidth
                label="Top P"
                name="top_p"
                type="number"
                inputProps={{ step: 0.1, min: 0, max: 1 }}
                value={setting.top_p}
                onChange={handleChange}
                tooltip="Controls diversity via nucleus sampling: 0.5 means half of all likelihood-weighted options are considered. Range: 0 to 1"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <TooltipTextField
                fullWidth
                label="Top K"
                name="top_k"
                type="number"
                inputProps={{ min: 0 }}
                value={setting.top_k}
                onChange={handleChange}
                tooltip="Controls diversity by limiting to k most likely tokens. 0 means no limit"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <TooltipTextField
                fullWidth
                label="Min Length"
                name="min_length"
                type="number"
                inputProps={{ min: 0 }}
                value={setting.min_length}
                onChange={handleChange}
                tooltip="The minimum number of tokens to generate in the response"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <TooltipTextField
                fullWidth
                label="Max Length"
                name="max_length"
                type="number"
                inputProps={{ min: 1 }}
                value={setting.max_length}
                onChange={handleChange}
                tooltip="The maximum number of overall tokens"
              />
            </Grid>
            <Grid item xs={12}>
              <TooltipTextField
                fullWidth
                label="Repetition Penalty"
                name="repetition_penalty"
                type="number"
                inputProps={{ step: 0.1, min: 1 }}
                value={setting.repetition_penalty}
                onChange={handleChange}
                tooltip="Penalizes repetition: 1.0 means no penalty, >1.0 discourages repetition. Typically between 1.0 and 1.5"
              />
            </Grid>
            <Grid item xs={12}>
              <TooltipTextField
                fullWidth
                label="System Prompt"
                name="system_prompt"
                value={setting.system_prompt}
                onChange={handleChange}
                multiline
                rows={4}
                tooltip="A long-form text prompt that sets the context or behavior for the language model"
              />
            </Grid>
          </Grid>

          <Box mt={4}>
            <StyledButton variant="contained" type="submit">
              {id ? "Update LLM Call Settings" : "Add LLM Call Settings"}
            </StyledButton>
          </Box>
        </Box>
      </ContentBox>
      <StyledDialog
        open={openPriceModal}
        onClose={() => setOpenPriceModal(false)}
      >
        <StyledDialogTitle>Add Model Price</StyledDialogTitle>
        <StyledDialogContent>
          <TextField
            fullWidth
            label="Model Name"
            name="model_name"
            value={modelPrice.model_name}
            disabled
            margin="normal"
          />
          <FormControl fullWidth margin="normal">
            <InputLabel>Vendor</InputLabel>
            <Select
              name="vendor"
              value={modelPrice.vendor}
              onChange={handleModelPriceChange}
              required
            >
              <MenuItem value="">Select Vendor</MenuItem>
              {getVendorCodes().map(renderVendorMenuItem)}
            </Select>
          </FormControl>
          <TextField
            fullWidth
            label="Cost per Million Input Tokens"
            name="cpit_million"
            type="number"
            inputProps={{ step: 0.01, min: 0 }}
            value={modelPrice.cpit_million}
            onChange={handleModelPriceChange}
            required
            margin="normal"
          />
          <TextField
            fullWidth
            label="Cost per Million Output Tokens"
            name="cpt_million"
            type="number"
            inputProps={{ step: 0.01, min: 0 }}
            value={modelPrice.cpt_million}
            onChange={handleModelPriceChange}
            required
            margin="normal"
          />
          <TextField
            fullWidth
            label="Currency"
            name="currency"
            value={modelPrice.currency}
            onChange={handleModelPriceChange}
            required
            margin="normal"
          />
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => setOpenPriceModal(false)}>Cancel</Button>
          <StyledButton onClick={handleSaveModelPrice} color="primary">
            Save
          </StyledButton>
        </DialogActions>
      </StyledDialog>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={handleCloseSnackbar}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default LLMSettingsForm;
