import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
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
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
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

const ModelPriceForm = () => {
  const [price, setPrice] = useState({
    model_name: "",
    vendor: "",
    cpit: 0,
    cpt: 0,
    currency: "USD",
  });

  const [displayPrice, setDisplayPrice] = useState({
    cpit_million: 0,
    cpt_million: 0,
  });

  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    if (id) {
      fetchPrice();
    }
  }, [id]);

  const fetchPrice = async () => {
    try {
      const response = await apiClient.get(`/model-prices/${id}`);
      setPrice(response.data.data.attributes);
      // Convert per-token to per-million for display
      setDisplayPrice({
        cpit_million: response.data.data.attributes.cpit * 1000000,
        cpt_million: response.data.data.attributes.cpt * 1000000,
      });
    } catch (error) {
      console.error("Error fetching Model Price", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch Model Price details",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    if (name === "cpit_million" || name === "cpt_million") {
      setDisplayPrice((prev) => ({
        ...prev,
        [name]: parseFloat(value) || 0,
      }));
      // Update the actual price state with per-token values
      const perTokenName = name === "cpit_million" ? "cpit" : "cpt";
      setPrice((prev) => ({
        ...prev,
        [perTokenName]: (parseFloat(value) || 0) / 1000000,
      }));
    } else {
      setPrice((prev) => ({
        ...prev,
        [name]: value,
      }));
    }
  };

  const validateForm = () => {
    const newErrors = {};
    if (!price.model_name.trim())
      newErrors.model_name = "Model name is required";
    if (!price.vendor.trim()) newErrors.vendor = "Vendor is required";
    if (!price.currency.trim()) newErrors.currency = "Currency is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const priceData = {
      data: {
        type: "ModelPrice",
        attributes: price,
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/model-prices/${id}`, priceData);
      } else {
        await apiClient.post("/model-prices", priceData);
      }

      setSnackbar({
        open: true,
        message: id
          ? "Model Price updated successfully"
          : "Model Price created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/model-prices"), 2000);
    } catch (error) {
      console.error("Error saving Model Price", error);
      setSnackbar({
        open: true,
        message: "Failed to save Model Price. Please try again.",
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
          <img
            src={logo}
            alt={name}
            style={{
              width: 24,
              height: 24,
              marginRight: 8,
              objectFit: "contain",
            }}
          />
          <Typography>{name}</Typography>
        </Box>
      </MenuItem>
    );
  };

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          {id ? "Edit model price" : "Add model price"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/model-prices"
          color="inherit"
        >
          Back to model prices
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Model Prices define the cost per million tokens for using different language models. You can set the cost per million tokens for input and output, the provider, and the currency. This helps track usage costs, allowing you to manage and optimize expenses when interacting with different models.</Typography>  
      </Box>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TooltipTextField
                fullWidth
                label="Model Name"
                name="model_name"
                value={price.model_name}
                onChange={handleChange}
                error={!!errors.model_name}
                helperText={errors.model_name}
                required
                autoComplete="off"
                tooltip="The name of the language model (e.g., 'gpt-3.5-turbo', 'text-davinci-003')"
              />
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth error={!!errors.vendor}>
                <InputLabel>Vendor</InputLabel>
                <Select
                  name="vendor"
                  value={price.vendor}
                  onChange={handleChange}
                  required
                >
                  {getVendorCodes().map(renderVendorMenuItem)}
                </Select>
                {errors.vendor && (
                  <Typography color="error" variant="caption">
                    {errors.vendor}
                  </Typography>
                )}
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <TooltipTextField
                fullWidth
                label="Cost per Million Input Tokens"
                name="cpit_million"
                type="number"
                inputProps={{ step: 0.01, min: 0 }}
                value={displayPrice.cpit_million}
                onChange={handleChange}
                required
                tooltip="The cost per million input tokens (e.g., 0.40 for $0.40 per million tokens)"
              />
            </Grid>
            <Grid item xs={12}>
              <TooltipTextField
                fullWidth
                label="Cost per Million Output Tokens"
                name="cpt_million"
                type="number"
                inputProps={{ step: 0.01, min: 0 }}
                value={displayPrice.cpt_million}
                onChange={handleChange}
                required
                tooltip="The cost per million output tokens (e.g., 0.40 for $0.40 per million tokens)"
              />
            </Grid>
            <Grid item xs={12}>
              <TooltipTextField
                fullWidth
                label="Currency"
                name="currency"
                value={price.currency}
                onChange={handleChange}
                error={!!errors.currency}
                helperText={errors.currency}
                required
                autoComplete="off"
                tooltip="The currency for the cost per token (e.g., USD)"
              />
            </Grid>
          </Grid>

          <Box mt={4}>
            <PrimaryButton variant="contained" type="submit">
              {id ? "Update model price" : "Add model price"}
            </PrimaryButton>
          </Box>
        </Box>
      </ContentBox>

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

export default ModelPriceForm;
