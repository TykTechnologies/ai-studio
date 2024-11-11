import React, { useState, useEffect, useRef } from "react";
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
import EditIcon from "@mui/icons-material/Edit";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import DeleteIcon from "@mui/icons-material/Delete";
import CloudUploadIcon from "@mui/icons-material/CloudUpload";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import { styled } from "@mui/system";

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
      />
    </Paper>
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

const ToolForm = () => {
  const [tool, setTool] = useState({
    name: "",
    description: "",
    privacy_score: 0,
    auth_schema_name: "",
    auth_key: "",
    oas_spec: "",
    operations: "",
  });
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [oasSpecError, setOasSpecError] = useState(null);
  const [files, setFiles] = useState([]);
  const navigate = useNavigate();
  const { id } = useParams();
  const fileInputRef = useRef(null);

  useEffect(() => {
    if (id) {
      fetchTool();
      fetchToolOperations();
    }
  }, [id]);

  const fetchTool = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}`);
      const fetchedTool = response.data.data.attributes;

      fetchedTool.oas_spec = fetchedTool.oas_spec
        ? atob(fetchedTool.oas_spec)
        : "";

      setTool(fetchedTool);
      setFiles(fetchedTool.file_stores || []);
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

  const validateForm = () => {
    const newErrors = {};
    if (!tool.name.trim()) newErrors.name = "Name is required";
    if (!tool.description.trim())
      newErrors.description = "Description is required";
    if (tool.privacy_score < 0 || tool.privacy_score > 100)
      newErrors.privacy_score = "Privacy score must be between 0 and 100";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const toolData = {
      data: {
        type: "Tool",
        attributes: {
          ...tool,
          privacy_score: Number(tool.privacy_score),
          tool_type: "REST",
          oas_spec: tool.oas_spec ? btoa(tool.oas_spec) : "",
        },
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/tools/${id}`, toolData);
        await updateToolOperations();
      } else {
        const response = await apiClient.post("/tools", toolData);
        const newToolId = response.data.data.id;
        await updateToolOperations(newToolId);
      }

      setSnackbar({
        open: true,
        message: id ? "Tool updated successfully" : "Tool created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/tools"), 2000);
    } catch (error) {
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
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">{id ? "Edit Tool" : "Add Tool"}</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/tools"
          color="white"
        >
          Back to Tools
        </Button>
      </TitleBox>
      <ContentBox>
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
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Privacy Score"
                name="privacy_score"
                type="number"
                value={tool.privacy_score}
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
                      <Tooltip title="Privacy score is a value between 0 and 100, where 0 is the lowest and 100 is the highest. This determines the privacy level of the tool.">
                        <HelpOutlineIcon color="action" />
                      </Tooltip>
                    </InputAdornment>
                  ),
                }}
              />
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
              />
            </Grid>
          </Grid>

          <SectionTitle tooltip="Define the operations (endpoints) that this tool can use. These should correspond to paths in your OpenAPI Specification.">
            Operations
          </SectionTitle>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <OperationsInput
                value={tool.operations}
                onChange={handleOperationsChange}
              />
              <Typography variant="caption" color="textSecondary">
                Type an operation name and press comma or enter to add. Click on
                a chip to remove it.
              </Typography>
            </Grid>
          </Grid>

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

              <Button
                variant="contained"
                startIcon={<CloudUploadIcon />}
                onClick={() => fileInputRef.current.click()}
                sx={{ mt: 2 }}
              >
                Upload Additional Tool Documentation
              </Button>
            </AccordionDetails>
          </StyledAccordion>

          <Box mt={4}>
            <StyledButton variant="contained" type="submit">
              {id ? "Update Tool" : "Add Tool"}
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

export default ToolForm;
