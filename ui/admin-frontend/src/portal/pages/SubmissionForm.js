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
import { PrimaryButton, PrimaryOutlineButton } from "../../admin/styles/sharedStyles";
import {
  fetchVendors,
  getEmbedderDefaultModel,
  getEmbedderDefaultUrl,
} from "../../admin/utils/vendorUtils";

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
    suggested_privacy: 50,
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

    if (isEdit) {
      loadSubmission();
    }
  }, [id]);

  const loadSubmission = async () => {
    try {
      setLoading(true);
      const response = await pubClient.get(`/common/submissions/${id}`);
      const data = response.data.data;
      setResourceType(data.resource_type);
      setPayload(data.resource_payload || {});
      setMeta({
        suggested_privacy: data.suggested_privacy || 50,
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
      setSpecValidation(response.data.data);
    } catch (error) {
      setSpecValidation({
        valid: false,
        errors: [{ field: "oas_spec", message: "Validation request failed" }],
      });
    }
  };

  const validateForm = (submitForReview = false) => {
    const newErrors = {};
    if (!resourceType) newErrors.resource_type = "Please select a resource type";
    if (!payload.name?.trim()) newErrors.name = "Name is required";
    if (payload.name && payload.name.length > 200) newErrors.name = "Name must be under 200 characters";

    // Validate documentation URL format
    if (meta.documentation_url && meta.documentation_url.trim()) {
      try {
        new URL(meta.documentation_url);
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
      if (!payload.tool_type) {
        // Auto-set tool_type for tools
        payload.tool_type = "REST";
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
      const submissionData = {
        data: {
          attributes: {
            resource_type: resourceType,
            status: submitForReview ? "submitted" : "draft",
            resource_payload: payload,
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
      setSnackbar({
        open: true,
        message:
          error.response?.data?.errors?.[0]?.detail || "Failed to save",
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
          <InputLabel>Resource Type</InputLabel>
          <Select
            value={resourceType}
            label="Resource Type"
            onChange={(e) => {
              setResourceType(e.target.value);
              setPayload({});
              setTestResult(null);
              setSpecValidation(null);
            }}
          >
            <MenuItem value="datasource">Data Source</MenuItem>
            <MenuItem value="tool">Tool (OpenAPI)</MenuItem>
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
          {/* Basic info */}
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
                        <InputLabel>Vector Database Type</InputLabel>
                        <Select
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
                        <InputLabel>Embedder Vendor</InputLabel>
                        <Select
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
                        // Base64 encode for backend
                        try {
                          handlePayloadChange(
                            "oas_spec",
                            btoa(e.target.value)
                          );
                        } catch (err) {
                          // Non-ASCII chars
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
                  <Typography gutterBottom>
                    Suggested Privacy Score: {meta.suggested_privacy}
                  </Typography>
                  <Slider
                    value={meta.suggested_privacy}
                    onChange={(e, val) =>
                      handleMetaChange("suggested_privacy", val)
                    }
                    min={0}
                    max={100}
                    valueLabelDisplay="auto"
                  />
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

          {/* Support metadata */}
          <Accordion sx={{ mt: 2 }}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography variant="h6">
                Support & Documentation (Optional)
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
