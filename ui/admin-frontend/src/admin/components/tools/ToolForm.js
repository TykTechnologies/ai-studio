import GovernedMetadataFields, { GOVERNED_METADATA_SECTION_ID } from "../metadata/GovernedMetadataFields";
import { extractGovernedMetadataErrors } from "../../services/governedMetadataService";
import React, { useState, useEffect, useRef, useMemo } from "react";
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
  Chip,
  Paper,
  Checkbox,
  FormControlLabel,
  FormGroup,
  AccordionSummary,
  AccordionDetails,
  IconButton,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import DeleteIcon from "@mui/icons-material/Delete";
import CloudUploadIcon from "@mui/icons-material/CloudUpload";
import {
  PrimaryOutlineButton,
  SecondaryOutlineButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
  SecondaryLinkButton
} from "../../styles/sharedStyles";
import RelationshipPicker from "../common/relationship-picker";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../../components/unsaved-changes";
import { styled } from "@mui/system";
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";
import PrivacyLevelInput from "../common/privacy/PrivacyLevelInput";
import { isValidPrivacyScore } from "../common/privacy/privacyLevels";
import PublishSwitch from "../rbac/PublishSwitch";
import { P } from "../../rbac/permissions";
import { usePermissions } from "../../context/PermissionsContext";
import { parseOpenAPIOperations } from "../../utils/openapiOperations";

const SectionTitle = ({ children, tooltip }) => (
  <Box sx={{ display: "flex", alignItems: "center", mt: 3, mb: 2 }}>
    <Typography variant="h6" gutterBottom sx={{ mr: 1 }}>
      {children}
    </Typography>
    {tooltip && (
      <Tooltip title={tooltip}>
        <HelpOutlineIcon color="action" fontSize="small" />
      </Tooltip>
    )}
  </Box>
);

const OperationsInput = ({ value, onChange }) => {
  const [inputValue, setInputValue] = useState("");
  const [operations, setOperations] = useState([]);

  useEffect(() => {
    if (Array.isArray(value) && value.length > 0) {
      setOperations(value);
    } else if (typeof value === "string" && value.trim() !== "") {
      setOperations(
        value
          .split(",")
          .map((op) => op.trim())
          .filter(Boolean),
      );
    } else {
      setOperations([]);
    }
  }, [value]);

  const handleInputChange = (event) => {
    setInputValue(event.target.value);
  };

  const handleInputKeyDown = (event) => {
    if (event.key === "," || event.key === "Enter") {
      event.preventDefault();
      if (inputValue.trim()) {
        const newOperations = [...operations, inputValue.trim()];
        setOperations(newOperations);
        onChange(newOperations);
        setInputValue("");
      }
    }
  };

  const handleDelete = (opToDelete) => {
    const newOperations = operations.filter((op) => op !== opToDelete);
    setOperations(newOperations);
    onChange(newOperations);
  };

  return (
    <Paper
      sx={{
        display: "flex",
        flexWrap: "wrap",
        padding: "5px",
        border: "1px solid #ccc",
        borderRadius: "4px",
      }}
    >
      {operations.map((op) => (
        <Chip
          key={op}
          label={op}
          onDelete={() => handleDelete(op)}
          sx={{ margin: "2px" }}
        />
      ))}
      <TextField
        value={inputValue}
        onChange={handleInputChange}
        onKeyDown={handleInputKeyDown}
        placeholder="Type and press comma or enter to add"
        sx={{ flexGrow: 1, "& fieldset": { border: "none" } }}
        autoComplete="off"
      />
    </Paper>
  );
};

