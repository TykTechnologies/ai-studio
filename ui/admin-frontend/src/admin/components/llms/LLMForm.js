import GovernedMetadataFields, { GOVERNED_METADATA_SECTION_ID } from "../metadata/GovernedMetadataFields";
import { extractGovernedMetadataErrors, governedMetadataSaveWarning } from "../../services/governedMetadataService";
import { slugConflictDetail, focusNameField } from "../../utils/slugConflict";
import React, { useState, useEffect, useRef } from "react";
import apiClient from "../../utils/apiClient";
import focusFirstInvalid from "../../utils/focusFirstInvalid";
import { generateSlug } from "../../components/wizards/quick-start/utils";
import {
  TextField,
  Box,
  FormControl,
  FormHelperText,
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
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import {
  SecondaryLinkButton,
  SecondaryOutlineButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";
import RelationshipPicker from "../common/relationship-picker";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../../components/unsaved-changes";
import { useEdition } from "../../context/EditionContext";
import PublishSwitch from "../rbac/PublishSwitch";
import PrivacyLevelInput from "../common/privacy/PrivacyLevelInput";
import { isValidPrivacyScore } from "../common/privacy/privacyLevels";
import { P } from "../../rbac/permissions";
import pluginService from "../../services/pluginService";
import PluginConfigDialog from './PluginConfigDialog';
import {
  getVendorName,
  getVendorLogo,
  getVendorCodes,
  getVendorDefaultEndpoint,
  isDecisionOnlyVendor,
  vendorServesDecisions,
} from "../../utils/vendorLogos";
import { useTheme } from "@mui/material/styles";
import Stack from "@mui/material/Stack";
import AddIcon from "@mui/icons-material/Add";
import SettingsIcon from "@mui/icons-material/Settings";
import LLMFailoverSection, { validateFailover } from "./LLMFailoverSection";
import { listAll } from "../../utils/listAll";
import BudgetField from "../common/BudgetField";
import { ResourceTracingSection } from "../common/TracingSettingsSection";
import AllowInternalHostSwitch from "../common/AllowInternalHostSwitch";
import LLMPriceMultipliersField, {
  rowsFromPriceMultipliers,
  validatePriceMultipliers,
} from "./LLMPriceMultipliersField";

// Mirrors the server's vendor_workspace_id rule.
const WORKSPACE_ID_PATTERN = /^[A-Za-z0-9_-]{1,128}$/;

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
    dont_log_bodies: false, // When true, request/response bodies are not stored in logs
    failover: null, // Ordered waterfall of {llm_id, model} fallbacks; null = none
    vendor_workspace_id: "", // Anthropic only: pins every call to this workspace
    tracing_content_mode_max: "", // Agent tracing content cap (Enterprise); "" = no cap
    tracing_propagation_opt_out: false, // true: no traceparent to this provider
    allow_internal_host: false, // admin-only: the endpoint may be an internal address
  });
  const [vendors, setVendors] = useState([]);
  // Every other LLM, for the failover picker (id, name, allowed models...).
  const [availableLLMs, setAvailableLLMs] = useState([]);
  const [failoverErrors, setFailoverErrors] = useState({});
  // Price multiplier overrides, edited as {tier, value} rows and sent as
  // attributes.price_multipliers (a map, or null for none).
  const [priceMultiplierRows, setPriceMultiplierRows] = useState([]);
  const [priceMultiplierErrors, setPriceMultiplierErrors] = useState({});
  const [priceMultiplierServerError, setPriceMultiplierServerError] = useState("");
  // Governed metadata (Enterprise): values live beside the object and are
  // sent as attributes.governed_metadata; 422 field errors map back here.
  const [governedMetadata, setGovernedMetadata] = useState({});
  const [metadataErrors, setMetadataErrors] = useState({});
  const [filters, setFilters] = useState(null);
  const [availablePlugins, setAvailablePlugins] = useState([]);
  const [originalName, setOriginalName] = useState("");
  const [nameChanged, setNameChanged] = useState(false);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  
  const [filtersLoading, setFiltersLoading] = useState(true);
  const [, setPluginsLoading] = useState(true);
  // True once the LLM being edited is on screen (always true for a new one).
  const [loaded, setLoaded] = useState(false);

  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [showApiKey, setShowApiKey] = useState(false);
  const [showAwsSecretKey, setShowAwsSecretKey] = useState(false);

  // AWS Bedrock credential fields (stored in metadata on save)
  const [awsCreds, setAwsCreds] = useState({
    aws_access_key_id: "",
    aws_secret_access_key: "",
    aws_session_token: "",
  });

  // Plugin configuration dialog state
  const [configDialogOpen, setConfigDialogOpen] = useState(false);
  const [selectedPluginForConfig, setSelectedPluginForConfig] = useState(null);
  const navigate = useNavigate();
  const { id } = useParams();
  const theme = useTheme();
  const { isEnterprise } = useEdition();
  const [newModel, setNewModel] = useState("");

  // Unsaved-changes tracking over everything the form saves (the LLM record,
  // its ordered plugins, governed metadata and the Bedrock credentials).
  // markSaved() runs before the post-save redirect so the guard stays quiet.
  const { markSaved } = useUnsavedForm(
    { llm, governedMetadata, awsCreds },
    { ready: !id || loaded },
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/llms"));

  // The Filters picker works on full filter objects; the form keeps the id
  // list the API expects.
  const selectedFilters = (llm.filters || []).map(
    (filterId) =>
      (filters || []).find((f) => String(f.id) === String(filterId)) || {
        id: filterId,
        attributes: { name: "Unknown Filter" },
      },
  );
  const handleFiltersChange = (items) => {
    setLLM((prev) => ({ ...prev, filters: items.map((f) => String(f.id)) }));
  };
  // The chain order is persisted and executed top to bottom, so the list
  // above the picker has explicit move controls (same pattern as failover).
  const moveFilter = (index, delta) => {
    setLLM((prev) => {
      const next = [...(prev.filters || [])];
      const to = index + delta;
      if (to < 0 || to >= next.length) return prev;
      [next[index], next[to]] = [next[to], next[index]];
      return { ...prev, filters: next };
    });
  };

  useEffect(() => {
    setVendors(getVendorCodes());
    fetchFilters();
    fetchPlugins();
    fetchAvailableLLMs();
    if (id) {
      fetchLLM();
    }
  }, [id]);

  const fetchAvailableLLMs = async () => {
    try {
      const response = await listAll(apiClient, "/llms");
      const items = response?.data?.data || [];
      setAvailableLLMs(
        items.map((item) => ({
          id: Number(item.id),
          name: item.attributes?.name,
          vendor: item.attributes?.vendor,
          active: item.attributes?.active,
          allowed_models: item.attributes?.allowed_models || [],
          default_model: item.attributes?.default_model || "",
          privacy_score: item.attributes?.privacy_score ?? 0,
          namespace: item.attributes?.namespace || "",
        }))
      );
    } catch (error) {
      console.error("Error fetching LLMs for failover", error);
    }
  };

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
      const response = await listAll(apiClient, "/filters");
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
        dont_log_bodies: llmData.dont_log_bodies || false,
        failover: llmData.failover?.targets?.length ? llmData.failover : null,
        vendor_workspace_id: llmData.vendor_workspace_id || "",
        tracing_content_mode_max: llmData.tracing_content_mode_max || "",
        tracing_propagation_opt_out: Boolean(llmData.tracing_propagation_opt_out),
        allow_internal_host: Boolean(llmData.allow_internal_host),
      });
      setOriginalName(llmData.name);
      setPriceMultiplierRows(rowsFromPriceMultipliers(llmData.price_multipliers));
      setGovernedMetadata(llmResponse.data.data.governed_metadata || {});

      // Extract AWS Bedrock credentials from metadata if vendor is bedrock
      if (llmData.vendor === "bedrock" && llmData.metadata) {
        setAwsCreds({
          aws_access_key_id: llmData.metadata.aws_access_key_id || "",
          aws_secret_access_key: llmData.metadata.aws_secret_access_key || "",
          aws_session_token: llmData.metadata.aws_session_token || "",
        });
      }
    } catch (error) {
      console.error("Error fetching LLM", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch LLM details",
        severity: "error",
      });
    } finally {
      setLoaded(true);
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
    } else if (name === "vendor") {
      const defaultEndpoint = getVendorDefaultEndpoint(value);
      const previousDefaultEndpoint = getVendorDefaultEndpoint(llm.vendor);
      setLLM((prev) => ({
        ...prev,
        vendor: value,
        // Update endpoint if: empty, or still matches the previous vendor's default
        api_endpoint:
          !prev.api_endpoint || prev.api_endpoint === previousDefaultEndpoint
            ? defaultEndpoint
            : prev.api_endpoint,
      }));
    } else {
      setLLM({ ...llm, [name]: value });
    }
  };

  // null is "no limit"; 0 is a budget of 0.
  const handleBudgetChange = (value) => {
    setLLM(prev => ({
      ...prev,
      monthly_budget: value,
      budget_start_date: value !== null ? prev.budget_start_date || new Date().toISOString() : null
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
    if (!isValidPrivacyScore(llm.privacy_score))
      newErrors.privacy_score = "Privacy level must be between 0 and 100";
    if (llm.vendor === "anthropic" && llm.vendor_workspace_id && !WORKSPACE_ID_PATTERN.test(llm.vendor_workspace_id))
      newErrors.vendor_workspace_id = "Anthropic workspace ID must be 1-128 letters, digits, '_' or '-'";
    // Mirror the server's waterfall rules so a bad rung is named here first.
    const llmsById = Object.fromEntries(availableLLMs.map((l) => [String(l.id), l]));
    const nextFailoverErrors = validateFailover(llm.failover, llmsById, llm);
    setFailoverErrors(nextFailoverErrors);
    if (Object.keys(nextFailoverErrors).length > 0) {
      newErrors.failover = "Fix the failover targets";
    }
    const { rowErrors: nextPriceErrors } = validatePriceMultipliers(priceMultiplierRows);
    setPriceMultiplierErrors(nextPriceErrors);
    setPriceMultiplierServerError("");
    if (Object.keys(nextPriceErrors).length > 0) {
      newErrors.price_multipliers = "Fix the price multipliers";
    }
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) {
      focusFirstInvalid(e.currentTarget);
      return;
    }

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

    // For Bedrock: pack AWS credentials into metadata and clear api_key
    let metadata = llmDataWithoutPlugins.metadata || {};
    if (llm.vendor === "bedrock") {
      metadata = {
        ...metadata,
        aws_access_key_id: awsCreds.aws_access_key_id,
        aws_secret_access_key: awsCreds.aws_secret_access_key,
        aws_session_token: awsCreds.aws_session_token || undefined,
      };
      // Remove session token key entirely if empty
      if (!metadata.aws_session_token) {
        delete metadata.aws_session_token;
      }
    }

    const llmData = {
      data: {
        type: "LLM",
        attributes: {
          ...llmDataWithoutPlugins,
          privacy_score: Number(llm.privacy_score),
          active: Boolean(llm.active),
          filters: llm.filters.map((filterId) => parseInt(filterId, 10)),
          metadata: metadata,
          governed_metadata: governedMetadata,
          // Only Anthropic uses the pin; switching vendor away clears it.
          vendor_workspace_id: llm.vendor === "anthropic" ? llm.vendor_workspace_id : "",
          // The whole map replaces the stored one; null removes every override.
          price_multipliers: validatePriceMultipliers(priceMultiplierRows).map,
          // An empty waterfall is sent as null so a PATCH clears it.
          failover: llm.failover?.targets?.length
            ? {
                targets: llm.failover.targets.map((t) => ({ llm_id: Number(t.llm_id), model: t.model })),
                triggers: llm.failover.triggers || undefined,
              }
            : null,
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

      markSaved();
      const metadataWarning = governedMetadataSaveWarning(llmResponse);
      navigate("/admin/llms", {
        state: {
          snackbar: metadataWarning
            ? { message: `${id ? "LLM provider updated" : "LLM provider created"}, but ${metadataWarning}`, severity: "warning" }
            : { message: id ? "LLM provider updated successfully" : "LLM provider created successfully", severity: "success" },
        },
      });
    } catch (error) {
      if (error.response?.status === 422) {
        const fieldErrors = extractGovernedMetadataErrors(error);
        setMetadataErrors(fieldErrors);
        setSnackbar({
          open: true,
          message: fieldErrors._ || "Governance metadata failed validation. Fix the highlighted fields.",
          severity: "error",
        });
        document.getElementById(GOVERNED_METADATA_SECTION_ID)?.scrollIntoView({ behavior: "smooth", block: "center" });
        return;
      }
      // The server names the failover rung it rejected ("failover target 2: ...").
      const detail = error.response?.data?.errors?.[0]?.detail;
      if (error.response?.status === 400 && detail && detail.startsWith("failover")) {
        setFailoverErrors({ _: detail });
        setSnackbar({ open: true, message: detail, severity: "error" });
        return;
      }
      if (error.response?.status === 400 && detail && detail.startsWith("price_multipliers")) {
        setPriceMultiplierServerError(detail);
        setSnackbar({ open: true, message: detail, severity: "error" });
        return;
      }
      if (error.response?.status === 400 && detail && detail.startsWith("vendor_workspace_id")) {
        setErrors((prev) => ({ ...prev, vendor_workspace_id: detail }));
        setSnackbar({ open: true, message: detail, severity: "error" });
        return;
      }
      const slugConflict = slugConflictDetail(error);
      if (slugConflict) {
        // Another LLM already has this name's slug: say so on the Name field.
        setErrors((prev) => ({ ...prev, name: slugConflict }));
        setSnackbar({ open: true, message: slugConflict, severity: "error" });
        focusNameField();
        return;
      }
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
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">{id ? "Edit LLM provider" : "Add LLM provider"}</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/llms"
          color="inherit"
        >
          Back to LLM providers
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">LLM providers power AI assistants in chats and can be made available to developers in the portal and gateway when set to Active. To control access, each LLM provider must be part of a catalog to be used by specific teams.</Typography>  
      </Box>
      <ContentBox sx={{ pt: 0 }}>
        <Box component="form" onSubmit={handleSubmit} noValidate>
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
              <FormControl fullWidth error={!!errors.vendor}>
                <InputLabel id="llmform-vendor-label">Vendor</InputLabel>
                <Select
                  labelId="llmform-vendor-label"
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
                          alt=""
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
                {errors.vendor && <FormHelperText>{errors.vendor}</FormHelperText>}
              </FormControl>
              {vendorServesDecisions(llm.vendor) && (
                <Alert severity="info" sx={{ mt: 1 }} data-testid="llmform-decision-hint">
                  {isDecisionOnlyVendor(llm.vendor)
                    ? "This vendor serves decision models only: typed answers to questions, billed by input tokens. Apps call it through /v1/decisions (OpenAI Decisions schema); it cannot back chat rooms, agents or routers."
                    : "This vendor also serves decision models: Apps can call them through /v1/decisions."}
                </Alert>
              )}
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
          </Grid>

          {/* Credentials sit right under the identity fields: they are what a
              new provider needs before anything else on this form matters. */}
          <StyledAccordion defaultExpanded>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Access Details</Typography>
            </AccordionSummary>
            <AccordionDetails>
              {llm.vendor === "bedrock" ? (
                <>
                  <Typography variant="body2" color="text.secondary" paragraph>
                    AWS Bedrock requires an AWS region endpoint and IAM credentials.
                    For security, store sensitive credentials using the Secrets Manager
                    and reference them here as <strong>$SECRET/your-secret-name</strong> (environment variables as <strong>$ENV/YOUR_ENV_VAR</strong> only when your operator allows them).
                  </Typography>
                  <Grid container spacing={3}>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="Bedrock Endpoint"
                        name="api_endpoint"
                        value={llm.api_endpoint}
                        onChange={handleChange}
                        helperText="e.g., https://bedrock-runtime.us-east-1.amazonaws.com"
                      />
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="AWS Access Key ID"
                        value={awsCreds.aws_access_key_id}
                        onChange={(e) => setAwsCreds({ ...awsCreds, aws_access_key_id: e.target.value })}
                        placeholder="AKIA... or $SECRET/aws-access-key-id"
                        helperText="Enter directly or use $SECRET/name to reference a stored secret"
                      />
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="AWS Secret Access Key"
                        type={showAwsSecretKey ? "text" : "password"}
                        value={awsCreds.aws_secret_access_key}
                        onChange={(e) => setAwsCreds({ ...awsCreds, aws_secret_access_key: e.target.value })}
                        placeholder="$SECRET/aws-secret-access-key"
                        helperText="Recommended: use $SECRET/name to reference a stored secret rather than entering the key directly"
                        InputProps={{
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                onClick={() => setShowAwsSecretKey(!showAwsSecretKey)}
                                edge="end"
                              >
                                {showAwsSecretKey ? <VisibilityOff /> : <Visibility />}
                              </IconButton>
                            </InputAdornment>
                          ),
                        }}
                      />
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="AWS Session Token (optional)"
                        value={awsCreds.aws_session_token}
                        onChange={(e) => setAwsCreds({ ...awsCreds, aws_session_token: e.target.value })}
                        placeholder="$SECRET/aws-session-token"
                        helperText="Required only for temporary credentials (assumed roles, SSO). Use $SECRET/name for secure storage."
                      />
                    </Grid>
                  </Grid>
                </>
              ) : (
                <>
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
                        helperText="Enter the key, or reference a stored secret as $SECRET/name (environment variables as $ENV/NAME only when your operator allows them)."
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
                    {llm.vendor === "anthropic" && (
                      <Grid item xs={12}>
                        <TextField
                          fullWidth
                          label="Anthropic workspace ID"
                          name="vendor_workspace_id"
                          value={llm.vendor_workspace_id || ""}
                          onChange={handleChange}
                          placeholder="wrkspc_01..."
                          error={!!errors.vendor_workspace_id}
                          helperText={
                            errors.vendor_workspace_id ||
                            "Optional. Pins every call for this LLM (gateway calls, batches and Chat sessions) to this workspace; client-supplied workspace headers are ignored. Leave empty to use the API key's default workspace."
                          }
                        />
                      </Grid>
                    )}
                  </Grid>
                </>
              )}
            </AccordionDetails>
          </StyledAccordion>

          <Grid container spacing={3}>
            <Grid item xs={12}>
              <Grid container spacing={2}>
                <Grid item xs={12} md={6}>
                  <BudgetField
                    value={llm.monthly_budget}
                    onChange={handleBudgetChange}
                    testIdPrefix="llm-budget"
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
                    disabled={llm.monthly_budget === null || llm.monthly_budget === undefined}
                    InputLabelProps={{
                      shrink: true,
                    }}
                    helperText="Budget cycle start date"
                  />
                </Grid>
              </Grid>
            </Grid>
            <Grid item xs={12}>
              <LLMPriceMultipliersField
                vendor={llm.vendor}
                rows={priceMultiplierRows}
                onChange={setPriceMultiplierRows}
                rowErrors={priceMultiplierErrors}
                serverError={priceMultiplierServerError}
              />
            </Grid>
            <Grid item xs={12}>
              {/* One privacy control everywhere (UX review M4): a named level
                  with the 0–100 score alongside. */}
              <PrivacyLevelInput
                value={llm.privacy_score}
                onChange={(score) => setLLM((prev) => ({ ...prev, privacy_score: score }))}
                error={!!errors.privacy_score}
                helperText={errors.privacy_score}
              />
            </Grid>
            <Grid item xs={12}>
              <Typography variant="subtitle2" gutterBottom>
                Allowed Models
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Add regex patterns to whitelist specific models. Patterns are
                matched <strong>anywhere in the model name</strong>, so
                "gpt-4.*" also matches "legacy-gpt-4o" — anchor with ^ and $
                (e.g. "^gpt-4.*$") to match the whole name. Leaving this empty
                allows <strong>all</strong> models.
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
            <Grid item xs={12}>
              <LLMFailoverSection
                value={llm.failover}
                onChange={(next) => setLLM((prev) => ({ ...prev, failover: next }))}
                llms={availableLLMs}
                currentId={id}
                vendor={llm.vendor}
                errors={failoverErrors}
              />
            </Grid>
          </Grid>

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
                  <PublishSwitch
                    permission={P.LLMS_PUBLISH}
                    checked={llm.active}
                    onChange={handleSwitchChange}
                    name="active"
                    label="Active"
                  />
                  <Typography variant="caption" color="text.secondary" display="block">
                    Available to the portal and gateway when on
                  </Typography>
                </Grid>
                <Grid item xs={12}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={llm.dont_log_bodies}
                        onChange={(e) => setLLM({ ...llm, dont_log_bodies: e.target.checked })}
                        name="dont_log_bodies"
                        color="primary"
                      />
                    }
                    label="Disable Request/Response Body Logging"
                  />
                  <Typography variant="caption" color="text.secondary" display="block">
                    When enabled, request and response bodies will not be stored in proxy logs for this LLM provider.
                    Otherwise the log stores the request as it left the request filters (a redaction is logged
                    redacted), but a request a filter blocks is logged as it was sent.
                  </Typography>
                </Grid>
                <Grid item xs={12}>
                  <AllowInternalHostSwitch
                    checked={llm.allow_internal_host}
                    onChange={(v) => setLLM((prev) => ({ ...prev, allow_internal_host: v }))}
                    hostLabel="LLM's endpoint (and its linked embedders)"
                  />
                </Grid>
              </Grid>
            </AccordionDetails>
          </StyledAccordion>

          <ResourceTracingSection
            resourceLabel="LLM"
            contentModeMax={llm.tracing_content_mode_max}
            onContentModeMaxChange={(v) => setLLM((prev) => ({ ...prev, tracing_content_mode_max: v }))}
            propagationOptOut={llm.tracing_propagation_opt_out}
            onPropagationOptOutChange={(v) => setLLM((prev) => ({ ...prev, tracing_propagation_opt_out: v }))}
          />

          {/* Filters Section - Enterprise Only */}
          {isEnterprise && (
            <StyledAccordion>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography>Filters</Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Typography variant="body2" color="text.secondary" paragraph>
                  Filters added here are executed in the AI Gateway when a request
                  flows through the REST endpoint. Filters run top to bottom; the
                  first block wins.
                </Typography>
                {selectedFilters.length > 1 && (
                  <Box data-testid="llm-filter-order" sx={{ mb: 2 }}>
                    {selectedFilters.map((filter, index) => (
                      <Box
                        key={String(filter.id)}
                        data-testid="llm-filter-order-item"
                        sx={{ display: "flex", alignItems: "center", gap: 1 }}
                      >
                        <Typography variant="body2" sx={{ minWidth: 24 }}>
                          {index + 1}.
                        </Typography>
                        <Typography variant="body2" sx={{ flexGrow: 1 }}>
                          {filter?.attributes?.name ?? "Unknown Filter"}
                        </Typography>
                        <IconButton
                          size="small"
                          aria-label={`move filter ${index + 1} up`}
                          disabled={index === 0}
                          onClick={() => moveFilter(index, -1)}
                        >
                          <ArrowUpwardIcon fontSize="small" />
                        </IconButton>
                        <IconButton
                          size="small"
                          aria-label={`move filter ${index + 1} down`}
                          disabled={index === selectedFilters.length - 1}
                          onClick={() => moveFilter(index, 1)}
                        >
                          <ArrowDownwardIcon fontSize="small" />
                        </IconButton>
                      </Box>
                    ))}
                  </Box>
                )}
                {filtersLoading ? (
                  <Typography>Loading filters...</Typography>
                ) : filters === null ? (
                  <Typography>
                    Error loading filters. Please try again.
                  </Typography>
                ) : filters.length === 0 ? (
                  <Typography>No filters available.</Typography>
                ) : (
                  <RelationshipPicker
                    itemLabel="filter"
                    value={selectedFilters}
                    onChange={handleFiltersChange}
                    options={filters}
                    getOptionLabel={(filter) => filter?.attributes?.name ?? "Unknown Filter"}
                  />
                )}
              </AccordionDetails>
            </StyledAccordion>
          )}

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
                <InputLabel id="llmform-plugins-label">Plugins</InputLabel>
                <Select
                  labelId="llmform-plugins-label"
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

          {/* Governance metadata (Enterprise; hidden when no schema applies) */}
          <GovernedMetadataFields
            objectType="llm"
            value={governedMetadata}
            onChange={(next) => {
              setGovernedMetadata(next);
              setMetadataErrors({});
            }}
            errors={metadataErrors}
          />

          {/* Edge Availability Section */}
          <EdgeAvailabilitySection
            value={llm.namespace}
            onChange={handleNamespaceChange}
            defaultExpanded={false}
          />

          {/* Native browser validation is off (noValidate) so the errors are
              rendered here where tests and screen readers can see them. */}
          {Object.keys(errors).length > 0 && (
            <Alert severity="error" sx={{ mt: 3 }} data-testid="llm-form-errors">
              Fix the following before saving: {Object.values(errors).join("; ")}
            </Alert>
          )}
          <Box mt={4} display="flex" gap={2}>
            <SecondaryOutlineButton onClick={handleCancel}>
              Cancel
            </SecondaryOutlineButton>
            <PrimaryButton variant="contained" type="submit">
              {id ? "Update LLM provider" : "Add LLM provider"}
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
