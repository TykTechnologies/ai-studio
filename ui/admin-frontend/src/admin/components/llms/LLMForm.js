import React, { useState, useEffect, useRef } from "react";
import apiClient from "../../utils/apiClient";
import { generateSlug } from "../../components/wizards/quick-start/utils";
import {
  TextField,
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
  AccordionSummary,
  AccordionDetails,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import Visibility from "@mui/icons-material/Visibility";
import VisibilityOff from "@mui/icons-material/VisibilityOff";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";
import pluginService from "../../services/pluginService";
import PluginConfigDialog from './PluginConfigDialog';
import {
  getVendorName,
  getVendorLogo,
  getVendorCodes,
} from "../../utils/vendorLogos";
import { useTheme } from "@mui/material/styles";
import Stack from "@mui/material/Stack";
import AddIcon from "@mui/icons-material/Add";
import SettingsIcon from "@mui/icons-material/Settings";

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
    allowed_models: [],
    monthly_budget: null,
    budget_start_date: null,
    namespace: "", // Added for edge availability
    plugins: [], // Added for plugin assignment
  });
  const [vendors, setVendors] = useState([]);
  const [filters, setFilters] = useState(null);
  const [availablePlugins, setAvailablePlugins] = useState([]);
  const [originalName, setOriginalName] = useState("");
  const [nameChanged, setNameChanged] = useState(false);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  
  const [filtersLoading, setFiltersLoading] = useState(true);
  const [, setPluginsLoading] = useState(true);

  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [showApiKey, setShowApiKey] = useState(false);

  // Plugin configuration dialog state
  const [configDialogOpen, setConfigDialogOpen] = useState(false);
  const [selectedPluginForConfig, setSelectedPluginForConfig] = useState(null);
  const navigate = useNavigate();
  const { id } = useParams();
  const theme = useTheme();
  const [newModel, setNewModel] = useState("");

  useEffect(() => {
    setVendors(getVendorCodes());
    fetchFilters();
    fetchPlugins();
    if (id) {
      fetchLLM();
    }
  }, [id]);

  const handleAddModel = () => {
    if (newModel.trim()) {
      setLLM((prev) => ({
        ...prev,
        allowed_models: [...(prev.allowed_models || []), newModel.trim()],
      }));
      setNewModel("");
    }
  };

  const handleDeleteModel = (modelToDelete) => {
    setLLM((prev) => ({
      ...prev,
      allowed_models: prev.allowed_models.filter(
        (model) => model !== modelToDelete,
      ),
    }));
  };

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

  const fetchPlugins = async () => {
    setPluginsLoading(true);
    try {
      const response = await pluginService.listPlugins(1, 100, '', true);
      setAvailablePlugins(response.data || []);
    } catch (error) {
      console.error("Error fetching plugins", error);
      setSnackbar({
        open: true,
        message: `Failed to fetch plugins: ${error.message || "Unknown error"}`,
        severity: "error",
      });
    } finally {
      setPluginsLoading(false);
    }
  };

  const fetchLLM = async () => {
    try {
      const llmResponse = await apiClient.get(`/llms/${id}`);
      const llmData = llmResponse.data.data.attributes;
      
      // Get plugins from the LLM response (now included in the API response)
      const pluginsData = llmData.plugins || [];
      
      setLLM({
        ...llmData,
        filters: llmData.filters.map((filter) => filter.id.toString()),
        namespace: llmData.namespace || "",
        plugins: pluginsData.map((plugin) => plugin.id.toString()),
      });
      setOriginalName(llmData.name);
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
    } else if (name === "name") {
      setLLM({ ...llm, [name]: value });
      if (id && originalName) {
        setNameChanged(value !== originalName);
      }
    } else {
      setLLM({ ...llm, [name]: value });
    }
  };

  const handleBudgetChange = (e) => {
    const value = e.target.value === '' ? null : parseFloat(e.target.value);
    setLLM(prev => ({
      ...prev,
      monthly_budget: value,
      budget_start_date: value ? prev.budget_start_date || new Date().toISOString() : null
    }));
  };

  const handleBudgetStartDateChange = (e) => {
    const value = e.target.value;
    if (!value) {
      setLLM(prev => ({ ...prev, budget_start_date: null }));
      return;
    }
    // Create date in local timezone and convert to UTC
    const date = new Date(value + 'T00:00:00Z');
    setLLM(prev => ({ ...prev, budget_start_date: date.toISOString() }));
  };

  const handleSwitchChange = (e) => {
    setLLM({ ...llm, active: e.target.checked });
  };

  const handleNamespaceChange = (namespaces) => {
    // Convert array to comma-delimited string, or empty string for global
    const namespaceString = Array.isArray(namespaces) ? namespaces.join(', ') : namespaces;
    setLLM({ ...llm, namespace: namespaceString });
  };

  const handlePluginChange = (e) => {
    const { value } = e.target;
    setLLM({ ...llm, plugins: value });
  };

  const handlePluginRemove = (pluginIdToRemove) => {
    const updatedPlugins = llm.plugins.filter(id => id !== pluginIdToRemove);
    setLLM({ ...llm, plugins: updatedPlugins });
  };

  const handlePluginConfig = (pluginId) => {
    const plugin = availablePlugins.find((p) => p.id.toString() === pluginId);
    if (plugin) {
      setSelectedPluginForConfig(plugin);
      setConfigDialogOpen(true);
    }
  };

  const handlePluginConfigClose = () => {
    setConfigDialogOpen(false);
    setSelectedPluginForConfig(null);
  };

  const validateForm = () => {
    const newErrors = {};
    if (!llm.name.trim()) newErrors.name = "Name is required";
    if (!llm.vendor.trim()) newErrors.vendor = "Vendor is required";
    if (llm.privacy_score < 0 || llm.privacy_score > 100)
      newErrors.privacy_score = "Privacy level must be between 0 and 100";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    // If name has changed and we're editing an existing LLM, show confirmation dialog
    if (id && nameChanged) {
      setConfirmDialogOpen(true);
      return;
    }

    await saveLLM();
  };

  const saveLLM = async () => {
    // Remove plugins from the main LLM data since they're managed separately
    const { plugins, ...llmDataWithoutPlugins } = llm;
    
    const llmData = {
      data: {
        type: "LLM",
        attributes: {
          ...llmDataWithoutPlugins,
          privacy_score: Number(llm.privacy_score),
          active: Boolean(llm.active),
          filters: llm.filters.map((filterId) => parseInt(filterId, 10)),
        },
      },
    };

    try {
      let llmResponse;
      if (id) {
        llmResponse = await apiClient.patch(`/llms/${id}`, llmData);
      } else {
        llmResponse = await apiClient.post("/llms", llmData);
      }

      // Get the LLM ID from the response or use the existing ID
      const llmId = id || llmResponse.data?.data?.id;
      
      // Update plugins separately if LLM was saved successfully
      if (llmId) {
        const pluginIds = plugins ? plugins.map((pluginId) => parseInt(pluginId, 10)) : [];
        await apiClient.put(`/llms/${llmId}/plugins`, {
          plugin_ids: pluginIds,
        });
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
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">{id ? "Edit LLM provider" : "Add LLM provider"}</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/llms"
          color="inherit"
        >
          Back to LLMs
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">LLM providers power AI assistants in chats and can be made available to developers in the portal and gateway when set to Active. To control access, each LLM provider must be part of a catalog to be used by specific user groups.</Typography>  
      </Box>
      <ContentBox sx={{ pt: 0 }}>
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
                helperText={errors.name || (nameChanged ? 
                  `Warning: Changing the LLM name will change the REST endpoint from /llm/rest/${generateSlug(originalName)}/ to /llm/rest/${generateSlug(llm.name)}/` : "")}
                required
                FormHelperTextProps={{
                  sx: nameChanged ? { color: theme.palette.warning.main } : {}
                }}
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
              <Grid container spacing={2}>
                <Grid item xs={12} md={6}>
                  <TextField
                    fullWidth
                    label="Monthly Budget"
                    name="monthly_budget"
                    type="number"
                    inputProps={{
                      step: "0.01",
                      min: "0"
                    }}
                    value={llm.monthly_budget || ''}
                    onChange={handleBudgetChange}
                    InputProps={{
                      startAdornment: <InputAdornment position="start">$</InputAdornment>,
                    }}
                    helperText="Leave empty for no budget limit"
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <TextField
                    fullWidth
                    label="Budget Start Date"
                    name="budget_start_date"
                    type="date"
                    value={llm.budget_start_date ? new Date(llm.budget_start_date).toISOString().split('T')[0] : ''}
                    onChange={handleBudgetStartDateChange}
                    disabled={!llm.monthly_budget}
                    InputLabelProps={{
                      shrink: true,
                    }}
                    helperText="Budget cycle start date"
                  />
                </Grid>
              </Grid>
            </Grid>
            <Grid item xs={12}>
              <Typography variant="subtitle2" gutterBottom>
                Privacy levels
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Privacy levels define how data is protected by controlling LLM access based on its sensitivity. LLMs providers with lower privacy levels can’t access higher-level, data sources and tools, ensuring secure and appropriate data handling. Set a privacy level (0 lowest - 100 highest).
              </Typography>
              <TextField
                fullWidth
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
              />
            </Grid>
            <Grid item xs={12}>
              <Typography variant="subtitle2" gutterBottom>
                Allowed Models
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Add regex patterns to whitelist specific models (e.g., "gpt-4.*"
                for all GPT-4 models)
              </Typography>
              <Box sx={{ display: "flex", gap: 1, mb: 2 }}>
                <TextField
                  fullWidth
                  label="Model Pattern"
                  value={newModel}
                  autoComplete="off"
                  onChange={(e) => setNewModel(e.target.value)}
                  placeholder="Enter model pattern (e.g., gpt-4.*)"
                  onKeyPress={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      handleAddModel();
                    }
                  }}
                />
                <IconButton
                  onClick={handleAddModel}
                  sx={{ ml: 1 }}
                >
                  <AddIcon />
                </IconButton>
              </Box>
              <Stack
                direction="row"
                spacing={1}
                flexWrap="wrap"
                sx={{ gap: 1 }}
              >
                {llm.allowed_models?.map((model, index) => (
                  <Chip
                    key={index}
                    label={model}
                    onDelete={() => handleDeleteModel(model)}
                  />
                ))}
              </Stack>
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

          {/* Plugin Assignment Section */}
          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Plugins</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="textSecondary" paragraph>
                Select plugins to attach to this LLM. Plugins are executed in the order they are selected.
                Each plugin type (hook type) determines when the plugin is executed in the request lifecycle.
              </Typography>
              
              <FormControl fullWidth>
                <InputLabel>Plugins</InputLabel>
                <Select
                  multiple
                  value={llm.plugins}
                  onChange={handlePluginChange}
                  renderValue={(selected) => (
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                      {selected.map((pluginId) => {
                        const plugin = availablePlugins.find((p) => p.id.toString() === pluginId);
                        return (
                          <Chip
                            key={pluginId}
                            label={plugin ? plugin.name : pluginId}
                            size="small"
                            color="primary"
                            variant="outlined"
                            onDelete={() => handlePluginRemove(pluginId)}
                          />
                        );
                      })}
                    </Box>
                  )}
                >
                  {pluginService.getAvailableHookTypes().map((hookType) => {
                    const pluginsForType = availablePlugins.filter(p => p.hookType === hookType.value);
                    if (pluginsForType.length === 0) return null;
                    
                    return [
                      <MenuItem key={`header-${hookType.value}`} disabled>
                        <Typography variant="subtitle2" color="primary">
                          {hookType.label}
                        </Typography>
                      </MenuItem>,
                      ...pluginsForType.map((plugin) => (
                        <MenuItem key={plugin.id} value={plugin.id.toString()}>
                          <Box sx={{ pl: 2 }}>
                            <Typography variant="body2">
                              {plugin.name}
                            </Typography>
                            <Typography variant="caption" color="textSecondary">
                              {plugin.description || 'No description'}
                            </Typography>
                          </Box>
                        </MenuItem>
                      ))
                    ];
                  })}
                </Select>
              </FormControl>
              
              {llm.plugins.length > 0 && (
                <Box mt={2}>
                  <Typography variant="body2" color="textSecondary" gutterBottom>
                    Selected Plugins (execution order):
                  </Typography>
                  <Stack spacing={1}>
                    {llm.plugins.map((pluginId, index) => {
                      const plugin = availablePlugins.find((p) => p.id.toString() === pluginId);
                      return plugin ? (
                        <Box key={pluginId} sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 0.5 }}>
                          <Typography variant="body2" sx={{ minWidth: 24 }}>
                            {index + 1}.
                          </Typography>
                          <Chip
                            label={pluginService.getHookTypeLabel(plugin.hookType)}
                            size="small"
                            color="primary"
                            variant="outlined"
                          />
                          <Typography variant="body2" sx={{ flex: 1 }}>
                            {plugin.name}
                          </Typography>
                          <IconButton
                            size="small"
                            onClick={() => handlePluginConfig(pluginId)}
                            sx={{
                              color: 'action.secondary',
                              '&:hover': { color: 'primary.main' }
                            }}
                          >
                            <SettingsIcon fontSize="small" />
                          </IconButton>
                        </Box>
                      ) : null;
                    })}
                  </Stack>
                </Box>
              )}
            </AccordionDetails>
          </StyledAccordion>

          {/* Edge Availability Section */}
          <EdgeAvailabilitySection
            value={llm.namespace}
            onChange={handleNamespaceChange}
            defaultExpanded={false}
          />

          <Box mt={4}>
            <PrimaryButton variant="contained" type="submit">
              {id ? "Update LLM" : "Add LLM"}
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

      {/* Confirmation Dialog for Name Change */}
      <Dialog
        open={confirmDialogOpen}
        onClose={() => setConfirmDialogOpen(false)}
      >
        <DialogTitle>Confirm Name Change</DialogTitle>
        <DialogContent>
          <DialogContentText>
            You are about to change the LLM name from "{originalName}" to "{llm.name}". 
            This will change the following endpoints:
            <ul>
              <li>REST endpoint: /llm/rest/{generateSlug(originalName)}/ → /llm/rest/{generateSlug(llm.name)}/</li>
              <li>Stream endpoint: /llm/stream/{generateSlug(originalName)}/ → /llm/stream/{generateSlug(llm.name)}/</li>
              <li>AI endpoint: /ai/{generateSlug(originalName)}/v1 → /ai/{generateSlug(llm.name)}/v1</li>
            </ul>
            This will affect existing integrations that reference this LLM.
            Are you sure you want to continue?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <SecondaryLinkButton onClick={() => setConfirmDialogOpen(false)}>
            Cancel
          </SecondaryLinkButton>
          <PrimaryButton 
            onClick={() => {
              setConfirmDialogOpen(false);
              saveLLM();
            }}
          >
            Confirm
          </PrimaryButton>
        </DialogActions>
      </Dialog>

      {/* Plugin Configuration Dialog */}
      <PluginConfigDialog
        open={configDialogOpen}
        onClose={handlePluginConfigClose}
        plugin={selectedPluginForConfig}
        llmId={id}
        onConfigSaved={(pluginId, config) => {
          console.log('Plugin config saved:', pluginId, config);
          // Could trigger a refresh or show success message here
        }}
      />
    </>
  );
};

export default LLMForm;