// Checklist derived from the pasted spec: checked = allowed. Operations that
// are stored on the tool but no longer in the spec stay listed (checked) so a
// stale entry is never dropped silently.
const OperationsChecklist = ({ operations, selected, onChange }) => {
  const known = new Set(operations.map((op) => op.operationId));
  const stale = selected.filter((id) => !known.has(id));

  const toggle = (operationId) => {
    if (selected.includes(operationId)) {
      onChange(selected.filter((id) => id !== operationId));
    } else {
      onChange([...selected, operationId]);
    }
  };

  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, mb: 1 }}>
        <Button size="small" onClick={() => onChange(operations.map((op) => op.operationId))}>
          Select all
        </Button>
        <Button size="small" onClick={() => onChange([])}>
          Clear
        </Button>
        <Typography variant="caption" color="textSecondary" sx={{ alignSelf: "center", ml: 1 }}>
          {selected.length} of {operations.length + stale.length} allowed
        </Typography>
      </Box>
      <FormGroup data-testid="operations-checklist">
        {operations.map((op) => (
          <FormControlLabel
            key={op.operationId}
            control={
              <Checkbox
                checked={selected.includes(op.operationId)}
                onChange={() => toggle(op.operationId)}
                name={op.operationId}
              />
            }
            label={
              <Box>
                <Typography variant="body2" component="span" sx={{ fontFamily: "monospace" }}>
                  {op.operationId}
                </Typography>
                <Typography variant="caption" color="textSecondary" component="span" sx={{ ml: 1 }}>
                  {op.method} {op.path}
                  {op.summary ? ` — ${op.summary}` : ""}
                </Typography>
              </Box>
            }
          />
        ))}
        {stale.map((operationId) => (
          <FormControlLabel
            key={`stale-${operationId}`}
            control={<Checkbox checked onChange={() => toggle(operationId)} name={operationId} />}
            label={
              <Box>
                <Typography variant="body2" component="span" sx={{ fontFamily: "monospace" }}>
                  {operationId}
                </Typography>
                <Typography variant="caption" color="warning.main" component="span" sx={{ ml: 1 }}>
                  not in the pasted spec
                </Typography>
              </Box>
            }
          />
        ))}
      </FormGroup>
    </Box>
  );
};

const findInvalidCharPosition = (inputString) => {
  const validPattern = /^[\x20-\x7E\n\r\t]*$/;
  const lines = inputString.split("\n");

  for (let lineNum = 0; lineNum < lines.length; lineNum++) {
    const line = lines[lineNum];
    for (let colNum = 0; colNum < line.length; colNum++) {
      const char = line[colNum];
      if (!validPattern.test(char)) {
        return {
          line: lineNum + 1,
          column: colNum + 1,
          char: char,
        };
      }
    }
  }

  return null;
};

const StyledTextField = styled(TextField)({
  "& .MuiInputBase-root": {
    fontFamily: "monospace",
    fontSize: "14px",
  },
});

// A filter's response_filter flag decides which side of the tool call it
// governs: requests on the way to the tool, responses on the way back.
const filterDirectionLabel = (filter) =>
  filter?.attributes?.response_filter
    ? "Response filter — runs on the tool's result"
    : "Request filter — runs on arguments sent to the tool";

