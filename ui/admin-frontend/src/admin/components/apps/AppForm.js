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
  CircularProgress,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import CustomSelectMany from "../common/CustomSelectMany"; // Assuming this is suitable

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
  });
  const [credential, setCredential] = useState(null);
  const [users, setUsers] = useState([]);
  const [llms, setLLMs] = useState([]);
  const [datasources, setDatasources] = useState([]);
  const [availableTools, setAvailableTools] = useState([]); // Added for available tools
  const [loading, setLoading] = useState(false); // Added loading state
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();

  const fetchAppData = useCallback(async () => {
    setLoading(true);
    try {
      const [usersRes, llmsRes, datasourcesRes, toolsRes] = await Promise.all([
        apiClient.get("/users", { params: { all: true } }),
        apiClient.get("/llms", { params: { all: true } }),
        apiClient.get("/datasources", { params: { all: true } }),
        appToolAPI.listAvailableTools(), // Fetch available tools
      ]);
      setUsers(usersRes.data.data || []);
      setLLMs(llmsRes.data.data || []);
      setDatasources(datasourcesRes.data.data || []);
      setAvailableTools(toolsRes.data.data || []);

      if (id) {
        const appRes = await apiClient.get(`/apps/${id}`);
        const appData = appRes.data.data.attributes;
        setApp({
          name: appData.name || "",
          description: appData.description || "",
          user_id: appData.user_id ? String(appData.user_id) : "",
          // Ensure these are arrays of strings for the Select component
          llm_ids: Array.isArray(appData.llms) ? appData.llms.map(item => String(item.id)) : [],
          datasource_ids: Array.isArray(appData.datasources) ? appData.datasources.map(item => String(item.id)) : [],
          tool_ids: Array.isArray(appData.tools) ? appData.tools.map(item => String(item.id)) : [],
          monthly_budget: appData.monthly_budget,
          budget_start_date: appData.budget_start_date,
        });
        if (appData.credential_id) {
          fetchCredential(appData.credential_id);
        }
      }
    } catch (error) {
      console.error("Error fetching initial data for app form", error);
      setSnackbar({
        open: true,
        message: "Failed to load required data for the form.",
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchAppData();
  }, [fetchAppData]);
  
  const fetchCredential = async (credentialId) => {
    try {
      const response = await apiClient.get(`/credentials/${credentialId}`);
      setCredential(response.data.data); 
    } catch (error) {
      console.error("Error fetching credential", error);
    }
  };

  const handleCredentialActiveToggle = async (event) => {
    const newActiveState = event.target.checked;
    if (!credential) return;

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
        attributes: { ...prevState.attributes, active: newActiveState },
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
        message: "Failed to update credential state.",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setApp((prevApp) => ({ ...prevApp, [name]: value }));
  };

  const handleBudgetChange = (e) => {
    const value = e.target.value === '' ? null : parseFloat(e.target.value);
    setApp(prev => ({
      ...prev,
      monthly_budget: value,
      budget_start_date: value ? prev.budget_start_date || new Date().toISOString().split('T')[0] : null
    }));
  };

  const handleBudgetStartDateChange = (e) => {
    const value = e.target.value ? new Date(e.target.value).toISOString().split('T')[0] : null;
    setApp(prev => ({ ...prev, budget_start_date: value }));
  };


  const handleMultiSelectChange = (name, selectedIds) => {
    setApp((prevApp) => ({ ...prevApp, [name]: selectedIds }));
  };

  const validateForm = () => {
    const newErrors = {};
    if (!app.name.trim()) newErrors.name = "Name is required";
    if (!app.user_id) newErrors.user_id = "User is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    // Ensure IDs are converted to integers as expected by the backend
    const appPayload = {
      ...app,
      user_id: parseInt(app.user_id, 10), // user_id is typically an integer
      llm_ids: app.llm_ids.map(id => parseInt(id, 10)),
      datasource_ids: app.datasource_ids.map(id => parseInt(id, 10)),
      tool_ids: app.tool_ids.map(id => parseInt(id, 10))
    };
    
    // Remove null budget_start_date if monthly_budget is null
    if (appPayload.monthly_budget === null) {
      appPayload.budget_start_date = null;
    }


    const appDataRequest = {
      data: {
        type: "app", // Corrected type to "app" from "apps"
        attributes: appPayload,
      },
    };

    try {
      setLoading(true);
      if (id) {
        await apiClient.patch(`/apps/${id}`, appDataRequest);
      } else {
        await apiClient.post("/apps", appDataRequest);
      }
      setSnackbar({
        open: true,
        message: id ? "App updated successfully" : "App created successfully",
        severity: "success",
      });
      setTimeout(() => navigate("/admin/apps"), 1000);
    } catch (error) {
      console.error("Error saving app", error);
      setSnackbar({
        open: true,
        message: error.response?.data?.errors?.[0]?.detail || "Failed to save app.",
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  };
  
  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") return;
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading && !id) { // Show loader only on initial load for new app form
    return <CircularProgress />;
  }


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
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Apps are used to grant developers direct access to LLMs, data sources, and tools in the AI Portal. With active credentials, an app can use the gateway API to work directly with LLMs, access the data source API to search data, or utilize configured tools.</Typography>  
      </Box>
      <ContentBox>
        {loading && id && <CircularProgress sx={{ display: 'block', margin: 'auto', mb: 2 }} />} 
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
              <FormControl fullWidth error={!!errors.user_id} required>
                <InputLabel>User</InputLabel>
                <Select name="user_id" value={app.user_id} onChange={handleChange}>
                  {users.map((user) => (
                    <MenuItem key={user.id} value={user.id.toString()}>
                      {user.attributes.name} ({user.attributes.email})
                    </MenuItem>
                  ))}
                </Select>
                {errors.user_id && <Typography color="error" sx={{ fontSize: '0.75rem', mt: 0.5, ml: 2}}>{errors.user_id}</Typography>}
              </FormControl>
            </Grid>
            
            <Grid item xs={12}>
              <CustomSelectMany
                label="LLMs"
                name="llm_ids"
                value={app.llm_ids}
                options={llms.map(llm => ({ id: llm.id.toString(), name: llm.attributes.name }))}
                onChange={(selectedIds) => handleMultiSelectChange("llm_ids", selectedIds)}
              />
            </Grid>

            <Grid item xs={12}>
               <CustomSelectMany
                label="Datasources"
                name="datasource_ids"
                value={app.datasource_ids}
                options={datasources.map(ds => ({ id: ds.id.toString(), name: ds.attributes.name }))}
                onChange={(selectedIds) => handleMultiSelectChange("datasource_ids", selectedIds)}
              />
            </Grid>

            <Grid item xs={12}>
               <CustomSelectMany
                label="Tools"
                name="tool_ids"
                value={app.tool_ids}
                options={availableTools.map(tool => ({ id: tool.id.toString(), name: tool.attributes.name }))}
                onChange={(selectedIds) => handleMultiSelectChange("tool_ids", selectedIds)}
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
                    inputProps={{ step: "0.01", min: "0" }}
                    value={app.monthly_budget === null ? '' : app.monthly_budget}
                    onChange={handleBudgetChange}
                    InputProps={{ startAdornment: <InputAdornment position="start">$</InputAdornment> }}
                    helperText="Leave empty for no budget limit"
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <TextField
                    fullWidth
                    label="Budget Start Date"
                    name="budget_start_date"
                    type="date"
                    value={app.budget_start_date ? app.budget_start_date.split('T')[0] : ''}
                    onChange={handleBudgetStartDateChange}
                    disabled={app.monthly_budget === null}
                    InputLabelProps={{ shrink: true }}
                    helperText="Budget cycle start date. Required if budget is set."
                  />
                </Grid>
              </Grid>
            </Grid>
          </Grid>

          {credential && (
            <StyledAccordion sx={{ mt: 3 }}>
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
                      InputProps={{ readOnly: true }}
                    />
                  </Grid>
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      label="Secret"
                      value="********************************" // Masked
                      InputProps={{ readOnly: true }}
                      helperText="Secret is only shown on creation. If lost, a new credential must be generated."
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
                      label="Credential Active"
                    />
                  </Grid>
                </Grid>
              </AccordionDetails>
            </StyledAccordion>
          )}

          <Box mt={4}>
            <PrimaryButton variant="contained" type="submit" disabled={loading}>
              {loading ? <CircularProgress size={24} /> : (id ? "Update app" : "Add app")}
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
        <Alert onClose={handleCloseSnackbar} severity={snackbar.severity} sx={{ width: "100%" }}>
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default AppForm;
