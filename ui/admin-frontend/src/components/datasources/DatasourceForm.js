import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
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
  Tooltip,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Chip,
  Paper,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import Visibility from "@mui/icons-material/Visibility";
import VisibilityOff from "@mui/icons-material/VisibilityOff";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import InfoIcon from "@mui/icons-material/Info";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import {
  getVendorData,
  getVectorStoreHelpText,
  getEmbedderHelpText,
  fetchVendors,
} from "../../utils/vendorUtils";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const DatasourceForm = () => {
  const [datasource, setDatasource] = useState({
    name: "",
    short_description: "",
    long_description: "",
    db_source_type: "",
    embed_vendor: "",
    privacy_score: 0,
    db_conn_string: "",
    db_conn_api_key: "",
    embed_api_key: "",
    embed_url: "",
    embed_model: "",
    icon: "",
    url: "",
    active: false,
    tags: [],
    db_name: "",
    user_id: "", // Add this line
  });

  const [users, setUsers] = useState([]);
  const [vectorStores, setVectorStores] = useState([]);
  const [embedders, setEmbedders] = useState([]);
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [showDbConnApiKey, setShowDbConnApiKey] = useState(false);
  const [showEmbedApiKey, setShowEmbedApiKey] = useState(false);
  const [newTag, setNewTag] = useState("");
  const navigate = useNavigate();
  const { id } = useParams();
  const [vectorStoreHelpText, setVectorStoreHelpText] = useState("");
  const [embedderHelpText, setEmbedderHelpText] = useState("");

  useEffect(() => {
    const loadVendors = async () => {
      const { embedders, vectorStores } = await fetchVendors();
      setVectorStores(vectorStores.map((vs) => vs.code));
      setEmbedders(embedders.map((e) => e.code));
    };
    loadVendors();
    fetchUsers();

    // Fetch datasource data if in edit mode
    if (id) {
      const fetchDatasource = async () => {
        try {
          const response = await apiClient.get(`/datasources/${id}`);
          const datasourceData = response.data.data.attributes;
          setDatasource({
            ...datasourceData,
            tags: datasourceData.tags
              ? datasourceData.tags.map((tag) => tag.attributes.name)
              : [],
            user_id: datasourceData.user_id.toString(), // Convert user_id to string
          });
          setVectorStoreHelpText(
            getVectorStoreHelpText(datasourceData.db_source_type),
          );
          setEmbedderHelpText(getEmbedderHelpText(datasourceData.embed_vendor));
        } catch (error) {
          console.error("Error fetching datasource:", error);
          setSnackbar({
            open: true,
            message: "Failed to fetch datasource data. Please try again.",
            severity: "error",
          });
        }
      };
      fetchDatasource();
    }
  }, [id]);

  const fetchUsers = async () => {
    try {
      const response = await apiClient.get("/users");
      setUsers(response.data.data || []);
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    if (name === "privacy_score") {
      const numValue = Math.min(Math.max(parseInt(value) || 0, 0), 100);
      setDatasource((prev) => ({ ...prev, [name]: numValue }));
    } else {
      setDatasource((prev) => ({ ...prev, [name]: value }));
    }

    if (name === "db_source_type") {
      setVectorStoreHelpText(getVectorStoreHelpText(value));
    } else if (name === "embed_vendor") {
      setEmbedderHelpText(getEmbedderHelpText(value));
    }
  };

  const handleSwitchChange = (e) => {
    setDatasource((prev) => ({ ...prev, active: e.target.checked }));
  };

  const handleAddTag = () => {
    if (newTag && !datasource.tags.includes(newTag)) {
      setDatasource({ ...datasource, tags: [...datasource.tags, newTag] });
      setNewTag("");
    }
  };

  const handleDeleteTag = (tagToDelete) => {
    setDatasource({
      ...datasource,
      tags: datasource.tags.filter((tag) => tag !== tagToDelete),
    });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!datasource.name.trim()) newErrors.name = "Name is required";
    if (!datasource.db_source_type.trim())
      newErrors.db_source_type = "Vector Database Type is required";
    if (!datasource.embed_vendor.trim())
      newErrors.embed_vendor = "Embedding Service Vendor is required";
    if (datasource.privacy_score < 0 || datasource.privacy_score > 100)
      newErrors.privacy_score = "Privacy score must be between 0 and 100";
    if (!datasource.user_id) newErrors.user_id = "User is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const datasourceData = {
      data: {
        type: "datasources",
        ...(id && { id }), // Include id for PATCH requests
        attributes: {
          ...datasource,
          privacy_score: Number(datasource.privacy_score),
          active: Boolean(datasource.active),
          tags: datasource.tags,
          user_id: parseInt(datasource.user_id, 10),
        },
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/datasources/${id}`, datasourceData);
      } else {
        await apiClient.post("/datasources", datasourceData);
      }

      setSnackbar({
        open: true,
        message: id
          ? "Datasource updated successfully"
          : "Datasource created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/datasources"), 2000);
    } catch (error) {
      console.error("Error saving datasource", error);
      setSnackbar({
        open: true,
        message: "Failed to save datasource. Please try again.",
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

  console.log("vectorStores:", vectorStores);
  console.log("datasource:", datasource);

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">
          {id ? "Edit Datasource" : "Add Datasource"}
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/datasources"
          color="white"
        >
          Back to Datasources
        </Button>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <SectionTitle>Basic Information</SectionTitle>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                name="name"
                value={datasource.name}
                onChange={handleChange}
                error={!!errors.name}
                helperText={errors.name}
                required
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Short Description"
                name="short_description"
                value={datasource.short_description}
                onChange={handleChange}
                multiline
                rows={2}
              />
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth error={!!errors.user_id}>
                <InputLabel>User</InputLabel>
                <Select
                  name="user_id"
                  value={datasource.user_id}
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
              <FormControl fullWidth required error={!!errors.db_source_type}>
                <InputLabel>Vector Database Type</InputLabel>
                <Select
                  name="db_source_type"
                  value={datasource.db_source_type || ""}
                  onChange={handleChange}
                >
                  {vectorStores.map((code) => {
                    const vendorData = getVendorData(code, "vectorStore");
                    return (
                      <MenuItem key={code} value={code}>
                        <Box sx={{ display: "flex", alignItems: "center" }}>
                          <img
                            src={vendorData.logo}
                            alt={vendorData.name}
                            style={{
                              width: 24,
                              height: 24,
                              marginRight: 8,
                              objectFit: "contain",
                            }}
                          />
                          {vendorData.name}
                        </Box>
                      </MenuItem>
                    );
                  })}
                </Select>
              </FormControl>
              {vectorStoreHelpText && (
                <Paper
                  elevation={0}
                  sx={{
                    mt: 1,
                    p: 1,
                    bgcolor: "info.light",
                    color: "info.contrastText",
                    display: "flex",
                    alignItems: "center",
                  }}
                >
                  <InfoIcon sx={{ mr: 1 }} />
                  <Typography variant="body2">{vectorStoreHelpText}</Typography>
                </Paper>
              )}
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth required error={!!errors.embed_vendor}>
                <InputLabel>Embedding Service Vendor</InputLabel>
                <Select
                  name="embed_vendor"
                  value={datasource.embed_vendor}
                  onChange={handleChange}
                >
                  {embedders.map((code) => {
                    const vendorData = getVendorData(code, "embedder");
                    return (
                      <MenuItem key={code} value={code}>
                        <Box sx={{ display: "flex", alignItems: "center" }}>
                          <img
                            src={vendorData.logo}
                            alt={vendorData.name}
                            style={{
                              width: 24,
                              height: 24,
                              marginRight: 8,
                              objectFit: "contain",
                            }}
                          />
                          {vendorData.name}
                        </Box>
                      </MenuItem>
                    );
                  })}
                </Select>
              </FormControl>
              {embedderHelpText && (
                <Paper
                  elevation={0}
                  sx={{
                    mt: 1,
                    p: 1,
                    bgcolor: "info.light",
                    color: "info.contrastText",
                    display: "flex",
                    alignItems: "center",
                  }}
                >
                  <InfoIcon sx={{ mr: 1 }} />
                  <Typography variant="body2">{embedderHelpText}</Typography>
                </Paper>
              )}
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Privacy Score"
                name="privacy_score"
                type="number"
                value={datasource.privacy_score}
                onChange={handleChange}
                error={!!errors.privacy_score}
                helperText={errors.privacy_score}
                inputProps={{
                  min: 0,
                  max: 100,
                  step: 1,
                }}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <Tooltip title="Privacy score must be between 0 and 100, where 0 is the lowest and 100 is the highest.">
                        <HelpOutlineIcon color="action" />
                      </Tooltip>
                    </InputAdornment>
                  ),
                }}
              />
            </Grid>
            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={datasource.active}
                    onChange={handleSwitchChange}
                    name="active"
                    color="primary"
                  />
                }
                label="Active"
              />
            </Grid>
          </Grid>

          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Vector Database Access Details</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Grid container spacing={3}>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Database / Namespace Name"
                    name="db_name"
                    value={datasource.db_name}
                    onChange={handleChange}
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Connection String"
                    name="db_conn_string"
                    value={datasource.db_conn_string}
                    onChange={handleChange}
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="API Key"
                    name="db_conn_api_key"
                    type={showDbConnApiKey ? "text" : "password"}
                    value={datasource.db_conn_api_key}
                    onChange={handleChange}
                    InputProps={{
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            onClick={() =>
                              setShowDbConnApiKey(!showDbConnApiKey)
                            }
                            edge="end"
                          >
                            {showDbConnApiKey ? (
                              <VisibilityOff />
                            ) : (
                              <Visibility />
                            )}
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
              <Typography>Embedding Service Details</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Grid container spacing={3}>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Model"
                    name="embed_model"
                    value={datasource.embed_model}
                    onChange={handleChange}
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Service URL"
                    name="embed_url"
                    value={datasource.embed_url}
                    onChange={handleChange}
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="API Key"
                    name="embed_api_key"
                    type={showEmbedApiKey ? "text" : "password"}
                    value={datasource.embed_api_key}
                    onChange={handleChange}
                    InputProps={{
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            onClick={() => setShowEmbedApiKey(!showEmbedApiKey)}
                            edge="end"
                          >
                            {showEmbedApiKey ? (
                              <VisibilityOff />
                            ) : (
                              <Visibility />
                            )}
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
              <Typography>Additional Information</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Grid container spacing={3}>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Long Description"
                    name="long_description"
                    value={datasource.long_description}
                    onChange={handleChange}
                    multiline
                    rows={4}
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Icon URL"
                    name="icon"
                    value={datasource.icon}
                    onChange={handleChange}
                  />
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    label="Add Tag"
                    value={newTag}
                    onChange={(e) => setNewTag(e.target.value)}
                    onKeyPress={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        handleAddTag();
                      }
                    }}
                    InputProps={{
                      endAdornment: (
                        <InputAdornment position="end">
                          <Button onClick={handleAddTag}>Add</Button>
                        </InputAdornment>
                      ),
                    }}
                  />
                </Grid>
                <Grid item xs={12}>
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                    {datasource.tags.map((tag, index) => (
                      <Chip
                        key={index}
                        label={tag}
                        onDelete={() => handleDeleteTag(tag)}
                      />
                    ))}
                  </Box>
                </Grid>
              </Grid>
            </AccordionDetails>
          </StyledAccordion>

          <Box mt={4}>
            <StyledButton variant="contained" type="submit">
              {id ? "Update Datasource" : "Add Datasource"}
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
    </StyledPaper>
  );
};

export default DatasourceForm;