const ToolForm = () => {
  const { can } = usePermissions();
  const [tool, setTool] = useState({
    name: "",
    description: "",
    privacy_score: 0,
    // Tools default to live, but the API rejects an explicit `active: true`
    // from a caller without tools:publish, so start a new tool off for them
    // (the switch is disabled for them anyway and the API would have saved
    // a draft).
    active: can(P.TOOLS_PUBLISH),
    auth_schema_name: "",
    auth_key: "",
    oas_spec: "",
    operations: [], // the API expects a string array; "" was rejected on create
    namespace: "",
  });
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [oasSpecError, setOasSpecError] = useState(null);
  const [files, setFiles] = useState([]);
  const [availableFilters, setAvailableFilters] = useState([]);
  // Filters and dependencies are edited in the form and committed on save:
  // the submit handler diffs the selection against what was loaded and issues
  // the per-relationship POST/DELETE calls after the tool itself is saved.
  // (They used to commit on click, out of step with the rest of the form.)
  const [toolFilters, setToolFilters] = useState([]);
  const [loadedFilterIds, setLoadedFilterIds] = useState([]);
  const navigate = useNavigate();
  const { id } = useParams();
  const fileInputRef = useRef(null);
  const [availableTools, setAvailableTools] = useState([]);
  const [toolDependencies, setToolDependencies] = useState([]);
  const [loadedDependencyIds, setLoadedDependencyIds] = useState([]);
  // True once every part of the tool being edited is on screen.
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (id) {
      Promise.all([
        fetchTool(),
        fetchToolOperations(),
        fetchToolFilters(),
        fetchToolDependencies(),
      ]).finally(() => setLoaded(true));
    }
    fetchAvailableTools();
    fetchAvailableFilters();
  }, [id]);

  const fetchAvailableTools = async () => {
    try {
      const response = await apiClient.get("/tools");
      // Filter out the current tool from available dependencies
      const tools = response.data.data.filter((tool) => tool.id !== id);
      setAvailableTools(tools);
    } catch (error) {
      console.error("Error fetching available tools", error);
      setAvailableTools([]);
      setSnackbar({
        open: true,
        message: "Failed to fetch available tools",
        severity: "error",
      });
    }
  };

  const fetchToolDependencies = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}/dependencies`);
      const deps = response.data.data || [];
      setToolDependencies(deps);
      setLoadedDependencyIds(deps.map((dep) => String(dep.id)));
    } catch (error) {
      console.error("Error fetching tool dependencies", error);
      setToolDependencies([]);
      setSnackbar({
        open: true,
        message: "Failed to fetch tool dependencies",
        severity: "error",
      });
    }
  };

  // Turns a dependency POST failure into the message the old click-to-add
  // flow showed, so the circular-reference case is still named.
  const dependencyErrorMessage = (error) => {
    let errorMessage = "Failed to add dependency";

    // Handle specific error messages
    if (error.response?.data?.errors?.[0]?.detail) {
      const detail = error.response.data.errors[0].detail;
      if (detail.includes("circular reference")) {
        errorMessage =
          "Cannot add this dependency: it would create a circular reference";
      } else if (detail.includes("cannot depend on itself")) {
        errorMessage = "A tool cannot depend on itself";
      }
    }
    return errorMessage;
  };

  // Commits the Dependencies picker: adds what was selected since load and
  // removes what was deselected. Runs after the tool record is saved.
  const syncDependencies = async (toolId) => {
    const selectedIds = toolDependencies.map((dep) => String(dep.id));
    const toAdd = selectedIds.filter((depId) => !loadedDependencyIds.includes(depId));
    const toRemove = loadedDependencyIds.filter((depId) => !selectedIds.includes(depId));

    for (const depId of toAdd) {
      try {
        await apiClient.post(`/tools/${toolId}/dependencies/${depId}`);
      } catch (error) {
        console.error("Error adding dependency", error);
        throw new Error(dependencyErrorMessage(error));
      }
    }
    for (const depId of toRemove) {
      try {
        await apiClient.delete(`/tools/${toolId}/dependencies/${depId}`);
      } catch (error) {
        console.error("Error removing dependency", error);
        throw new Error("Failed to remove dependency");
      }
    }
    setLoadedDependencyIds(selectedIds);
  };

  // Governed metadata (Enterprise): values live beside the object and are
  // sent as attributes.governed_metadata; 422 field errors map back here.
  const [governedMetadata, setGovernedMetadata] = useState({});
  const [metadataErrors, setMetadataErrors] = useState({});

  const fetchTool = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}`);
      const fetchedTool = response.data.data.attributes;

      fetchedTool.oas_spec = fetchedTool.oas_spec
        ? atob(fetchedTool.oas_spec)
        : "";

      setTool(fetchedTool);
      setFiles(fetchedTool.file_stores || []);
      setGovernedMetadata(response.data.data.governed_metadata || {});
    } catch (error) {
      console.error("Error fetching tool", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch tool details",
        severity: "error",
      });
    }
  };

  const fetchToolOperations = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}/operations`);
      const operations = response.data.data.operations;
      setTool((prevTool) => ({
        ...prevTool,
        operations: operations,
      }));
    } catch (error) {
      console.error("Error fetching tool operations", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch tool operations",
        severity: "error",
      });
    }
  };

  const fetchAvailableFilters = async () => {
    try {
      const response = await apiClient.get("/filters");
      // Make sure we're accessing the correct part of the response
      setAvailableFilters(response.data || []); // Add fallback to empty array
    } catch (error) {
      console.error("Error fetching available filters", error);
      setAvailableFilters([]); // Set to empty array on error
      setSnackbar({
        open: true,
        message: "Failed to fetch available filters",
        severity: "error",
      });
    }
  };

  const fetchToolFilters = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}/filters`);
      // Make sure we're accessing the correct part of the response
      const loadedFilters = response.data.data || []; // Add fallback to empty array
      setToolFilters(loadedFilters);
      setLoadedFilterIds(loadedFilters.map((filter) => String(filter.id)));
    } catch (error) {
      console.error("Error fetching tool filters", error);
      setToolFilters([]); // Set to empty array on error
      setSnackbar({
        open: true,
        message: "Failed to fetch tool filters",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    if (name === "privacy_score") {
      const numValue = Math.min(Math.max(parseInt(value) || 0, 0), 100);
      setTool({ ...tool, [name]: numValue });
    } else if (name === "oas_spec") {
      const invalidChar = findInvalidCharPosition(value);
      if (invalidChar) {
        setOasSpecError(
          `Invalid character '${invalidChar.char}' found at line ${invalidChar.line}, column ${invalidChar.column}`,
        );
      } else {
        setOasSpecError(null);
      }
      setTool({ ...tool, [name]: value });
    } else {
      setTool({ ...tool, [name]: value });
    }
  };

  const handleOperationsChange = (value) => {
    setTool({ ...tool, operations: Array.isArray(value) ? value : [] });
  };

  // Operations offered by the pasted spec (null when it cannot be parsed).
  const specOperations = useMemo(() => parseOpenAPIOperations(tool.oas_spec), [tool.oas_spec]);
  const selectedOperations = useMemo(() => {
    if (Array.isArray(tool.operations)) return tool.operations;
    if (typeof tool.operations === "string" && tool.operations.trim()) {
      return tool.operations.split(",").map((op) => op.trim()).filter(Boolean);
    }
    return [];
  }, [tool.operations]);

  const handleNamespaceChange = (namespaces) => {
    const namespaceString = Array.isArray(namespaces) ? namespaces.join(', ') : namespaces;
    setTool({ ...tool, namespace: namespaceString });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!tool.name.trim()) newErrors.name = "Name is required";
    if (!tool.description.trim())
      newErrors.description = "Description is required";
    if (!isValidPrivacyScore(tool.privacy_score))
      newErrors.privacy_score = "Privacy level must be between 0 and 100";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // Commits the Filters picker the same way as syncDependencies.
  const syncFilters = async (toolId) => {
    const selectedIds = toolFilters.map((filter) => String(filter.id));
    const toAdd = selectedIds.filter((filterId) => !loadedFilterIds.includes(filterId));
    const toRemove = loadedFilterIds.filter((filterId) => !selectedIds.includes(filterId));

    for (const filterId of toAdd) {
      try {
        await apiClient.post(`/tools/${toolId}/filters/${filterId}`);
      } catch (error) {
        console.error("Error adding filter", error);
        throw new Error("Failed to add filter");
      }
    }
    for (const filterId of toRemove) {
      try {
        await apiClient.delete(`/tools/${toolId}/filters/${filterId}`);
      } catch (error) {
        console.error("Error removing filter", error);
        throw new Error("Failed to remove filter");
      }
    }
    setLoadedFilterIds(selectedIds);
  };

  // Unsaved-changes tracking over everything the form saves. Uploaded files
  // are excluded: the upload itself commits on selection.
  const { markSaved } = useUnsavedForm(
    {
      tool,
      governedMetadata,
      dependencyIds: toolDependencies.map((dep) => String(dep.id)),
      filterIds: toolFilters.map((filter) => String(filter.id)),
    },
    { ready: !id || loaded },
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/tools"));

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const toolData = {
      data: {
        type: "Tool",
        attributes: {
          ...tool,
          operations: selectedOperations,
          privacy_score: Number(tool.privacy_score),
          active: Boolean(tool.active),
          tool_type: "REST",
          oas_spec: tool.oas_spec ? btoa(tool.oas_spec) : "",
          governed_metadata: governedMetadata,
        },
      },
    };

    // Set once the tool record itself is saved, so a failure in the
    // relationship calls that follow can be reported as what it is.
    let savedToolId = null;
    try {
      if (id) {
        await apiClient.patch(`/tools/${id}`, toolData);
        savedToolId = id;
        await updateToolOperations();
      } else {
        const response = await apiClient.post("/tools", toolData);
        const newToolId = response.data.data.id;
        savedToolId = newToolId;
        await updateToolOperations(newToolId);
      }

      await syncDependencies(savedToolId);
      await syncFilters(savedToolId);

      markSaved();
      navigate("/admin/tools", {
        state: { snackbar: { message: id ? "Tool updated successfully" : "Tool created successfully", severity: "success" } },
      });
    } catch (error) {
      if (savedToolId && !error.response) {
        // The tool is saved; only a dependency or filter change was refused.
        console.error("Error saving tool relationships", error);
        setSnackbar({
          open: true,
          message: `${error.message}. The tool itself was saved.`,
          severity: "error",
        });
        return;
      }
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
      console.error("Error saving tool", error);
      setSnackbar({
        open: true,
        message: "Failed to save tool. Please try again.",
        severity: "error",
      });
    }
  };

  const updateToolOperations = async (toolId = id) => {
    const operations = Array.isArray(tool.operations)
      ? tool.operations
      : tool.operations.split(",").map((op) => op.trim());

    if (id) {
      const currentOperations = await apiClient.get(
        `/tools/${toolId}/operations`,
      );
      for (const operation of currentOperations.data.data.operations) {
        await apiClient.delete(`/tools/${toolId}/operations`, {
          data: { data: { type: "Operation", attributes: { operation } } },
        });
      }
    }

    for (const operation of operations) {
      if (operation) {
        await apiClient.post(`/tools/${toolId}/operations`, {
          data: { type: "Operation", attributes: { operation } },
        });
      }
    }
  };

  const handleFileUpload = async (event) => {
    const file = event.target.files[0];
    if (!file) return;

    try {
      const formData = new FormData();
      formData.append("file", file);
      formData.append("description", `Documentation for tool: ${tool.name}`);

      const fileStoreResponse = await apiClient.post("/filestore", formData, {
        headers: {
          "Content-Type": "multipart/form-data",
        },
      });

      const fileStoreId = fileStoreResponse.data.data.id;

      await apiClient.post(`/tools/${id}/filestores/${fileStoreId}`);

      const updatedToolResponse = await apiClient.get(`/tools/${id}`);
      setFiles(updatedToolResponse.data.data.attributes.file_stores || []);

      setSnackbar({
        open: true,
        message: "File uploaded successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error uploading file", error);
      setSnackbar({
        open: true,
        message: "Failed to upload file",
        severity: "error",
      });
    }

    event.target.value = "";
  };

  const handleDeleteFile = async (fileStoreId) => {
    try {
      await apiClient.delete(`/tools/${id}/filestores/${fileStoreId}`);

      setFiles(files.filter((file) => file.id !== fileStoreId));

      setSnackbar({
        open: true,
        message: "File removed successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error deleting file", error);
      setSnackbar({
        open: true,
        message: "Failed to remove file",
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
        <Typography variant="headingXLarge">{id ? "Edit tool" : "Add tool"}</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/tools"
          color="white"
        >
          Back to tools
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Tools are external services that enhance the AI's capabilities by providing access to additional data and functions within chat rooms. Defined by the OpenAPI specification, you can specify which operations the LLM can use to fulfill user requests effectively.</Typography>  
      </Box>
      <ContentBox sx={{ pt: 0 }}>
        <Box component="form" onSubmit={handleSubmit}>
          <SectionTitle>Tool Information</SectionTitle>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                name="name"
                value={tool.name}
                onChange={handleChange}
                error={!!errors.name}
                helperText={errors.name}
                required
                autoComplete="off"
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Description"
                name="description"
                value={tool.description}
                onChange={handleChange}
                error={!!errors.description}
                helperText={errors.description}
                multiline
                rows={4}
                required
                autoComplete="off"
              />
            </Grid>
            <Grid item xs={12}>
              {/* One privacy control everywhere (UX review M4): a named level
                  with the 0–100 score alongside. */}
              <PrivacyLevelInput
                value={tool.privacy_score}
                onChange={(score) => setTool((prev) => ({ ...prev, privacy_score: score }))}
                error={!!errors.privacy_score}
                helperText={errors.privacy_score}
              />
            </Grid>
            <Grid item xs={12}>
              {/* Same live switch as the LLM and data source forms (UX review
                  M4b). The API needs tools:publish to turn it on. */}
              <PublishSwitch
                permission={P.TOOLS_PUBLISH}
                checked={tool.active}
                onChange={(e) => setTool((prev) => ({ ...prev, active: e.target.checked }))}
                name="active"
                label="Active"
              />
              <Typography variant="caption" color="text.secondary" display="block">
                Available to the portal and gateway when on
              </Typography>
            </Grid>
          </Grid>

          <SectionTitle tooltip="Paste your OpenAPI Specification JSON or YAML here. This defines the structure and capabilities of your API.">
            OpenAPI Specification
          </SectionTitle>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <StyledTextField
                fullWidth
                label="OAS Spec"
                name="oas_spec"
                value={tool.oas_spec}
                onChange={handleChange}
                error={!!oasSpecError}
                helperText={oasSpecError}
                multiline
                rows={12}
                variant="outlined"
                autoComplete="off"
              />
            </Grid>
          </Grid>

          <SectionTitle tooltip="Tick the operations (endpoints) the LLM may call. They come from the operationId values in your OpenAPI Specification.">
            Operations
          </SectionTitle>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              {specOperations && specOperations.length > 0 ? (
                <OperationsChecklist
                  operations={specOperations}
                  selected={selectedOperations}
                  onChange={handleOperationsChange}
                />
              ) : (
                <>
                  <Alert severity="info" sx={{ mb: 2 }}>
                    {tool.oas_spec && tool.oas_spec.trim()
                      ? "The specification could not be parsed, or none of its operations has an operationId, so operations have to be typed by hand."
                      : "Paste an OpenAPI specification above to pick operations from a list, or type them by hand."}
                  </Alert>
                  <OperationsInput
                    value={tool.operations}
                    onChange={handleOperationsChange}
                  />
                  <Typography variant="caption" color="textSecondary">
                    Type an operation name and press comma or enter to add. Click on
                    a chip to remove it.
                  </Typography>
                </>
              )}
            </Grid>
          </Grid>

          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Dependencies</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="text.secondary" paragraph>
                Add other tools that this tool depends on. Dependencies are
                tools that need to be available for this tool to function
                properly.
              </Typography>
              <Grid container spacing={3}>
                <Grid item xs={12}>
                  <RelationshipPicker
                    itemLabel="dependency"
                    value={toolDependencies}
                    onChange={setToolDependencies}
                    options={availableTools}
                    getOptionLabel={(dependency) => dependency?.attributes?.name ?? ""}
                    getOptionSecondary={(dependency) => dependency?.attributes?.description}
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
                Filters govern this tool's traffic.
                A <strong>request</strong> filter runs on the arguments being
                sent to the tool, so it can redact sensitive information before
                it leaves, or block the call so the tool is never contacted. A{" "}
                <strong>response</strong> filter runs on the result before it is
                sent back to the LLM. Which one a filter is depends on how it
                was configured on the Filters page.
              </Typography>
              <Grid container spacing={3}>
                <Grid item xs={12}>
                  <RelationshipPicker
                    itemLabel="filter"
                    value={toolFilters}
                    onChange={setToolFilters}
                    options={availableFilters || []}
                    getOptionLabel={(filter) => filter?.attributes?.name ?? ""}
                    getOptionSecondary={filterDirectionLabel}
                  />
                </Grid>
              </Grid>
            </AccordionDetails>
          </StyledAccordion>

          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Authentication Details</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="text.secondary" paragraph>
                If your tool requires authentication, please ensure to provide
                the name of the Auth schema to use from the OAS Specification
                (only API Key and bearer token types are supported), as well as
                the API Key to use.
              </Typography>
              <Grid container spacing={3}>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Auth Schema Name"
                    name="auth_schema_name"
                    value={tool.auth_schema_name}
                    onChange={handleChange}
                    autoComplete="off"
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Auth Key"
                    name="auth_key"
                    type="password"
                    value={tool.auth_key}
                    onChange={handleChange}
                  />
                </Grid>
              </Grid>
            </AccordionDetails>
          </StyledAccordion>

          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Extra Context</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="text.secondary" paragraph>
                Upload additional documentation or context files for this tool.
                These files will be used to provide additional context during
                tool operation.
              </Typography>

              <List>
                {files.map((file) => (
                  <ListItem key={file.id}>
                    <ListItemText
                      primary={file.attributes.file_name}
                      secondary={`Size: ${file.attributes.length} bytes`}
                    />
                    <ListItemSecondaryAction>
                      <IconButton
                        edge="end"
                        aria-label="delete"
                        onClick={() => handleDeleteFile(file.id)}
                      >
                        <DeleteIcon />
                      </IconButton>
                    </ListItemSecondaryAction>
                  </ListItem>
                ))}
              </List>

              <input
                type="file"
                ref={fileInputRef}
                style={{ display: "none" }}
                onChange={handleFileUpload}
              />

              <PrimaryOutlineButton
                variant="contained"
                startIcon={<CloudUploadIcon />}
                onClick={() => fileInputRef.current.click()}
                sx={{ mt: 2 }}
              >
                Upload Additional Tool Documentation
              </PrimaryOutlineButton>
            </AccordionDetails>
          </StyledAccordion>

          {/* Governance metadata (Enterprise; hidden when no schema applies) */}
          <GovernedMetadataFields
            objectType="tool"
            value={governedMetadata}
            onChange={(next) => {
              setGovernedMetadata(next);
              setMetadataErrors({});
            }}
            errors={metadataErrors}
          />

          {/* Edge Availability Section (Enterprise only) */}
          <EdgeAvailabilitySection
            value={tool.namespace}
            onChange={handleNamespaceChange}
            defaultExpanded={false}
          />

          <Box mt={4} display="flex" gap={2}>
            <SecondaryOutlineButton onClick={handleCancel}>
              Cancel
            </SecondaryOutlineButton>
            <PrimaryButton variant="contained" type="submit">
              {id ? "Update tool" : "Add tool"}
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

export default ToolForm;
