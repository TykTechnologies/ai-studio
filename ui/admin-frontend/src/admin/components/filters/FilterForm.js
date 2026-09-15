import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Box,
  Typography,
  Grid,
  Snackbar,
  Alert,
  FormControlLabel,
  Checkbox,
  FormHelperText,
  ToggleButton,
  ToggleButtonGroup,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  SecondaryLinkButton,
  SecondaryOutlineButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../../styles/sharedStyles";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../../components/unsaved-changes";
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";
import Editor from "react-simple-code-editor";
import { highlight, languages } from "prismjs/components/prism-core";
import "prismjs/components/prism-clike";
import "prismjs/components/prism-javascript";
import "prismjs/themes/prism-tomorrow.css";
import ScriptTemplateSelector from "./ScriptTemplateSelector";
import ScriptTestPanel from "./ScriptTestPanel";
import GuardrailConfigForm, { emptyGuardrailConfig } from "./GuardrailConfigForm";

export const FILTER_KIND_SCRIPT = "script";
export const FILTER_KIND_GUARDRAIL = "guardrail";

const FilterForm = () => {
  const [filter, setFilter] = useState({
    name: "",
    description: "",
    script: "",
    response_filter: false, // Response filter checkbox
    namespace: "", // Added for edge availability
    kind: FILTER_KIND_SCRIPT,
    config: null,
  });
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [loaded, setLoaded] = useState(false);
  const navigate = useNavigate();
  const { id } = useParams();

  // Unsaved-changes tracking over the editable fields only (the fetched
  // filter also carries ids and timestamps, which never change here).
  const { markSaved } = useUnsavedForm(
    {
      name: filter.name,
      description: filter.description,
      script: filter.script,
      response_filter: filter.response_filter,
      namespace: filter.namespace,
      kind: filter.kind,
      config: JSON.stringify(filter.config || null),
    },
    { ready: !id || loaded }
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/filters"));

  useEffect(() => {
    if (id) {
      fetchFilter();
    }
  }, [id]);

  const fetchFilter = async () => {
    try {
      const response = await apiClient.get(`/filters/${id}`);
      const filterData = response.data.attributes; // Remove .data here
      setFilter({
        ...filterData,
        script: filterData.script ? atob(filterData.script) : "", // Decode base64
        response_filter: filterData.response_filter || false, // Response filter flag
        namespace: filterData.namespace || "",
        kind: filterData.kind || FILTER_KIND_SCRIPT,
        config: filterData.config || null,
      });
      setLoaded(true);
    } catch (error) {
      console.error("Error fetching filter", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch filter details",
        severity: "error",
      });
    }
  };

  const isGuardrail = filter.kind === FILTER_KIND_GUARDRAIL;

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFilter({ ...filter, [name]: value });
  };

  const handleCheckboxChange = (e) => {
    const { name, checked } = e.target;
    setFilter({ ...filter, [name]: checked });
  };

  const handleKindChange = (e, kind) => {
    if (!kind) return;
    setFilter({
      ...filter,
      kind,
      config: kind === FILTER_KIND_GUARDRAIL ? filter.config || emptyGuardrailConfig() : filter.config,
    });
    setErrors({});
  };

  const handleConfigChange = (config) => {
    setFilter({ ...filter, config });
  };

  const handleNamespaceChange = (namespaces) => {
    // Convert array to comma-delimited string, or empty string for global
    const namespaceString = Array.isArray(namespaces) ? namespaces.join(', ') : namespaces;
    setFilter({ ...filter, namespace: namespaceString });
  };

  const handleTemplateSelect = (templateScript) => {
    setFilter({ ...filter, script: templateScript });
  };

  const handleScriptChange = (code) => {
    setFilter({ ...filter, script: code });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!filter.name.trim()) newErrors.name = "Name is required";
    if (isGuardrail) {
      const config = filter.config || {};
      if (!config.provider) newErrors.provider = "Choose a provider";
      if (!config.detectors || config.detectors.length === 0) newErrors.detectors = "Enable at least one detector";
    } else if (!filter.script.trim()) {
      newErrors.script = "Script is required";
    }
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const { config, ...rest } = filter;
    const attributes = {
      ...rest,
      kind: filter.kind,
      script: isGuardrail ? "" : btoa(filter.script), // Encode to base64
    };
    if (isGuardrail) {
      attributes.config = config;
    }
    const filterData = {
      data: {
        type: "filter", // Changed to lowercase "filter"
        attributes,
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/filters/${id}`, filterData);
      } else {
        await apiClient.post("/filters", filterData);
      }

      markSaved();
      navigate("/admin/filters", {
        state: {
          snackbar: {
            message: id
              ? "Filter updated successfully"
              : "Filter created successfully",
            severity: "success",
          },
        },
      });
    } catch (error) {
      console.error("Error saving filter", error);
      // The server names what is wrong with a guardrail config; show that
      // rather than a generic failure.
      const detail = error.response?.data?.errors?.[0]?.detail;
      setSnackbar({
        open: true,
        message: detail ? `Failed to save filter: ${detail}` : "Failed to save filter. Please try again.",
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
        <Typography variant="headingXLarge">
          {id ? "Edit filter" : "Add filter"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/filters"
          color="inherit"
        >
          Back to filters
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Filters are used as a security layer to process and modify data before it is passed to the LLM. For example, filters can remove personally identifiable information to ensure privacy.</Typography>
      </Box>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                name="name"
                value={filter.name}
                onChange={handleChange}
                error={!!errors.name}
                helperText={errors.name}
                required
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Description"
                name="description"
                value={filter.description}
                onChange={handleChange}
                multiline
                rows={3}
              />
            </Grid>
            <Grid item xs={12}>
              <Typography variant="subtitle2" gutterBottom>
                Filter type
              </Typography>
              <ToggleButtonGroup
                exclusive
                value={filter.kind}
                onChange={handleKindChange}
                size="small"
                aria-label="Filter type"
              >
                <ToggleButton value={FILTER_KIND_GUARDRAIL} data-testid="filter-kind-guardrail">
                  Guardrail
                </ToggleButton>
                <ToggleButton value={FILTER_KIND_SCRIPT} data-testid="filter-kind-script">
                  Script
                </ToggleButton>
              </ToggleButtonGroup>
              <FormHelperText sx={{ mt: 1 }}>
                {isGuardrail
                  ? "A guardrail runs a detection provider (the built-in pattern library, or an external classifier or NER service) and blocks, redacts or logs on its verdict. No code."
                  : "A script runs Tengo code you write against the request or response."}
              </FormHelperText>
            </Grid>
            <Grid item xs={12}>
              <Box sx={{ mb: 2 }}>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={filter.response_filter || false}
                      onChange={handleCheckboxChange}
                      name="response_filter"
                    />
                  }
                  label="Is this a Response Filter?"
                />
                <FormHelperText sx={{ ml: 4, mt: 0 }}>
                  Response filters run on LLM responses only (not tools). They can only block responses, not modify them. Streaming responses will be interrupted if blocked.
                </FormHelperText>
              </Box>
            </Grid>

            {isGuardrail ? (
              <Grid item xs={12}>
                <GuardrailConfigForm
                  value={filter.config}
                  onChange={handleConfigChange}
                  responseFilter={!!filter.response_filter}
                  errors={errors}
                />
              </Grid>
            ) : (
              <>
                <Grid item xs={12}>
                  <ScriptTemplateSelector
                    onTemplateSelect={handleTemplateSelect}
                    currentScript={filter.script}
                    filterType={filter.response_filter ? "response" : "request"}
                  />
                </Grid>

                <Grid item xs={12}>
                  <Typography variant="subtitle2" gutterBottom>
                    Script *
                  </Typography>
                  <Box
                    sx={{
                      border: errors.script ? "1px solid #d32f2f" : "1px solid #444",
                      borderRadius: "4px",
                      minHeight: "400px",
                      "& textarea": {
                        outline: "none !important",
                      },
                    }}
                  >
                    <Editor
                      value={filter.script}
                      onValueChange={handleScriptChange}
                      highlight={(code) => highlight(code, languages.js, "javascript")}
                      padding={10}
                      style={{
                        fontFamily: '"Fira code", "Fira Mono", "Monaco", monospace',
                        fontSize: 14,
                        backgroundColor: "#2d2d2d",
                        color: "#ccc",
                        minHeight: "400px",
                      }}
                    />
                  </Box>
                  {errors.script && (
                    <FormHelperText error>{errors.script}</FormHelperText>
                  )}
                </Grid>
              </>
            )}

            <Grid item xs={12}>
              <ScriptTestPanel
                script={filter.script}
                kind={filter.kind}
                config={filter.config}
                filterId={id}
                filterType={filter.response_filter ? "response" : "request"}
              />
            </Grid>
          </Grid>

          {/* Edge Availability Section */}
          <EdgeAvailabilitySection
            value={filter.namespace}
            onChange={handleNamespaceChange}
            defaultExpanded={false}
          />

          <Box mt={4} display="flex" gap={2}>
            <SecondaryOutlineButton onClick={handleCancel}>
              Cancel
            </SecondaryOutlineButton>
            <PrimaryButton variant="contained" type="submit">
              {id ? "Update filter" : "Add filter"}
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

export default FilterForm;
