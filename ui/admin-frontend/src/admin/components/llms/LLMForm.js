import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
  Box,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Typography,
  Grid,
  Snackbar,
  Alert,
  Switch,
  FormControlLabel,
  InputAdornment,
  IconButton,
  Tooltip,
  AccordionSummary,
  AccordionDetails,
  Chip,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import Visibility from "@mui/icons-material/Visibility";
import VisibilityOff from "@mui/icons-material/VisibilityOff";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import {
  getVendorName,
  getVendorLogo,
  getVendorCodes,
} from "../../utils/vendorLogos";
import { useTheme } from "@mui/material/styles";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const LLMForm = () => {
  const [llm, setLLM] = useState({
    name: "",
    short_description: "",
    long_description: "",
    vendor: "",
    privacy_score: 0,
    api_endpoint: "",
    api_key: "",
    logo_url: "",
    active: false,
    filters: [],
    default_model: "",
  });
  const [vendors, setVendors] = useState([]);
  const [filters, setFilters] = useState(null);

  const [filtersLoading, setFiltersLoading] = useState(true);

  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [showApiKey, setShowApiKey] = useState(false);
  const navigate = useNavigate();
  const { id } = useParams();
  const theme = useTheme();

  useEffect(() => {
    setVendors(getVendorCodes());
    fetchFilters();
    if (id) {
      fetchLLM();
    }
  }, [id]);

  const fetchFilters = async () => {
    setFiltersLoading(true);
    try {
      const response = await apiClient.get("/filters");
      if (Array.isArray(response.data)) {
        setFilters(response.data);
      } else {
        throw new Error("Invalid response format");
      }
    } catch (error) {
      console.error("Error fetching filters", error);
      setFilters([]);
      setSnackbar({
        open: true,
        message: `Failed to fetch filters: ${error.message || "Unknown error"}`,
        severity: "error",
      });
    } finally {
      setFiltersLoading(false);
    }
  };

  const fetchLLM = async () => {
    try {
      const response = await apiClient.get(`/llms/${id}`);
      const llmData = response.data.data.attributes;
      setLLM({
        ...llmData,
        filters: llmData.filters.map((filter) => filter.id.toString()),
      });
    } catch (error) {
      console.error("Error fetching LLM", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch LLM details",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    if (name === "privacy_score") {
      const numValue = Math.min(Math.max(parseInt(value) || 0, 0), 100);
      setLLM({ ...llm, [name]: numValue });
    } else if (name === "filters") {
      const stringFilters = value.map((filterId) => filterId.toString());
      setLLM({ ...llm, filters: stringFilters });
    } else {
      setLLM({ ...llm, [name]: value });
    }
  };

  const handleSwitchChange = (e) => {
    setLLM({ ...llm, active: e.target.checked });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!llm.name.trim()) newErrors.name = "Name is required";
    if (!llm.vendor.trim()) newErrors.vendor = "Vendor is required";
    if (llm.privacy_score < 0 || llm.privacy_score > 100)
      newErrors.privacy_score = "Privacy score must be between 0 and 100";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const llmData = {
      data: {
        type: "LLM",
        attributes: {
          ...llm,
          privacy_score: Number(llm.privacy_score),
          active: Boolean(llm.active),
          filters: llm.filters.map((filterId) => parseInt(filterId, 10)),
        },
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/llms/${id}`, llmData);
      } else {
        await apiClient.post("/llms", llmData);
      }

      setSnackbar({
        open: true,
        message: id ? "LLM updated successfully" : "LLM created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/llms"), 2000);
    } catch (error) {
      console.error("Error saving LLM", error);
      setSnackbar({
        open: true,
        message: "Failed to save LLM. Please try again.",
        severity: "error",
      });
    }
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">{id ? "Edit LLM" : "Add LLM"}</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/llms"
          color="white"
        >
          Back to LLMs
        </Button>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <SectionTitle>LLM Description</SectionTitle>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                name="name"
                value={llm.name}
                onChange={handleChange}
                error={!!errors.name}
                helperText={errors.name}
                required
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Short Description"
                name="short_description"
                value={llm.short_description}
                onChange={handleChange}
                multiline
                rows={2}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Long Description"
                name="long_description"
                value={llm.long_description}
                onChange={handleChange}
                multiline
                rows={4}
              />
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth>
                <InputLabel>Vendor</InputLabel>
                <Select
                  name="vendor"
                  value={llm.vendor}
                  onChange={handleChange}
                  error={!!errors.vendor}
                  required
                >
                  {vendors.map((vendorCode) => (
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
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Default Model"
                name="default_model"
                value={llm.default_model}
                onChange={handleChange}
                helperText="Specify the default model to use for this LLM (e.g., gpt-4, claude-2)"
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Privacy Score"
                name="privacy_score"
                type="number"
                value={llm.privacy_score}
                onChange={handleChange}
                error={!!errors.privacy_score}
                helperText={errors.privacy_score}
                inputProps={{
                  min: 0,
                  max: 100,
                  step: 1,
                }}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <Tooltip title="Privacy score is a value between 0 and 100, where 0 is the lowest and 100 is the highest. This determines the privacy level of the LLM for Data Source sharing.">
                        <HelpOutlineIcon color="action" />
                      </Tooltip>
                    </InputAdornment>
                  ),
                }}
              />
            </Grid>
          </Grid>

          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Access Details</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="text.secondary" paragraph>
                Some LLMs do not require an API Key for access, or have a
                default URL (for example Anthropic and OpenAI). If enabling an
                LLM for the AI Gateway, the endpoint is required for proper
                functioning.
              </Typography>
              <Grid container spacing={3}>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="API Endpoint"
                    name="api_endpoint"
                    value={llm.api_endpoint}
                    onChange={handleChange}
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="API Key"
                    name="api_key"
                    type={showApiKey ? "text" : "password"}
                    value={llm.api_key}
                    onChange={handleChange}
                    InputProps={{
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            onClick={() => setShowApiKey(!showApiKey)}
                            edge="end"
                          >
                            {showApiKey ? <VisibilityOff /> : <Visibility />}
                          </IconButton>
                        </InputAdornment>
                      ),
                    }}
                  />
                </Grid>
              </Grid>
            </AccordionDetails>
          </StyledAccordion>

          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Portal Display Information</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="text.secondary" paragraph>
                The following settings will be used in the Portal UI that your
                end-users / developers will see when browsing for LLMs to use.
              </Typography>
              <Grid container spacing={3}>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Logo URL"
                    name="logo_url"
                    value={llm.logo_url}
                    onChange={handleChange}
                  />
                </Grid>
                <Grid item xs={12}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={llm.active}
                        onChange={handleSwitchChange}
                        name="active"
                        color="primary"
                      />
                    }
                    label="Enabled in Proxy"
                  />
                </Grid>
              </Grid>
            </AccordionDetails>
          </StyledAccordion>

          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Filters</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="text.secondary" paragraph>
                Filters added here are executed in the AI Gateway when a request
                flows through the REST endpoint.
              </Typography>
              {filtersLoading ? (
                <Typography>Loading filters...</Typography>
              ) : filters === null ? (
                <Typography>
                  Error loading filters. Please try again.
                </Typography>
              ) : filters.length === 0 ? (
                <Typography>No filters available.</Typography>
              ) : (
                <FormControl fullWidth>
                  <InputLabel>Filters</InputLabel>
                  <Select
                    multiple
                    name="filters"
                    value={llm.filters || []}
                    onChange={handleChange}
                    renderValue={(selected) => (
                      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                        {selected.map((value) => {
                          const filter = filters.find((f) => f.id === value);
                          return (
                            <Chip
                              key={value}
                              label={
                                filter
                                  ? filter.attributes.name
                                  : "Unknown Filter"
                              }
                            />
                          );
                        })}
                      </Box>
                    )}
                  >
                    {filters.map((filter) => (
                      <MenuItem key={filter.id} value={filter.id.toString()}>
                        {filter.attributes.name}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              )}
            </AccordionDetails>
          </StyledAccordion>

          <Box mt={4}>
            <StyledButton variant="contained" type="submit">
              {id ? "Update LLM" : "Add LLM"}
            </StyledButton>
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
    </StyledPaper>
  );
};

export default LLMForm;
