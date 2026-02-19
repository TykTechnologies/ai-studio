import React, { useState, useEffect, useCallback } from "react";
import apiClient, { appToolAPI } from "../../utils/apiClient"; // Import appToolAPI
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
  Chip,
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
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";
import { useEdition } from "../../context/EditionContext";

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
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();
  const { isEnterprise } = useEdition(); // Get edition info

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
      const response = await apiClient.get("/users");
      setUsers(response.data.data || []);
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const fetchLLMs = async () => {
    try {
      const response = await apiClient.get("/llms");
      setLLMs(response.data.data || []);
    } catch (error) {
      console.error("Error fetching LLMs", error);
    }
  };

  const fetchDatasources = async () => {
    try {
      const response = await apiClient.get("/datasources");
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

  const handleMultiSelectChange = (e) => {
    const { name, value } = e.target;
    setApp({ ...app, [name]: value });
  };

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

      setSnackbar({
        open: true,
        message: id ? "App updated successfully" : "App created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/apps"), 2000);
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
                <InputLabel>User</InputLabel>
                <Select
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
              <FormControl fullWidth>
                <InputLabel>LLMs</InputLabel>
                <Select
                  multiple
                  name="llm_ids"
                  value={app.llm_ids}
                  onChange={handleMultiSelectChange}
                  renderValue={(selected) => (
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                      {selected.map((value) => {
                        const llm = llms.find((l) => l.id === value);
                        return (
                          <Chip
                            key={value}
                            label={llm ? llm.attributes.name : value}
                          />
                        );
                      })}
                    </Box>
                  )}
                >
                  {llms.map((llm) => (
                    <MenuItem key={llm.id} value={llm.id.toString()}>
                      {llm.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
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
              <FormControl fullWidth>
                <InputLabel>Datasources</InputLabel>
                <Select
                  multiple
                  name="datasource_ids"
                  value={app.datasource_ids}
                  onChange={handleMultiSelectChange}
                  renderValue={(selected) => (
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                      {selected.map((value) => {
                        const datasource = datasources.find(
                          (ds) => ds.id === value,
                        );
                        return (
                          <Chip
                            key={value}
                            label={
                              datasource ? datasource.attributes.name : value
                            }
                          />
                        );
                      })}
                    </Box>
                  )}
                >
                  {datasources.map((datasource) => (
                    <MenuItem
                      key={datasource.id}
                      value={datasource.id.toString()}
                    >
                      {datasource.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth>
                <InputLabel>Tools</InputLabel>
                <Select
                  multiple
                  name="tool_ids"
                  value={app.tool_ids}
                  onChange={handleMultiSelectChange}
                  renderValue={(selected) => (
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                      {selected.map((value) => {
                        const tool = availableTools.find(
                          (t) => t.id.toString() === value,
                        );
                        return (
                          <Chip
                            key={value}
                            label={
                              tool ? tool.attributes.name : value
                            }
                          />
                        );
                      })}
                    </Box>
                  )}
                >
                  {availableTools.map((tool) => (
                    <MenuItem
                      key={tool.id}
                      value={tool.id.toString()}
                    >
                      {tool.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>

            {/* Dynamic Plugin Resource Type Sections */}
            {pluginResourceTypes.map((rt) => {
              const key = `${rt.plugin_id}:${rt.slug}`;
              const instances = pluginResourceInstances[key] || [];
              const selected = pluginResourceSelections[key] || [];

              return (
                <Grid item xs={12} key={key}>
                  <FormControl fullWidth>
                    <InputLabel>{rt.name}</InputLabel>
                    <Select
                      multiple
                      value={selected}
                      onChange={(e) => {
                        setPluginResourceSelections((prev) => ({
                          ...prev,
                          [key]: e.target.value,
                        }));
                      }}
                      renderValue={(sel) => (
                        <Box
                          sx={{
                            display: "flex",
                            flexWrap: "wrap",
                            gap: 0.5,
                          }}
                        >
                          {sel.map((val) => {
                            const inst = instances.find(
                              (i) => i.id === val,
                            );
                            return (
                              <Chip
                                key={val}
                                label={inst ? inst.name : val}
                              />
                            );
                          })}
                        </Box>
                      )}
                    >
                      {instances.map((inst) => (
                        <MenuItem key={inst.id} value={inst.id}>
                          {inst.name}
                          {rt.has_privacy_score &&
                            inst.privacy_score > 0 &&
                            ` (Privacy: ${inst.privacy_score})`}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
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
                      label="Active"
                    />
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

          <Box mt={4}>
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
