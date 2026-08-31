import React, { useState, useEffect, useRef } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
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
  Tooltip,
  AccordionSummary,
  AccordionDetails,
  Chip,
  Paper,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import Visibility from "@mui/icons-material/Visibility";
import VisibilityOff from "@mui/icons-material/VisibilityOff";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import InfoIcon from "@mui/icons-material/Info";
import DeleteIcon from "@mui/icons-material/Delete";
import CloudUploadIcon from "@mui/icons-material/CloudUpload";
import AutorenewIcon from "@mui/icons-material/Autorenew";

import {
  PrimaryOutlineButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledAccordion,
  SecondaryLinkButton
} from "../../styles/sharedStyles";
import {
  getVendorData,
  getVectorStoreHelpText,
  getEmbedderHelpText,
  getEmbedderDefaultModel,
  getEmbedderDefaultUrl,
  fetchVendors,
} from "../../utils/vendorUtils";
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";

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
    user_id: "",
    namespace: "",
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
  const [files, setFiles] = useState([]);
  const navigate = useNavigate();
  const { id } = useParams();
  const [vectorStoreHelpText, setVectorStoreHelpText] = useState("");
  const [embedderHelpText, setEmbedderHelpText] = useState("");
  const fileInputRef = useRef(null);

  useEffect(() => {
    const loadVendors = async () => {
      const { embedders, vectorStores } = await fetchVendors();
      setVectorStores(vectorStores.map((vs) => vs.code));
      setEmbedders(embedders.map((e) => e.code));
    };
    loadVendors();
    fetchUsers();

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
            user_id: datasourceData.user_id.toString(),
          });
          setFiles(datasourceData.files || []);
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

  const handleFileUpload = async (event) => {
    const file = event.target.files[0];
    if (!file) return;

    try {
      const formData = new FormData();
      formData.append("file", file);
      formData.append(
        "description",
        `Documentation for datasource: ${datasource.name}`,
      );

      const fileStoreResponse = await apiClient.post("/filestore", formData, {
        headers: {
          "Content-Type": "multipart/form-data",
        },
      });

      const fileStoreId = fileStoreResponse.data.data.id;

      await apiClient.post(`/datasources/${id}/filestores/${fileStoreId}`);

      const updatedDatasourceResponse = await apiClient.get(
        `/datasources/${id}`,
      );
      setFiles(updatedDatasourceResponse.data.data.attributes.files || []);

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
      await apiClient.delete(`/datasources/${id}/filestores/${fileStoreId}`);

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

  const fetchUsers = async () => {
    try {
      const response = await apiClient.get("/users");
      setUsers(response.data.data || []);
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const handleStartProcessing = async () => {
    try {
      await apiClient.post(`/datasources/${id}/process-embeddings`);
      setSnackbar({
        open: true,
        message: "File processing started. This may take a few minutes.",
        severity: "info",
      });
    } catch (error) {
      console.error("Error starting file processing:", error);
      setSnackbar({
        open: true,
        message: "Failed to start file processing. Please try again.",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    if (name === "privacy_score") {
      const numValue = Math.min(Math.max(parseInt(value) || 0, 0), 100);
      setDatasource((prev) => ({ ...prev, [name]: numValue }));
    } else if (name === "embed_vendor") {
      const defaultModel = getEmbedderDefaultModel(value);
      const defaultUrl = getEmbedderDefaultUrl(value);
      const previousDefaultModel = getEmbedderDefaultModel(datasource.embed_vendor);
      const previousDefaultUrl = getEmbedderDefaultUrl(datasource.embed_vendor);
      setDatasource((prev) => ({
        ...prev,
        embed_vendor: value,
        // Update if: empty, or still matches the previous vendor's default
        embed_model:
          !prev.embed_model || prev.embed_model === previousDefaultModel
            ? defaultModel
            : prev.embed_model,
        embed_url:
          !prev.embed_url || prev.embed_url === previousDefaultUrl
            ? defaultUrl
            : prev.embed_url,
      }));
      setEmbedderHelpText(getEmbedderHelpText(value));
    } else {
      setDatasource((prev) => ({ ...prev, [name]: value }));
    }

    if (name === "db_source_type") {
      setVectorStoreHelpText(getVectorStoreHelpText(value));
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

  const handleNamespaceChange = (namespaces) => {
    const namespaceString = Array.isArray(namespaces) ? namespaces.join(', ') : namespaces;
    setDatasource({ ...datasource, namespace: namespaceString });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!datasource.name.trim()) newErrors.name = "Name is required";
    if (!datasource.db_source_type.trim())
      newErrors.db_source_type = "Vector Database Type is required";
    if (!datasource.embed_vendor.trim())
      newErrors.embed_vendor = "Embedding Service Vendor is required";
    if (datasource.privacy_score < 0 || datasource.privacy_score > 100)
      newErrors.privacy_score = "Privacy level must be between 0 and 100";
    if (!datasource.user_id) newErrors.user_id = "User is required";
    setErrors(newErrors);
    return newErrors;
  };

  // Fields in the order they appear, so a failed submit scrolls to the first
  // thing the user actually needs to fix. Without this the submit button sits
  // below the fold and a validation failure looks like a button that does
  // nothing at all.
  const FIELD_ORDER = [
    "name",
    "user_id",
    "db_source_type",
    "embed_vendor",
    "privacy_score",
  ];

  const focusFirstError = (fieldErrors) => {
    const firstField = FIELD_ORDER.find((field) => fieldErrors[field]);
    if (!firstField) return;
    document
      .getElementById(`datasource-field-${firstField}`)
      ?.scrollIntoView({ behavior: "smooth", block: "center" });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    const fieldErrors = validateForm();
    if (Object.keys(fieldErrors).length > 0) {
      focusFirstError(fieldErrors);
      return;
    }

    const datasourceData = {
      data: {
        type: "datasources",
        ...(id && { id }),
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
          : "Data source created and added to the Default data catalog. It is inactive until you activate it.",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/datasources"), 2000);
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

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          {id ? "Edit data source" : "Add data source"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/datasources"
          color="inherit"
        >
          Back to data sources
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Data sources let you store and access information to enhance AI conversations using Retrieval Augmented Generation (RAG). By using embedding providers to convert content into searchable vectors, your AI can deliver more accurate, informed, and engaging responses.</Typography>  
      </Box> 
      <ContentBox sx={{ pt: 0 }} >
        <Box component="form" onSubmit={handleSubmit}>
          <SectionTitle>Basic Information</SectionTitle>
          <Grid container spacing={3}>
            {!id && (
              <Grid item xs={12}>
                {/* ensureDatasourceInDefaultCatalogue adds any datasource that
                    ends up in no catalogue to Default, which every user on the
                    instance can see. Nothing on this form said so, so creating
                    a sensitive data source put it on everyone's menu at that
                    instant. */}
                <Alert severity="info">
                  A data source that is not assigned to a catalog is added to
                  the <strong>Default</strong> data catalog, which every user on
                  this instance can see. It stays <strong>inactive</strong>
                  until you activate it, so being listed is not the same as
                  being usable. Assign it to a specific catalog if it should not
                  be in Default.
                </Alert>
              </Grid>
            )}
            <Grid item xs={12}>
              <TextField
                id="datasource-field-name"
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
              {/* `required` belongs on the FormControl: on the Select it
                  validates but renders no asterisk, so this field read as
                  optional while silently blocking every submit. */}
              <FormControl
                id="datasource-field-user_id"
                fullWidth
                required
                error={!!errors.user_id}
              >
                <InputLabel id="datasourceform-user-label">User</InputLabel>
                <Select
                  labelId="datasourceform-user-label"
                  name="user_id"
                  value={datasource.user_id}
                  onChange={handleChange}
                >
                  {users.map((user) => (
                    <MenuItem key={user.id} value={user.id}>
                      {user.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
                {errors.user_id && (
                  <FormHelperText>{errors.user_id}</FormHelperText>
                )}
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <FormControl
                id="datasource-field-db_source_type"
                fullWidth
                required
                error={!!errors.db_source_type}
              >
                <InputLabel id="datasourceform-vector-database-type-label">
                  Vector Database Type
                </InputLabel>
                <Select
                  labelId="datasourceform-vector-database-type-label"
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
                {errors.db_source_type && (
                  <FormHelperText>{errors.db_source_type}</FormHelperText>
                )}
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
              <FormControl
                id="datasource-field-embed_vendor"
                fullWidth
                required
                error={!!errors.embed_vendor}
              >
                <InputLabel id="datasourceform-embedding-service-vendor-label">
                  Embedding Service Vendor
                </InputLabel>
                <Select
                  labelId="datasourceform-embedding-service-vendor-label"
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
                {errors.embed_vendor && (
                  <FormHelperText>{errors.embed_vendor}</FormHelperText>
                )}
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
              <Typography variant="subtitle2" gutterBottom>
                Privacy levels
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Privacy levels define how data is protected by controlling LLM access based on its sensitivity. LLMs providers with lower privacy levels can’t access higher-level, data sources and tools, ensuring secure and appropriate data handling. Set a privacy level (0 lowest - 100 highest).
              </Typography>
              <TextField
                fullWidth
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

          {id && (
            <StyledAccordion>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography>File Quick Upload</Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Typography variant="body2" color="text.secondary" paragraph>
                  Upload documentation to be processed for this datasource.
                  These files will be processed, chunked, and embedded into your
                  chosen data store.
                </Typography>

                <List>
                  {files.map((file) => (
                    <ListItem key={file.id}>
                      <ListItemText
                        primary={file.attributes.file_name}
                        secondary={
                          <>
                            {`Size: ${file.attributes.length} bytes`}
                            <br />
                            {file.attributes.last_processed_on &&
                            file.attributes.last_processed_on !==
                              "0001-01-01T00:00:00Z"
                              ? `Last processed: ${new Date(
                                  file.attributes.last_processed_on,
                                ).toLocaleString()}`
                              : "Not processed"}
                          </>
                        }
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

                <Box sx={{ display: "flex", gap: 2, mt: 2 }}>
                  <PrimaryOutlineButton
                    variant="contained"
                    startIcon={<CloudUploadIcon />}
                    onClick={() => fileInputRef.current.click()}
                  >
                    Upload Additional Datasource Documentation
                  </PrimaryOutlineButton>

                  <Button
                    variant="contained"
                    color="secondary"
                    onClick={handleStartProcessing}
                    disabled={files.length === 0}
                    startIcon={<AutorenewIcon />}
                  >
                    Start Processing
                  </Button>
                </Box>
              </AccordionDetails>
            </StyledAccordion>
          )}

          {/* Edge Availability Section (Enterprise only) */}
          <EdgeAvailabilitySection
            value={datasource.namespace}
            onChange={handleNamespaceChange}
            defaultExpanded={false}
          />

          <Box mt={4}>
            <PrimaryButton variant="contained" type="submit">
              {id ? "Update data source" : "Add data source"}
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

export default DatasourceForm;
