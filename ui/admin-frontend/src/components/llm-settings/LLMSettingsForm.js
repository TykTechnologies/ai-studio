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
    max_tokens: 100,
    top_p: 1,
    top_k: 50,
    min_length: 0,
    max_length: 1000,
    repetition_penalty: 1,
  },
  // ... (other presets remain unchanged)
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
    cpt: 0.0,
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
      console.error("Error fetching LLM Setting", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch LLM Setting details",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setSetting((prevSetting) => ({
      ...prevSetting,
      [name]: name === "model_name" ? value : parseFloat(value) || 0,
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
          ? "LLM Setting updated successfully"
          : "LLM Setting created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/llm-settings"), 2000);
    } catch (error) {
      console.error("Error saving LLM Setting", error);
      setSnackbar({
        open: true,
        message: "Failed to save LLM Setting. Please try again.",
        severity: "error",
      });
    }
  };

  const handleModelPriceChange = (e) => {
    const { name, value } = e.target;
    setModelPrice({
      ...modelPrice,
      [name]: name === "cpt" ? parseFloat(value) || 0 : value,
    });
  };

  const handleSaveModelPrice = async () => {
    try {
      await apiClient.post("/model-prices", {
        data: {
          type: "ModelPrice",
          attributes: {
            ...modelPrice,
            cpt: parseFloat(modelPrice.cpt),
            currency: modelPrice.currency,
          },
        },
      });
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
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">
          {id ? "Edit LLM Setting" : "Add LLM Setting"}
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/llm-settings"
          color="white"
        >
          Back to LLM Settings
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
          </Grid>

          <Box mt={4}>
            <StyledButton variant="contained" type="submit">
              {id ? "Update LLM Setting" : "Add LLM Setting"}
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
            label="Cost per Token"
            name="cpt"
            type="number"
            inputProps={{ step: 0.000001, min: 0 }}
            value={modelPrice.cpt}
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
    </StyledPaper>
  );
};

export default LLMSettingsForm;
