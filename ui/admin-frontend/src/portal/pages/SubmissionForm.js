import React, { useState, useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import ReactMarkdown from "react-markdown";
import pubClient from "../../admin/utils/pubClient";
import {
  Container,
  Typography,
  Box,
  Grid,
  TextField,
  Button,
  Snackbar,
  Alert,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Slider,
  FormControlLabel,
  Checkbox,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  IconButton,
  InputAdornment,
  CircularProgress,
  Chip,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import VisibilityIcon from "@mui/icons-material/Visibility";
import VisibilityOffIcon from "@mui/icons-material/VisibilityOff";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import ErrorIcon from "@mui/icons-material/Error";
import validator from "@rjsf/validator-ajv8";
import { PrimaryButton, PrimaryOutlineButton } from "../../admin/styles/sharedStyles";
import {
  fetchVendors,
  getEmbedderDefaultModel,
  getEmbedderDefaultUrl,
} from "../../admin/utils/vendorUtils";
import SchemaFormRenderer from "../../admin/components/plugins/SchemaFormRenderer";

// The resource type dropdown carries plugin-provided types as "plugin:<id>" so
// one control can pick both the built-in kinds and every ResourceProvider type
// the server advertises. resourceType itself stays "plugin" for those, with the
// concrete type id held separately, which is the shape the API expects.
const PLUGIN_TYPE_PREFIX = "plugin:";

// Everything in a plugin payload except `name` is free-form when the type has
// no submission schema; it is edited as one JSON blob.
const pluginExtraFieldsJson = (payload) => {
  const { name, ...rest } = payload || {};
  return Object.keys(rest).length > 0 ? JSON.stringify(rest, null, 2) : "";
};

const vectorStoreOptions = [
  "chroma",
  "pgvector",
  "pinecone",
  "redis",
  "qdrant",
  "weaviate",
];

const SubmissionForm = () => {
  const navigate = useNavigate();
  const { id } = useParams();
  const isEdit = Boolean(id);

  const [resourceType, setResourceType] = useState("");
  const [payload, setPayload] = useState({});
  const [meta, setMeta] = useState({
    suggested_privacy: 0,
    privacy_justification: "",
    primary_contact: "",
    secondary_contact: "",
    sla_expectation: "",
    documentation_url: "",
    notes: "",
    data_cutoff_date: "",
  });
  const [attestations, setAttestations] = useState([]);
  const [attestationChecks, setAttestationChecks] = useState({});
  const [embedders, setEmbedders] = useState([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState({});
  const [duplicateWarning, setDuplicateWarning] = useState(null);
  const [testResult, setTestResult] = useState(null);
  const [showApiKey, setShowApiKey] = useState(false);
  const [showEmbedKey, setShowEmbedKey] = useState(false);
  const [specValidation, setSpecValidation] = useState(null);
  // Plugin-provided resource types (ResourceProvider plugins).
  const [pluginTypes, setPluginTypes] = useState([]);
  const [pluginResourceTypeId, setPluginResourceTypeId] = useState(null);
  // The type as returned on an existing submission, so a draft still renders
  // its schema if the type list has not loaded (or the type was deactivated).
  const [loadedPluginType, setLoadedPluginType] = useState(null);
  const [pluginExtraJson, setPluginExtraJson] = useState("");
  const [pluginExtraJsonError, setPluginExtraJsonError] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  useEffect(() => {
    const loadVendors = async () => {
      const { embedders: e } = await fetchVendors();
      setEmbedders(e.map((em) => em.code));
    };
    loadVendors();
    loadAttestations();
    loadPluginTypes();

    if (isEdit) {
      loadSubmission();
    }
  }, [id]);

  const loadPluginTypes = async () => {
    try {
      const response = await pubClient.get("/common/plugin-resource-types");
      setPluginTypes(response.data?.data || []);
    } catch (error) {
      // No plugin types is the normal case on an installation without
      // ResourceProvider plugins; the built-in types still work.
    }
  };

  const loadSubmission = async () => {
    try {
      setLoading(true);
      const response = await pubClient.get(`/common/submissions/${id}`);
      const data = response.data.data;
      setResourceType(data.resource_type);
      setPayload(data.resource_payload || {});
      if (data.resource_type === "plugin") {
        setPluginResourceTypeId(data.plugin_resource_type_id ?? null);
        setLoadedPluginType(data.plugin_resource_type || null);
        setPluginExtraJson(pluginExtraFieldsJson(data.resource_payload));
      }
      setMeta({
        suggested_privacy: data.suggested_privacy ?? 0,
        privacy_justification: data.privacy_justification || "",
        primary_contact: data.primary_contact || "",
        secondary_contact: data.secondary_contact || "",
        sla_expectation: data.sla_expectation || "",
        documentation_url: data.documentation_url || "",
        notes: data.notes || "",
        data_cutoff_date: data.data_cutoff_date || "",
      });
    } catch (error) {
      setSnackbar({
        open: true,
        message: "Failed to load submission",
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  };

  const loadAttestations = async () => {
    try {
      const response = await pubClient.get(
        "/common/submissions/attestation-templates"
      );
      setAttestations(response.data.data || []);
    } catch (error) {
      // Attestations are optional
    }
  };

  const handlePayloadChange = (field, value) => {
    setPayload((prev) => ({ ...prev, [field]: value }));
  };

  const handleMetaChange = (field, value) => {
    setMeta((prev) => ({ ...prev, [field]: value }));
  };

  const handleEmbedVendorChange = (vendor) => {
    handlePayloadChange("embed_vendor", vendor);
    handlePayloadChange("embed_model", getEmbedderDefaultModel(vendor));
    handlePayloadChange("embed_url", getEmbedderDefaultUrl(vendor));
  };

  const checkDuplicates = async () => {
    try {
      const response = await pubClient.post(
        "/common/submissions/check-duplicates",
        {
          resource_type: resourceType,
          resource_payload: payload,
        }
      );
      const dupes = response.data.data;
      if (dupes && dupes.length > 0) {
        setDuplicateWarning(dupes);
      } else {
        setDuplicateWarning(null);
      }
    } catch (error) {
      // Non-blocking
    }
  };

  const handleTestDatasource = async () => {
    try {
      setTestResult(null);
      const response = await pubClient.post(
        "/common/submissions/test-datasource",
        {
          embed_vendor: payload.embed_vendor,
          embed_url: payload.embed_url,
          embed_api_key: payload.embed_api_key,
          embed_model: payload.embed_model,
        }
      );
      setTestResult(response.data.data);
    } catch (error) {
      setTestResult({
        embedder_valid: false,
        embedder_error: "Test failed: " + (error.message || "Unknown error"),
      });
    }
  };

  const handleValidateSpec = async () => {
    try {
      setSpecValidation(null);
      const response = await pubClient.post(
        "/common/submissions/validate-spec",
        {
          oas_spec: payload.oas_spec,
        }
      );
      const validation = response.data.data;
      setSpecValidation(validation);

      // Start with every operation exposed and let the contributor narrow.
      // Starting empty meant a contributor who never touched the chips
      // submitted `available_operations: ""` -- a Tool that can never work,
      // discovered only after a review and an approval.
      const operations = validation?.extracted?.operations || [];
      if (validation?.valid && operations.length > 0 && !payload.available_operations) {
        handlePayloadChange("available_operations", operations.join(","));
      }
    } catch (error) {
      setSpecValidation({
        valid: false,
        errors: [{ field: "oas_spec", message: "Validation request failed" }],
      });
    }
  };

  const selectedPluginType =
    resourceType === "plugin"
      ? pluginTypes.find((t) => t.id === pluginResourceTypeId) ||
        loadedPluginType
      : null;
  // The schema on the submission wins: it is the one the payload was written
  // against. The fetched list is the fallback for a fresh submission.
  const pluginSchema =
    resourceType === "plugin"
      ? loadedPluginType?.submission_schema ||
        pluginTypes.find((t) => t.id === pluginResourceTypeId)
          ?.submission_schema ||
        null
      : null;

  const handleResourceTypeChange = (value) => {
    if (value.startsWith(PLUGIN_TYPE_PREFIX)) {
      setResourceType("plugin");
      setPluginResourceTypeId(Number(value.slice(PLUGIN_TYPE_PREFIX.length)));
    } else {
      setResourceType(value);
      setPluginResourceTypeId(null);
    }
    setPayload({});
    setPluginExtraJson("");
    setPluginExtraJsonError(null);
    setTestResult(null);
    setSpecValidation(null);
  };

  const handlePluginExtraJsonChange = (text) => {
    setPluginExtraJson(text);
    if (!text.trim()) {
      setPluginExtraJsonError(null);
      setPayload((prev) => ({ name: prev.name }));
      return;
    }
    try {
      const parsed = JSON.parse(text);
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        setPluginExtraJsonError("Additional fields must be a JSON object");
        return;
      }
      setPluginExtraJsonError(null);
      setPayload((prev) => ({ ...parsed, name: prev.name }));
    } catch (err) {
      setPluginExtraJsonError("Additional fields must be valid JSON");
    }
  };

  const validateForm = (submitForReview = false) => {
    const newErrors = {};
    if (!resourceType) newErrors.resource_type = "Please select a resource type";
    if (resourceType === "plugin" && !pluginResourceTypeId)
      newErrors.resource_type = "Please select a resource type";
    if (!payload.name?.trim()) newErrors.name = "Name is required";
    if (payload.name && payload.name.length > 200) newErrors.name = "Name must be under 200 characters";

    // Validate documentation URL — must be http or https (prevent javascript: XSS)
    if (meta.documentation_url && meta.documentation_url.trim()) {
      try {
        const parsed = new URL(meta.documentation_url);
        if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
          newErrors.documentation_url = "URL must use http or https protocol";
        }
      } catch {
        newErrors.documentation_url = "Must be a valid URL (e.g., https://docs.example.com)";
      }
    }

    // Validate primary contact has at minimum some content
    if (submitForReview && !meta.primary_contact?.trim()) {
      newErrors.primary_contact = "Primary contact is required when submitting for review";
    }

    // Validate email-like pattern in contacts if provided
    const emailPattern = /\S+@\S+\.\S+/;
    if (meta.primary_contact && meta.primary_contact.includes("@") && !emailPattern.test(meta.primary_contact)) {
      newErrors.primary_contact = "Contact must include a valid email address";
    }

    if (resourceType === "datasource") {
      if (!payload.db_source_type)
        newErrors.db_source_type = "Vector DB type is required";
      if (!payload.embed_vendor)
        newErrors.embed_vendor = "Embedder vendor is required";
      if (!payload.embed_model?.trim())
        newErrors.embed_model = "Embedding model is required";
    }

    if (resourceType === "tool") {
      if (!payload.oas_spec) newErrors.oas_spec = "OAS spec is required";
      // tool_type is defaulted in buildResourcePayload rather than here: a
      // setPayload during validation does not land before handleSave reads
      // `payload`, which is why submitted tools arrived with tool_type "" and
      // rendered as less capable than an identical admin-created tool.

      // An empty operation set produces a Tool that can never work, and it
      // would travel through a review and an approval before anybody found
      // out. Catch it at the front door.
      if (
        submitForReview &&
        specValidation?.extracted?.operations?.length > 0 &&
        !(payload.available_operations || "").split(",").filter(Boolean).length
      ) {
        newErrors.available_operations =
          "Select at least one operation to expose, or this tool will be approved unable to do anything";
      }
    }

    if (resourceType === "plugin") {
      // The server validates against the same schema and rejects with a 400;
      // catching it here keeps the message next to the field that caused it.
      if (pluginSchema) {
        const { errors: schemaErrors } = validator.validateFormData(
          payload,
          pluginSchema
        );
        if (schemaErrors.length > 0) {
          newErrors.resource_payload = schemaErrors
            .map((e) => e.stack || e.message)
            .join("; ");
        }
      } else if (pluginExtraJsonError) {
        newErrors.resource_payload = pluginExtraJsonError;
      }
    }

    // Privacy score range validation
    if (meta.suggested_privacy < 0 || meta.suggested_privacy > 100) {
      newErrors.suggested_privacy = "Privacy score must be between 0 and 100";
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSave = async (submitForReview = false) => {
    if (!validateForm(submitForReview)) return;

    // Check required attestations for submit
    if (submitForReview) {
      const requiredAttestations = attestations.filter(
        (a) =>
          a.required &&
          a.active &&
          (a.applies_to_type === resourceType ||
            a.applies_to_type === "all")
      );
      const unchecked = requiredAttestations.filter(
        (a) => !attestationChecks[a.id]
      );
      if (unchecked.length > 0) {
        setSnackbar({
          open: true,
          message: "Please accept all required attestations before submitting",
          severity: "error",
        });
        return;
      }
    }

    setSaving(true);
    try {
      // Defaults are applied here, on the value actually sent, rather than via
      // a setPayload that has not landed by the time this runs.
      const resourcePayload =
        resourceType === "tool" && !payload.tool_type
          ? { ...payload, tool_type: "REST" }
          : payload;

      const submissionData = {
        data: {
          attributes: {
            resource_type: resourceType,
            ...(resourceType === "plugin"
              ? { plugin_resource_type_id: pluginResourceTypeId }
              : {}),
            status: submitForReview ? "submitted" : "draft",
            resource_payload: resourcePayload,
            attestations: {
              accepted: Object.entries(attestationChecks)
                .filter(([, checked]) => checked)
                .map(([templateId]) => ({
                  template_id: parseInt(templateId),
                  accepted_at: new Date().toISOString(),
                })),
            },
            suggested_privacy: meta.suggested_privacy,
            privacy_justification: meta.privacy_justification,
            primary_contact: meta.primary_contact,
            secondary_contact: meta.secondary_contact,
            sla_expectation: meta.sla_expectation,
            documentation_url: meta.documentation_url,
            notes: meta.notes,
            data_cutoff_date: meta.data_cutoff_date || null,
          },
        },
      };

      if (isEdit) {
        await pubClient.patch(
          `/common/submissions/${id}`,
          submissionData
        );
        if (submitForReview) {
          await pubClient.post(`/common/submissions/${id}/submit`);
        }
      } else {
        await pubClient.post("/common/submissions", submissionData);
      }

      setSnackbar({
        open: true,
        message: submitForReview
          ? "Submission sent for review"
          : "Draft saved",
        severity: "success",
      });
      setTimeout(() => navigate("/portal/contributions"), 1500);
    } catch (error) {
      // A schema violation comes back as one error per failing field; show
      // them all, and keep them on the page next to the fields, not only in a
      // snackbar that disappears.
      const details = (error.response?.data?.errors || [])
        .map((e) => e.detail)
        .filter(Boolean);
      if (error.response?.status === 400 && details.length > 0) {
        setErrors((prev) => ({ ...prev, resource_payload: details.join("; ") }));
      }
      setSnackbar({
        open: true,
        message: details.length > 0 ? details.join("; ") : "Failed to save",
        severity: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <Container sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Container>
    );
  }

  const applicableAttestations = attestations.filter(
    (a) =>
      a.active &&
      (a.applies_to_type === resourceType || a.applies_to_type === "all")
  );

  return (
    <Container maxWidth={false} sx={{ px: 3, py: 3, width: "100%" }}>
      <Box sx={{ display: "flex", alignItems: "center", mb: 3 }}>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/portal/contributions")}
          color="inherit"
        >
          Back to My Contributions
        </Button>
      </Box>

      <Typography variant="h4" sx={{ mb: 3 }}>
        {isEdit ? "Edit Submission" : "Submit a Resource"}
      </Typography>

      {/* Resource type selector */}
      {!isEdit && (
        <FormControl fullWidth sx={{ mb: 3 }} error={!!errors.resource_type}>
          <InputLabel id="submissionform-resource-type-label">Resource Type</InputLabel>
          <Select
            labelId="submissionform-resource-type-label"
            value={
              resourceType === "plugin" && pluginResourceTypeId
                ? `${PLUGIN_TYPE_PREFIX}${pluginResourceTypeId}`
                : resourceType
            }
            label="Resource Type"
            onChange={(e) => handleResourceTypeChange(e.target.value)}
            renderValue={(value) => {
              if (value === "datasource") return "Data Source";
              if (value === "tool") return "Tool (OpenAPI)";
              const t = pluginTypes.find(
                (pt) => `${PLUGIN_TYPE_PREFIX}${pt.id}` === value
              );
              return t ? `${t.name} (${t.plugin_name})` : value;
            }}
          >
            <MenuItem value="datasource">Data Source</MenuItem>
            <MenuItem value="tool">Tool (OpenAPI)</MenuItem>
            {pluginTypes.map((t) => (
              <MenuItem key={t.id} value={`${PLUGIN_TYPE_PREFIX}${t.id}`}>
                <Box>
                  <Typography variant="body1" component="div">
                    {t.name}
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    component="div"
                  >
                    {t.plugin_name}
                  </Typography>
                </Box>
              </MenuItem>
            ))}
          </Select>
          {errors.resource_type && (
            <Typography variant="caption" color="error">
              {errors.resource_type}
            </Typography>
          )}
        </FormControl>
      )}

      {resourceType && (
        <Box component="form" onSubmit={(e) => e.preventDefault()}>
          {/* Basic info. For a plugin type the schema form is the whole
              resource section and always carries `name`, so these generic
              fields would only duplicate it. */}
          {resourceType !== "plugin" && (
            <>
            <Typography variant="h6" sx={{ mb: 2 }}>
              Basic Information
            </Typography>
            <Grid container spacing={2} sx={{ mb: 3 }}>
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Name"
                  value={payload.name || ""}
                  onChange={(e) => handlePayloadChange("name", e.target.value)}
                  onBlur={checkDuplicates}
                  error={!!errors.name}
                  helperText={errors.name}
                  required
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label={
                    resourceType === "datasource"
                      ? "Short Description"
                      : "Description"
                  }
                  value={
                    payload.short_description || payload.description || ""
                  }
                  onChange={(e) =>
                    handlePayloadChange(
                      resourceType === "datasource"
                        ? "short_description"
                        : "description",
                      e.target.value
                    )
                  }
                  multiline
                  rows={2}
                />
              </Grid>
            </Grid>
            </>
          )}

          {duplicateWarning && (
            <Alert severity="warning" sx={{ mb: 3 }}>
              <Typography variant="subtitle2">
                Possible duplicates found:
              </Typography>
              {duplicateWarning.map((d) => (
                <Typography key={d.id} variant="body2">
                  "{d.name}" — {d.match_reason}
                </Typography>
              ))}
            </Alert>
          )}

          {/* Plugin-provided resource type: the plugin's submission schema
              drives the whole resource section. Without a schema the type
              only promises a name, so that plus a free-form JSON blob. */}
          {resourceType === "plugin" && (
            <Accordion defaultExpanded>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant="h6">
                  {selectedPluginType?.name || "Resource Details"}
                </Typography>
              </AccordionSummary>
              <AccordionDetails>
                {selectedPluginType?.description && (
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ mb: 2 }}
                  >
                    {selectedPluginType.description}
                  </Typography>
                )}
                {selectedPluginType?.plugin_name && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    display="block"
                    sx={{ mb: 2 }}
                  >
                    Provided by the {selectedPluginType.plugin_name} plugin
                  </Typography>
                )}
                {(errors.name || errors.resource_payload) && (
                  <Alert severity="error" sx={{ mb: 2 }}>
                    {errors.name && (
                      <Typography variant="body2">{errors.name}</Typography>
                    )}
                    {errors.resource_payload && (
                      <Typography variant="body2">
                        {errors.resource_payload}
                      </Typography>
                    )}
                  </Alert>
                )}
                {pluginSchema ? (
                  <SchemaFormRenderer
                    schema={pluginSchema}
                    formData={payload}
                    onChange={(data) => setPayload(data || {})}
                  />
                ) : (
                  <Grid container spacing={2}>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="Name"
                        value={payload.name || ""}
                        onChange={(e) =>
                          handlePayloadChange("name", e.target.value)
                        }
                        error={!!errors.name}
                        helperText={errors.name}
                        required
                      />
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="Additional fields (JSON)"
                        multiline
                        rows={8}
                        value={pluginExtraJson}
                        onChange={(e) =>
                          handlePluginExtraJsonChange(e.target.value)
                        }
                        error={!!pluginExtraJsonError}
                        helperText={
                          pluginExtraJsonError ||
                          "Optional. A JSON object of any further fields this resource type expects."
                        }
                        sx={{
                          "& .MuiInputBase-input": {
                            fontFamily: "monospace",
                            fontSize: "0.85rem",
                          },
                        }}
                      />
                    </Grid>
                  </Grid>
                )}
              </AccordionDetails>
            </Accordion>
          )}

          {/* Datasource-specific fields */}
          {resourceType === "datasource" && (
            <>
              <Accordion defaultExpanded>
                <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                  <Typography variant="h6">
                    Vector Database Access Details
                  </Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <Grid container spacing={2}>
                    <Grid item xs={12} sm={6}>
                      <FormControl
                        fullWidth
                        error={!!errors.db_source_type}
                      >
                        <InputLabel id="submissionform-vector-database-type-label">Vector Database Type</InputLabel>
                        <Select
                          labelId="submissionform-vector-database-type-label"
                          value={payload.db_source_type || ""}
                          label="Vector Database Type"
                          onChange={(e) =>
                            handlePayloadChange(
                              "db_source_type",
                              e.target.value
                            )
                          }
                        >
                          {vectorStoreOptions.map((vs) => (
                            <MenuItem key={vs} value={vs}>
                              {vs}
                            </MenuItem>
                          ))}
                        </Select>
                      </FormControl>
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="Database / Namespace"
                        value={payload.db_name || ""}
                        onChange={(e) =>
                          handlePayloadChange("db_name", e.target.value)
                        }
                      />
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="Connection String"
                        value={payload.db_conn_string || ""}
                        onChange={(e) =>
                          handlePayloadChange(
                            "db_conn_string",
                            e.target.value
                          )
                        }
                        onBlur={checkDuplicates}
                        helperText={
                          payload.db_conn_string === "[redacted]"
                            ? "Saved credential — clear and re-enter to change"
                            : undefined
                        }
                      />
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="Database API Key"
                        type={showApiKey ? "text" : "password"}
                        value={payload.db_conn_api_key || ""}
                        onChange={(e) =>
                          handlePayloadChange(
                            "db_conn_api_key",
                            e.target.value
                          )
                        }
                        helperText={
                          payload.db_conn_api_key === "[redacted]"
                            ? "Saved credential — clear and re-enter to change"
                            : undefined
                        }
                        InputProps={{
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                onClick={() => setShowApiKey(!showApiKey)}
                                edge="end"
                              >
                                {showApiKey ? (
                                  <VisibilityOffIcon />
                                ) : (
                                  <VisibilityIcon />
                                )}
                              </IconButton>
                            </InputAdornment>
                          ),
                        }}
                      />
                    </Grid>
                  </Grid>
                </AccordionDetails>
              </Accordion>

              <Accordion defaultExpanded>
                <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                  <Typography variant="h6">
                    Embedding Service Details
                  </Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <Grid container spacing={2}>
                    <Grid item xs={12} sm={6}>
                      <FormControl
                        fullWidth
                        error={!!errors.embed_vendor}
                      >
                        <InputLabel id="submissionform-embedder-vendor-label">Embedder Vendor</InputLabel>
                        <Select
                          labelId="submissionform-embedder-vendor-label"
                          value={payload.embed_vendor || ""}
                          label="Embedder Vendor"
                          onChange={(e) =>
                            handleEmbedVendorChange(e.target.value)
                          }
                        >
                          {embedders.map((e) => (
                            <MenuItem key={e} value={e}>
                              {e}
                            </MenuItem>
                          ))}
                        </Select>
                      </FormControl>
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="Embedding Model"
                        value={payload.embed_model || ""}
                        onChange={(e) =>
                          handlePayloadChange("embed_model", e.target.value)
                        }
                      />
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="Service URL"
                        value={payload.embed_url || ""}
                        onChange={(e) =>
                          handlePayloadChange("embed_url", e.target.value)
                        }
                      />
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="Embedding API Key"
                        type={showEmbedKey ? "text" : "password"}
                        value={payload.embed_api_key || ""}
                        onChange={(e) =>
                          handlePayloadChange(
                            "embed_api_key",
                            e.target.value
                          )
                        }
                        helperText={
                          payload.embed_api_key === "[redacted]"
                            ? "Saved credential — clear and re-enter to change"
                            : undefined
                        }
                        InputProps={{
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                onClick={() =>
                                  setShowEmbedKey(!showEmbedKey)
                                }
                                edge="end"
                              >
                                {showEmbedKey ? (
                                  <VisibilityOffIcon />
                                ) : (
                                  <VisibilityIcon />
                                )}
                              </IconButton>
                            </InputAdornment>
                          ),
                        }}
                      />
                    </Grid>
                    {/* Connection testing is admin-only — available during review */}
                  </Grid>
                </AccordionDetails>
              </Accordion>
            </>
          )}

          {/* Tool-specific fields */}
          {resourceType === "tool" && (
            <Accordion defaultExpanded>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant="h6">OpenAPI Specification</Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Grid container spacing={2}>
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      label="Paste OpenAPI Spec (YAML or JSON)"
                      multiline
                      rows={10}
                      value={payload.oas_spec_raw || ""}
                      onChange={(e) => {
                        handlePayloadChange("oas_spec_raw", e.target.value);
                        // Base64 encode for backend (Unicode-safe)
                        try {
                          const encoded = btoa(
                            unescape(encodeURIComponent(e.target.value))
                          );
                          handlePayloadChange("oas_spec", encoded);
                        } catch (err) {
                          // Encoding failed — clear the encoded value
                          handlePayloadChange("oas_spec", "");
                        }
                      }}
                      error={!!errors.oas_spec}
                      helperText={
                        errors.oas_spec || "Paste your OpenAPI 3.x spec in YAML or JSON format"
                      }
                      sx={{
                        "& .MuiInputBase-input": {
                          fontFamily: "monospace",
                          fontSize: "0.85rem",
                        },
                      }}
                    />
                  </Grid>
                  <Grid item xs={12}>
                    <PrimaryOutlineButton
                      onClick={handleValidateSpec}
                      disabled={!payload.oas_spec}
                    >
                      Validate Spec
                    </PrimaryOutlineButton>
                    {specValidation && (
                      <Box sx={{ mt: 1 }}>
                        {specValidation.valid ? (
                          <Alert severity="success">
                            Spec is valid.{" "}
                            {specValidation.extracted?.operations?.length || 0}{" "}
                            operations found.
                          </Alert>
                        ) : (
                          <Alert severity="error">
                            <Typography variant="subtitle2">
                              Validation errors:
                            </Typography>
                            {specValidation.errors?.map((err, i) => (
                              <Typography key={i} variant="body2">
                                [{err.field}] {err.message}
                              </Typography>
                            ))}
                          </Alert>
                        )}
                        {specValidation.warnings?.length > 0 && (
                          <Alert severity="warning" sx={{ mt: 1 }}>
                            {specValidation.warnings.map((w, i) => (
                              <Typography key={i} variant="body2">
                                [{w.field}] {w.message}
                              </Typography>
                            ))}
                          </Alert>
                        )}
                        {specValidation.extracted?.operations?.length >
                          0 && (
                          <Box sx={{ mt: 1 }}>
                            <Typography
                              variant="body2"
                              sx={{ mb: 0.5 }}
                            >
                              Select operations to expose:
                            </Typography>
                            {errors.available_operations && (
                              <Alert severity="error" sx={{ mb: 1 }}>
                                {errors.available_operations}
                              </Alert>
                            )}
                            <Box
                              sx={{
                                display: "flex",
                                flexWrap: "wrap",
                                gap: 0.5,
                              }}
                            >
                              {specValidation.extracted.operations.map(
                                (op) => (
                                  <Chip
                                    key={op}
                                    label={op}
                                    size="small"
                                    variant={
                                      (
                                        payload.available_operations || ""
                                      )
                                        .split(",")
                                        .includes(op)
                                        ? "filled"
                                        : "outlined"
                                    }
                                    color={
                                      (
                                        payload.available_operations || ""
                                      )
                                        .split(",")
                                        .includes(op)
                                        ? "primary"
                                        : "default"
                                    }
                                    onClick={() => {
                                      const current = (
                                        payload.available_operations || ""
                                      )
                                        .split(",")
                                        .filter(Boolean);
                                      const next = current.includes(op)
                                        ? current.filter((o) => o !== op)
                                        : [...current, op];
                                      handlePayloadChange(
                                        "available_operations",
                                        next.join(",")
                                      );
                                    }}
                                  />
                                )
                              )}
                            </Box>
                          </Box>
                        )}
                      </Box>
                    )}
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <TextField
                      fullWidth
                      label="Auth Scheme Name"
                      value={payload.auth_schema_name || ""}
                      onChange={(e) =>
                        handlePayloadChange(
                          "auth_schema_name",
                          e.target.value
                        )
                      }
                      helperText="e.g., ApiKeyAuth, BearerAuth"
                    />
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <TextField
                      fullWidth
                      label="Auth Key / Token"
                      type="password"
                      value={payload.auth_key || ""}
                      onChange={(e) =>
                        handlePayloadChange("auth_key", e.target.value)
                      }
                      helperText={
                        payload.auth_key === "[redacted]"
                          ? "Saved credential — clear and re-enter to change"
                          : undefined
                      }
                    />
                  </Grid>
                </Grid>
              </AccordionDetails>
            </Accordion>
          )}

          {/* Privacy score */}
          <Accordion defaultExpanded sx={{ mt: 2 }}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography variant="h6">Privacy & Governance</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Grid container spacing={2}>
                <Grid item xs={12}>
                  <Typography id="suggested-privacy-label" gutterBottom>
                    Suggested Privacy Score
                  </Typography>
                  {/* The slider carried no accessible name, and a value that
                      decides whether the resource can ever be used deserves a
                      typable input, not only a drag target. */}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
                    <Slider
                      aria-labelledby="suggested-privacy-label"
                      value={meta.suggested_privacy}
                      onChange={(e, val) =>
                        handleMetaChange("suggested_privacy", val)
                      }
                      min={0}
                      max={100}
                      valueLabelDisplay="auto"
                      sx={{ flexGrow: 1 }}
                    />
                    <TextField
                      type="number"
                      size="small"
                      label="Score"
                      value={meta.suggested_privacy}
                      onChange={(e) => {
                        const v = Number(e.target.value);
                        if (Number.isNaN(v)) return;
                        handleMetaChange(
                          "suggested_privacy",
                          Math.min(100, Math.max(0, v))
                        );
                      }}
                      inputProps={{ min: 0, max: 100, "aria-label": "Suggested privacy score" }}
                      sx={{ width: 96 }}
                    />
                  </Box>
                  <Typography variant="caption" color="text.secondary">
                    0 = public data, 100 = highly sensitive. Admin will set the
                    final score.
                  </Typography>
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Privacy Justification"
                    value={meta.privacy_justification}
                    onChange={(e) =>
                      handleMetaChange(
                        "privacy_justification",
                        e.target.value
                      )
                    }
                    multiline
                    rows={2}
                    helperText="Explain what kind of data this resource contains"
                  />
                </Grid>
              </Grid>
            </AccordionDetails>
          </Accordion>

          {/* Support metadata.
              Collapsed by default, but its fields render into the DOM anyway --
              so they were invisible to the contributor, who skipped them, and
              the reviewer was then left with nobody to contact. The primary
              contact is the field the review page most wants to have, so this
              opens by default like every other section on the form. */}
          <Accordion defaultExpanded sx={{ mt: 2 }}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography variant="h6">
                Support &amp; Documentation
              </Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Grid container spacing={2}>
                <Grid item xs={12} sm={6}>
                  <TextField
                    fullWidth
                    label="Primary Contact"
                    value={meta.primary_contact}
                    onChange={(e) =>
                      handleMetaChange("primary_contact", e.target.value)
                    }
                    error={!!errors.primary_contact}
                    helperText={errors.primary_contact || "Name and email"}
                  />
                </Grid>
                <Grid item xs={12} sm={6}>
                  <TextField
                    fullWidth
                    label="Secondary Contact"
                    value={meta.secondary_contact}
                    onChange={(e) =>
                      handleMetaChange("secondary_contact", e.target.value)
                    }
                  />
                </Grid>
                <Grid item xs={12} sm={6}>
                  <TextField
                    fullWidth
                    label="Documentation URL"
                    value={meta.documentation_url}
                    onChange={(e) =>
                      handleMetaChange("documentation_url", e.target.value)
                    }
                    error={!!errors.documentation_url}
                    helperText={errors.documentation_url}
                  />
                </Grid>
                <Grid item xs={12} sm={6}>
                  <TextField
                    fullWidth
                    label="SLA Expectation"
                    value={meta.sla_expectation}
                    onChange={(e) =>
                      handleMetaChange("sla_expectation", e.target.value)
                    }
                    helperText="e.g., 99.9% uptime during business hours"
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Notes"
                    value={meta.notes}
                    onChange={(e) =>
                      handleMetaChange("notes", e.target.value)
                    }
                    multiline
                    rows={3}
                  />
                </Grid>
              </Grid>
            </AccordionDetails>
          </Accordion>

          {/* Attestations */}
          {applicableAttestations.length > 0 && (
            <Box sx={{ mt: 3 }}>
              <Typography variant="h6" gutterBottom>
                Attestations
              </Typography>
              {applicableAttestations.map((att) => (
                <FormControlLabel
                  key={att.id}
                  control={
                    <Checkbox
                      checked={attestationChecks[att.id] || false}
                      onChange={(e) =>
                        setAttestationChecks((prev) => ({
                          ...prev,
                          [att.id]: e.target.checked,
                        }))
                      }
                    />
                  }
                  label={
                    <Box sx={{ "& p": { m: 0 }, "& a": { color: "primary.main" } }}>
                      <ReactMarkdown
                        components={{
                          p: ({ children }) => (
                            <Typography variant="body2" component="span">
                              {children}
                            </Typography>
                          ),
                          a: ({ href, children }) => (
                            <a href={href} target="_blank" rel="noopener noreferrer">
                              {children}
                            </a>
                          ),
                        }}
                      >
                        {att.text}
                      </ReactMarkdown>
                      {att.required && (
                        <Typography
                          component="span"
                          variant="caption"
                          color="error"
                        >
                          {" "}
                          (required)
                        </Typography>
                      )}
                    </Box>
                  }
                />
              ))}
            </Box>
          )}

          {/* Action buttons */}
          <Box sx={{ mt: 4, display: "flex", gap: 2 }}>
            <PrimaryOutlineButton
              onClick={() => handleSave(false)}
              disabled={saving}
            >
              Save Draft
            </PrimaryOutlineButton>
            <PrimaryButton
              onClick={() => handleSave(true)}
              disabled={saving}
            >
              {saving ? (
                <CircularProgress size={20} />
              ) : (
                "Submit for Review"
              )}
            </PrimaryButton>
          </Box>
        </Box>
      )}

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Container>
  );
};

export default SubmissionForm;
