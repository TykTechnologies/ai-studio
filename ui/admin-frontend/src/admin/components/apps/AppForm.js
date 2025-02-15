import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
  Box,
  Typography,
  Grid,
  Snackbar,
  Alert,
  Accordion,
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
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
  StyledAccordion,
} from "../../styles/sharedStyles";

const AppForm = () => {
  const [app, setApp] = useState({
    name: "",
    description: "",
    user_id: "",
    llm_ids: [],
    datasource_ids: [],
    monthly_budget: null,
    budget_start_date: null,
  });
  const [credential, setCredential] = useState(null);
  const [users, setUsers] = useState([]);
  const [llms, setLLMs] = useState([]);
  const [datasources, setDatasources] = useState([]);
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    fetchUsers();
    fetchLLMs();
    fetchDatasources();
    if (id) {
      fetchApp();
    }
  }, [id]);

  const fetchApp = async () => {
    try {
      const response = await apiClient.get(`/apps/${id}`);
      const appData = response.data.data.attributes;
      setApp({
        ...appData,
        llm_ids: Array.isArray(appData.llm_ids)
          ? appData.llm_ids.map(String)
          : [],
        datasource_ids: Array.isArray(appData.datasource_ids)
          ? appData.datasource_ids.map(String)
          : [],
      });
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
  };

  const fetchCredential = async (credentialId) => {
    try {
      const response = await apiClient.get(`/credentials/${credentialId}`);
      setCredential(response.data.data); // Store the full data object
    } catch (error) {
      console.error("Error fetching credential", error);
    }
  };

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

  const handleChange = (e) => {
    const { name, value } = e.target;
    if (name === "monthly_budget") {
      const numValue = value === '' ? null : parseFloat(value);
      setApp(prev => ({
        ...prev,
        monthly_budget: numValue,
        budget_start_date: numValue ? new Date().toISOString() : null
      }));
    } else {
      setApp({ ...app, [name]: value });
    }
  };

  const handleMultiSelectChange = (e) => {
    const { name, value } = e.target;
    setApp({ ...app, [name]: value });
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

    const appData = {
      data: {
        type: "apps",
        attributes: {
          ...app,
          user_id: parseInt(app.user_id, 10),
          llm_ids: app.llm_ids.map((id) => parseInt(id, 10)),
          datasource_ids: app.datasource_ids.map((id) => parseInt(id, 10)),
        },
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
        <Typography variant="h5">{id ? "Edit App" : "Add App"}</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/apps"
          color="inherit"
        >
          Back to Apps
        </Button>
      </TitleBox>
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
                onChange={(e) => {
                  const value = e.target.value === '' ? null : parseFloat(e.target.value);
                  setApp(prev => ({ ...prev, monthly_budget: value }));
                }}
                InputProps={{
                  startAdornment: <InputAdornment position="start">$</InputAdornment>,
                }}
                helperText="Leave empty for no budget limit"
              />
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
          </Grid>

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

          <Box mt={4}>
            <StyledButton variant="contained" type="submit">
              {id ? "Update App" : "Add App"}
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
    </>
  );
};

export default AppForm;
