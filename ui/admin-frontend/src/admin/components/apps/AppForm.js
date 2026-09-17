import React, { useState, useEffect, useCallback, useMemo } from "react";
import apiClient, { appToolAPI } from "../../utils/apiClient"; // Import appToolAPI
import { formatPrivacyLevel } from "../common/privacy/privacyLevels";
import {
  TextField,
  Box,
  Typography,
  Grid,
  Snackbar,
  Alert,
  AccordionSummary,
  AccordionDetails,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Switch,
  FormControlLabel,
  InputAdornment,
  Tooltip,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
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
import { useEdition } from "../../context/EditionContext";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../../components/unsaved-changes";
import { listAll } from "../../utils/listAll";

// The app stores relationships as id arrays (llm_ids, datasource_ids,
// tool_ids, plugin resource instance ids) and the API payload keeps that
// shape. The RelationshipPicker works on full items, so these two helpers
// translate at the edge: ids -> items for `value`, items -> ids on change. An
// id whose object is not in the loaded list (deleted, or the list has not
// arrived yet) is kept as a placeholder so saving never silently drops it.
const itemsForIds = (ids, list, idOf, placeholder) =>
  (ids || []).map((id) => {
    const found = (list || []).find((item) => String(idOf(item)) === String(id));
    return found || placeholder(id);
  });

const jsonApiName = (item) => item?.attributes?.name ?? String(item?.id ?? "");

const AppForm = () => {
  const [app, setApp] = useState({
    name: "",
    description: "",
    user_id: "",
    llm_ids: [],
    datasource_ids: [],
    tool_ids: [], // Added for tools
    monthly_budget: null,
    budget_start_date: null,
    namespace: "", // Added for edge availability
    metadata: {}, // Added for custom metadata
  });
  const [metadataJSON, setMetadataJSON] = useState("{}"); // JSON string for editor
  const [metadataError, setMetadataError] = useState("");
  const [credential, setCredential] = useState(null);
  const [users, setUsers] = useState([]);
  const [llms, setLLMs] = useState([]);
  const [datasources, setDatasources] = useState([]);
  const [availableTools, setAvailableTools] = useState([]);
  const [pluginResourceTypes, setPluginResourceTypes] = useState([]);
  const [pluginResourceInstances, setPluginResourceInstances] = useState({}); // { "pluginId:slug": [...instances] }
  const [pluginResourceSelections, setPluginResourceSelections] = useState({}); // { "pluginId:slug": [...selectedIds] }
  const [loaded, setLoaded] = useState(false);
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();
  const { isEnterprise } = useEdition(); // Get edition info

  // Unsaved-changes tracking over every field the form saves. The credential
  // "Active" switch is deliberately absent: it PATCHes on click and is
  // labelled as such, so it must never make the form look dirty.
  const dirtyValues = useMemo(
    () => ({ app, metadataJSON, pluginResourceSelections }),
    [app, metadataJSON, pluginResourceSelections],
  );
  const { markSaved } = useUnsavedForm(dirtyValues, { ready: !id || loaded });
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/apps"));

  const fetchCredential = useCallback(async (credentialId) => {
    try {
      const response = await apiClient.get(`/credentials/${credentialId}`);
      setCredential(response.data.data); // Store the full data object
    } catch (error) {
      console.error("Error fetching credential", error);
    }
  }, []);

  const fetchApp = useCallback(async () => {
    try {
      const response = await apiClient.get(`/apps/${id}`);
      const appData = response.data.data.attributes;
      const metadata = appData.metadata || {};
      setApp({
        ...appData,
        llm_ids: Array.isArray(appData.llm_ids)
          ? appData.llm_ids.map(String)
          : [],
        datasource_ids: Array.isArray(appData.datasource_ids)
          ? appData.datasource_ids.map(String)
          : [],
        tool_ids: Array.isArray(appData.tool_ids)
          ? appData.tool_ids.map(String)
          : [],
        namespace: appData.namespace || "",
        metadata: metadata,
      });
      setMetadataJSON(JSON.stringify(metadata, null, 2));

      // Load plugin resource selections from app response
      if (Array.isArray(appData.plugin_resources)) {
        const selections = {};
        for (const pr of appData.plugin_resources) {
          selections[`${pr.plugin_id}:${pr.resource_type_slug}`] =
            pr.instance_ids || [];
        }
        setPluginResourceSelections(selections);
      }

      if (appData.credential_id) {
        fetchCredential(appData.credential_id);
      }
    } catch (error) {
      console.error("Error fetching app", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch app details",
        severity: "error",
      });
    } finally {
      setLoaded(true);
    }
  }, [id, fetchCredential]);

  const fetchPluginResourceTypes = async () => {
    try {
      const response = await apiClient.get("/plugin-resource-types");
      const types = response.data.data || [];
      setPluginResourceTypes(types);

      // Fetch instances for each type
      for (const rt of types) {
        try {
          const instancesResp = await apiClient.get(
            `/plugin-resource-types/${rt.plugin_id}/${rt.slug}/instances`,
          );
          // TODO: this endpoint needs to be created — for now plugin RPC via the
          // existing Call mechanism will be used by the platform later.
          // Placeholder: store empty until endpoint is wired
          if (instancesResp.data && instancesResp.data.data) {
            setPluginResourceInstances((prev) => ({
              ...prev,
              [`${rt.plugin_id}:${rt.slug}`]: instancesResp.data.data,
            }));
          }
        } catch {
          // Instance fetch may not be available yet
        }
      }
    } catch {
      // Plugin resource types not available — that's fine
    }
  };

  useEffect(() => {
    fetchUsers();
    fetchLLMs();
    fetchDatasources();
    fetchTools();
    fetchPluginResourceTypes();
    if (id) {
      fetchApp();
    }
  }, [id, fetchApp]);

  // fetchApp is now defined using useCallback above

  const handleCredentialActiveToggle = async (event) => {
    const newActiveState = event.target.checked;

    try {
      const credentialInput = {
        data: {
          type: "credentials",
          attributes: {
            active: newActiveState,
          },
        },
      };

      await apiClient.patch(`/credentials/${credential.id}`, credentialInput);

      setCredential((prevState) => ({
        ...prevState,
        attributes: {
          ...prevState.attributes,
          active: newActiveState,
        },
      }));

      setSnackbar({
        open: true,
        message: `Credential ${newActiveState ? "activated" : "deactivated"} successfully`,
        severity: "success",
      });
    } catch (error) {
      console.error("Error updating credential active state", error);
      setSnackbar({
        open: true,
        message: "Failed to update credential state. Please try again.",
        severity: "error",
      });
    }
  };

  const fetchUsers = async () => {
    try {
      const response = await listAll(apiClient, "/users");
      setUsers(response.data.data || []);
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const fetchLLMs = async () => {
    try {
      const response = await listAll(apiClient, "/llms");
      setLLMs(response.data.data || []);
    } catch (error) {
      console.error("Error fetching LLMs", error);
    }
  };

  const fetchDatasources = async () => {
    try {
      const response = await listAll(apiClient, "/datasources");
      setDatasources(response.data.data || []);
    } catch (error) {
      console.error("Error fetching datasources", error);
    }
  };

  const fetchTools = async () => {
    try {
      const response = await appToolAPI.listAvailableTools();
      setAvailableTools(response.data.data || []);
    } catch (error) {
      console.error("Error fetching tools", error);
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setApp({ ...app, [name]: value });
  };

  const handleBudgetChange = (e) => {
    const value = e.target.value === '' ? null : parseFloat(e.target.value);
    setApp(prev => ({
      ...prev,
      monthly_budget: value,
      budget_start_date: value ? prev.budget_start_date || new Date().toISOString() : null
    }));
  };

  const handleBudgetStartDateChange = (e) => {
    const value = e.target.value ? new Date(e.target.value).toISOString() : null;
    setApp(prev => ({ ...prev, budget_start_date: value }));
  };

  // A RelationshipPicker hands back the full selected items; the form keeps
  // the id array the API expects.
  const handleRelationshipChange = (name) => (items) => {
    setApp((prev) => ({ ...prev, [name]: items.map((item) => String(item.id)) }));
  };

  const selectedLLMs = useMemo(
    () => itemsForIds(app.llm_ids, llms, (l) => l.id, (id) => ({ id, attributes: { name: String(id) } })),
    [app.llm_ids, llms],
  );
  const selectedDatasources = useMemo(
    () => itemsForIds(app.datasource_ids, datasources, (d) => d.id, (id) => ({ id, attributes: { name: String(id) } })),
    [app.datasource_ids, datasources],
  );
  const grantableTools = useMemo(
    () => availableTools.filter((t) => t.attributes?.app_grantable !== false),
    [availableTools],
  );
  const selectedTools = useMemo(
    () => itemsForIds(app.tool_ids, availableTools, (t) => t.id, (id) => ({ id, attributes: { name: String(id) } })),
    [app.tool_ids, availableTools],
  );

  const handleNamespaceChange = (namespaces) => {
    // Convert array to comma-delimited string, or empty string for global
    const namespaceString = Array.isArray(namespaces) ? namespaces.join(', ') : namespaces;
    setApp({ ...app, namespace: namespaceString });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!app.name.trim()) newErrors.name = "Name is required";
    if (!app.user_id) newErrors.user_id = "User ID is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    // Parse metadata JSON
    let parsedMetadata = {};
    if (metadataJSON.trim()) {
      try {
        parsedMetadata = JSON.parse(metadataJSON);
      } catch (err) {
        setSnackbar({
          open: true,
          message: "Invalid JSON in metadata field",
          severity: "error",
        });
        return;
      }
    }

    // Build plugin resource selections for API
    const pluginResourcesPayload = Object.entries(pluginResourceSelections)
      .filter(([, ids]) => ids.length > 0)
      .map(([key, ids]) => {
        const [pluginId, slug] = key.split(":");
        return {
          plugin_id: parseInt(pluginId, 10),
          resource_type_slug: slug,
          instance_ids: ids,
        };
      });

    const appPayload = {
      ...app,
      user_id: parseInt(app.user_id, 10),
      llm_ids: app.llm_ids.map((id) => parseInt(id, 10)),
      datasource_ids: app.datasource_ids.map((id) => parseInt(id, 10)),
      tool_ids: app.tool_ids.map((id) => parseInt(id, 10)),
      metadata: parsedMetadata,
      ...(pluginResourcesPayload.length > 0 && {
        plugin_resources: pluginResourcesPayload,
      }),
    };

    const appData = {
      data: {
        type: "apps",
        attributes: appPayload,
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/apps/${id}`, appData);
      } else {
        await apiClient.post("/apps", appData);
      }

      markSaved();
      navigate("/admin/apps", {
        state: { snackbar: { message: id ? "App updated successfully" : "App created successfully", severity: "success" } },
      });
    } catch (error) {
      console.error("Error saving app", error);
      setSnackbar({
        open: true,
        message: "Failed to save app. Please try again.",
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
        <Typography variant="headingXLarge">{id ? "Edit app" : "Add app"}</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/apps"
          color="inherit"
        >
          Back to apps
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Apps are used to grant developers direct access to LLMs and data sources in the AI Portal. With active credentials, an app can use the gateway API to work directly with LLMs or access the data source API to search through data. You can create apps for specific developers or set up catalogs so they can request access and customize their setup.</Typography>  
      </Box>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                name="name"
                value={app.name}
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
                value={app.description}
                onChange={handleChange}
                multiline
                rows={4}
              />
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth error={!!errors.user_id}>
                <InputLabel id="appform-user-label">User</InputLabel>
                <Select
                  labelId="appform-user-label"
                  name="user_id"
                  value={app.user_id}
                  onChange={handleChange}
                  required
                >
                  {users.map((user) => (
                    <MenuItem key={user.id} value={user.id}>
                      {user.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
                {errors.user_id && (
                  <Typography color="error">{errors.user_id}</Typography>
                )}
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <RelationshipPicker
                label="LLM providers"
                itemLabel="LLM provider"
                value={selectedLLMs}
                onChange={handleRelationshipChange("llm_ids")}
                options={llms}
                getOptionLabel={jsonApiName}
              />
            </Grid>
            <Grid item xs={12}>
              <Grid container spacing={2}>
                <Grid item xs={12} md={6}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <TextField
                      fullWidth
                      label="Monthly Budget"
                      name="monthly_budget"
                      type="number"
                      inputProps={{
                        step: "0.01",
                        min: "0"
                      }}
                      value={app.monthly_budget || ''}
                      onChange={handleBudgetChange}
                      disabled={!isEnterprise}
                      sx={{ opacity: isEnterprise ? 1 : 0.6 }}
                      InputProps={{
                        startAdornment: <InputAdornment position="start">$</InputAdornment>,
                      }}
                      helperText={isEnterprise ? "Leave empty for no budget limit" : "Budget enforcement is an Enterprise feature"}
                    />
                    {!isEnterprise && (
                      <Tooltip
                        title="Budget enforcement is an Enterprise feature"
                        arrow
                        placement="right"
                      >
                        <InfoOutlinedIcon sx={{ color: "text.secondary", cursor: "help" }} />
                      </Tooltip>
                    )}
                  </Box>
                </Grid>
                <Grid item xs={12} md={6}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <TextField
                      fullWidth
                      label="Budget Start Date"
                      name="budget_start_date"
                      type="date"
                      value={app.budget_start_date ? app.budget_start_date.split('T')[0] : ''}
                      onChange={handleBudgetStartDateChange}
                      disabled={!isEnterprise || !app.monthly_budget}
                      sx={{ opacity: isEnterprise ? 1 : 0.6 }}
                      InputLabelProps={{
                        shrink: true,
                      }}
                      helperText={isEnterprise ? "Budget cycle start date" : "Budget enforcement is an Enterprise feature"}
                    />
                    {!isEnterprise && (
                      <Tooltip
                        title="Budget enforcement is an Enterprise feature"
                        arrow
                        placement="right"
                      >
                        <InfoOutlinedIcon sx={{ color: "text.secondary", cursor: "help" }} />
                      </Tooltip>
                    )}
                  </Box>
                </Grid>
              </Grid>
            </Grid>
            <Grid item xs={12}>
              <RelationshipPicker
                label="Data sources"
                itemLabel="data source"
                value={selectedDatasources}
                onChange={handleRelationshipChange("datasource_ids")}
                options={datasources}
                getOptionLabel={jsonApiName}
              />
            </Grid>
            <Grid item xs={12}>
              {/* Only tools an App credential can reach are offered: a tool
                  with REST and MCP access both off is chat only. One that was
                  bound before it was switched off still shows as a chip (the
                  selection is resolved against the full list) and is sent
                  back unchanged, which the server accepts. */}
              <RelationshipPicker
                label="Tools"
                itemLabel="tool"
                value={selectedTools}
                onChange={handleRelationshipChange("tool_ids")}
                options={grantableTools}
                getOptionLabel={jsonApiName}
                helperText="Tools with REST API or MCP access on. Chat-only tools cannot be added to an App."
              />
            </Grid>

            {/* Dynamic Plugin Resource Type Sections: one picker per resource
                type, keyed "pluginId:slug". Selections stay as instance-id
                arrays so the plugin_resources payload is unchanged. */}
            {pluginResourceTypes.map((rt) => {
              const key = `${rt.plugin_id}:${rt.slug}`;
              // Only instances an App credential grants access to are offered.
              // An existing selection of anything else (bound before the type
              // was classified) still shows as a chip so it can be removed,
              // and is sent back unchanged, which the server accepts.
              const instances = (pluginResourceInstances[key] || []).filter(
                (inst) => inst.access_granted_via_app !== false,
              );
              const selected = itemsForIds(
                pluginResourceSelections[key],
                instances,
                (inst) => inst.id,
                (instId) => ({ id: instId, name: String(instId) }),
              );

              if (instances.length === 0 && selected.length === 0) return null;

              return (
                <Grid item xs={12} key={key}>
                  <RelationshipPicker
                    label={rt.name}
                    itemLabel={rt.name.toLowerCase()}
                    value={selected}
                    onChange={(items) => {
                      setPluginResourceSelections((prev) => ({
                        ...prev,
                        [key]: items.map((inst) => inst.id),
                      }));
                    }}
                    options={instances}
                    getOptionLabel={(inst) => inst?.name ?? String(inst?.id ?? "")}
                    getOptionSecondary={(inst) =>
                      rt.has_privacy_score && inst.privacy_score > 0
                        ? `Privacy: ${formatPrivacyLevel(inst.privacy_score)}`
                        : undefined
                    }
                  />
                </Grid>
              );
            })}
          </Grid>

          {/* Edge Availability Section */}
          <EdgeAvailabilitySection
            value={app.namespace}
            onChange={handleNamespaceChange}
            defaultExpanded={false}
          />

          {credential && (
            <StyledAccordion>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography>Credential Information</Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Grid container spacing={3}>
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      label="Key ID"
                      value={credential.attributes.key_id}
                      InputProps={{
                        readOnly: true,
                      }}
                    />
                  </Grid>
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      label="Secret"
                      value={credential.attributes.secret}
                      InputProps={{
                        readOnly: true,
                      }}
                      type="password"
                    />
                  </Grid>
                  <Grid item xs={12}>
                    <FormControlLabel
                      control={
                        <Switch
                          checked={credential.attributes.active}
                          onChange={handleCredentialActiveToggle}
                          name="active"
                          color="primary"
                        />
                      }
                      label="Credentials active (approved)"
                    />
                    {/* This switch PATCHes the credential on click, unlike
                        the rest of the form which waits for Update (UX
                        review F-03). Say so where the control is. */}
                    <Typography
                      variant="bodySmallDefault"
                      color="text.defaultSubdued"
                      component="div"
                      data-testid="credential-active-caption"
                    >
                      Applies immediately
                    </Typography>
                  </Grid>
                </Grid>
              </AccordionDetails>
            </StyledAccordion>
          )}

          {/* Metadata JSON Editor */}
          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Custom Metadata (JSON)</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Grid container spacing={2}>
                <Grid item xs={12}>
                  <Typography variant="body2" color="textSecondary" gutterBottom>
                    Add custom metadata as JSON. This data will be synced to edge instances and available to plugins.
                  </Typography>
                  <TextField
                    fullWidth
                    multiline
                    rows={8}
                    label="Metadata (JSON)"
                    value={metadataJSON}
                    onChange={(e) => {
                      setMetadataJSON(e.target.value);
                      // Validate JSON on change
                      try {
                        JSON.parse(e.target.value || "{}");
                        setMetadataError("");
                      } catch (err) {
                        setMetadataError("Invalid JSON: " + err.message);
                      }
                    }}
                    error={!!metadataError}
                    helperText={metadataError || "Example: {\"environment\": \"production\", \"region\": \"us-east-1\"}"}
                    placeholder='{"key": "value"}'
                    sx={{
                      fontFamily: 'Monaco, "Courier New", monospace',
                      "& textarea": {
                        fontFamily: 'Monaco, "Courier New", monospace',
                        fontSize: "0.875rem",
                      },
                    }}
                  />
                </Grid>
              </Grid>
            </AccordionDetails>
          </StyledAccordion>

          <Box mt={4} display="flex" gap={2}>
            <SecondaryOutlineButton onClick={handleCancel}>
              Cancel
            </SecondaryOutlineButton>
            <PrimaryButton variant="contained" type="submit">
              {id ? "Update app" : "Add app"}
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

export default AppForm;
