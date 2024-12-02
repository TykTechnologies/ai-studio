import React, { useState, useEffect, useRef } from "react";
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
  Chip,
  CircularProgress,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from "@mui/material";
import DeleteIcon from "@mui/icons-material/Delete";
import CloudUploadIcon from "@mui/icons-material/CloudUpload";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";

import { FormControlLabel, Switch } from "@mui/material";

import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
  StyledAccordion,
} from "../../styles/sharedStyles";

const ChatForm = () => {
  const [chat, setChat] = useState({
    name: "",
    llm_settings_id: "",
    llm_id: "",
    groups: [],
    filters: [],
    rag_n: "",
    tool_support: false,
    system_prompt: "",
    default_data_source_id: 0,
    default_tool_ids: [],
  });
  const [files, setFiles] = useState([]);
  const fileInputRef = useRef(null);
  const [llms, setLLMs] = useState([]);
  const [llmSettings, setLLMSettings] = useState([]);
  const [allGroups, setAllGroups] = useState([]);
  const [allFilters, setAllFilters] = useState([]);
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [loading, setLoading] = useState(true);
  const [apiError, setApiError] = useState(null);
  const [datasources, setDatasources] = useState([]);
  const [allTools, setAllTools] = useState([]);

  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      setApiError(null);
      try {
        await Promise.all([
          fetchFilters(),
          fetchLLMs(),
          fetchLLMSettings(),
          fetchGroups(),
          fetchDatasources(),
          fetchTools(), // Add this line
          id
            ? Promise.all([fetchChat(), fetchExtraContext()])
            : Promise.resolve(),
        ]);
      } catch (error) {
        console.error("Error fetching data", error);
        setApiError("An error occurred while fetching data. Please try again.");
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [id]);

  const fetchTools = async () => {
    try {
      const response = await apiClient.get("/tools");
      setAllTools(response.data.data || []);
    } catch (error) {
      console.error("Error fetching tools", error);
      throw error;
    }
  };

  const fetchExtraContext = async () => {
    if (!id) return;
    try {
      const response = await apiClient.get(`/chats/${id}/extra-context`);
      setFiles(response.data.data || []);
    } catch (error) {
      console.error("Error fetching extra context files", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch extra context files",
        severity: "error",
      });
    }
  };

  const handleFileUpload = async (event) => {
    const file = event.target.files[0];
    if (!file) return;

    try {
      const formData = new FormData();
      formData.append("file", file);
      formData.append("description", `Extra context for chat: ${chat.name}`);

      const fileStoreResponse = await apiClient.post("/filestore", formData, {
        headers: {
          "Content-Type": "multipart/form-data",
        },
      });

      const fileStoreId = fileStoreResponse.data.data.id;
      await apiClient.post(`/chats/${id}/extra-context/${fileStoreId}`);
      await fetchExtraContext();

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
      await apiClient.delete(`/chats/${id}/extra-context/${fileStoreId}`);
      await fetchExtraContext();

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

  const fetchDatasources = async () => {
    try {
      const response = await apiClient.get("/datasources");
      setDatasources(response.data.data || []);
    } catch (error) {
      console.error("Error fetching datasources", error);
      throw error;
    }
  };

  const fetchChat = async () => {
    try {
      const response = await apiClient.get(`/chats/${id}`);
      const chatData = response.data.data.attributes;
      setChat({
        name: chatData.name,
        llm_settings_id: chatData.llm_settings_id,
        llm_id: chatData.llm_id,
        groups: chatData.groups.map((group) => group.id.toString()),
        filters: chatData.filters.map((filter) => filter.id.toString()),
        rag_n: chatData.rag_n || "",
        tool_support: chatData.tool_support || false,
        system_prompt: chatData.system_prompt || "",
        default_data_source_id:
          chatData.default_data_source_id?.toString() || "",
        default_tool_ids: Array.isArray(chatData.default_tools)
          ? chatData.default_tools.map((tool) => tool.id.toString())
          : [],
      });
    } catch (error) {
      console.error("Error fetching chat", error);
      throw error;
    }
  };

  const handleToolChange = (event) => {
    setChat({ ...chat, default_tool_ids: event.target.value });
  };

  const fetchLLMs = async () => {
    try {
      const response = await apiClient.get("/llms");
      setLLMs(response.data.data);
    } catch (error) {
      console.error("Error fetching LLMs", error);
      throw error;
    }
  };

  const fetchLLMSettings = async () => {
    try {
      const response = await apiClient.get("/llm-settings");
      setLLMSettings(response.data.data);
    } catch (error) {
      console.error("Error fetching LLM settings", error);
      throw error;
    }
  };

  const fetchGroups = async () => {
    try {
      const response = await apiClient.get("/groups");
      setAllGroups(response.data.data);
    } catch (error) {
      console.error("Error fetching groups", error);
      throw error;
    }
  };

  const fetchFilters = async () => {
    try {
      const response = await apiClient.get("/filters");
      setAllFilters(response.data || []);
      console.log("Fetched filters:", response.data);
    } catch (error) {
      console.error("Error fetching filters", error);
      throw error;
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setChat({ ...chat, [name]: value });
  };

  const handleGroupChange = (event) => {
    setChat({ ...chat, groups: event.target.value });
  };

  const handleFilterChange = (event) => {
    setChat({ ...chat, filters: event.target.value });
  };

  const handleSwitchChange = (event) => {
    setChat({ ...chat, tool_support: event.target.checked });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!chat.name.trim()) newErrors.name = "Name is required";
    if (!chat.llm_settings_id)
      newErrors.llm_settings_id = "LLM Settings is required";
    if (!chat.llm_id) newErrors.llm_id = "LLM is required";
    if (chat.rag_n && (isNaN(chat.rag_n) || chat.rag_n < 0)) {
      newErrors.rag_n = "RAG N must be a non-negative number";
    }
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;
    const chatData = {
      data: {
        type: "Chat",
        attributes: {
          ...chat,
          llm_settings_id: parseInt(chat.llm_settings_id, 10),
          llm_id: parseInt(chat.llm_id, 10),
          group_ids: chat.groups.map((groupId) => parseInt(groupId, 10)),
          filter_ids: chat.filters.map((filterId) => parseInt(filterId, 10)),
          rag_n: chat.rag_n ? parseInt(chat.rag_n, 10) : null,
          tool_support: chat.tool_support,
          system_prompt: chat.system_prompt,
          default_data_source_id: chat.default_data_source_id
            ? parseInt(chat.default_data_source_id, 10)
            : null, // Add this line
          default_tool_ids: chat.default_tool_ids.map((id) => parseInt(id, 10)),
        },
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/chats/${id}`, chatData);
      } else {
        await apiClient.post("/chats", chatData);
      }

      setSnackbar({
        open: true,
        message: id ? "Chat updated successfully" : "Chat created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/chats"), 2000);
    } catch (error) {
      console.error("Error saving chat", error);
      setSnackbar({
        open: true,
        message: "Failed to save chat. Please try again.",
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

  if (loading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        height="100vh"
      >
        <CircularProgress />
      </Box>
    );
  }

  if (apiError) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        height="100vh"
      >
        <Typography color="error">{apiError}</Typography>
      </Box>
    );
  }

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">
          {id ? "Edit Chat Room" : "Add Chat Room"}
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/chats"
          color="white"
        >
          Back to Chat Rooms
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
                value={chat.name}
                onChange={handleChange}
                error={!!errors.name}
                helperText={errors.name}
                required
              />
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth error={!!errors.llm_settings_id}>
                <InputLabel>LLM Settings</InputLabel>
                <Select
                  name="llm_settings_id"
                  value={chat.llm_settings_id}
                  onChange={handleChange}
                  required
                >
                  {llmSettings.map((setting) => (
                    <MenuItem key={setting.id} value={setting.id}>
                      {setting.attributes.model_name}
                    </MenuItem>
                  ))}
                </Select>
                {errors.llm_settings_id && (
                  <Typography color="error">
                    {errors.llm_settings_id}
                  </Typography>
                )}
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth error={!!errors.llm_id}>
                <InputLabel>LLM</InputLabel>
                <Select
                  name="llm_id"
                  value={chat.llm_id}
                  onChange={handleChange}
                  required
                >
                  {llms.map((llm) => (
                    <MenuItem key={llm.id} value={llm.id}>
                      {llm.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
                {errors.llm_id && (
                  <Typography color="error">{errors.llm_id}</Typography>
                )}
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth>
                <InputLabel>Groups</InputLabel>
                <Select
                  multiple
                  value={chat.groups}
                  onChange={handleGroupChange}
                  renderValue={(selected) => (
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                      {selected.map((value) => (
                        <Chip
                          key={value}
                          label={
                            allGroups.find((group) => group.id === value)
                              ?.attributes.name
                          }
                        />
                      ))}
                    </Box>
                  )}
                >
                  {allGroups.map((group) => (
                    <MenuItem key={group.id} value={group.id}>
                      {group.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <FormControl fullWidth>
                <InputLabel>Filters</InputLabel>
                <Select
                  multiple
                  value={chat.filters}
                  onChange={handleFilterChange}
                  renderValue={(selected) => (
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                      {selected.map((value) => {
                        const filter = allFilters.find((f) => f.id === value);
                        return (
                          <Chip
                            key={value}
                            label={filter ? filter.attributes.name : "Unknown"}
                          />
                        );
                      })}
                    </Box>
                  )}
                >
                  {allFilters.map((filter) => (
                    <MenuItem key={filter.id} value={filter.id}>
                      {filter.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="System Prompt"
                name="system_prompt"
                value={chat.system_prompt}
                onChange={handleChange}
                multiline
                rows={4}
                placeholder="Enter the system prompt for this chat"
              />
            </Grid>

            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={chat.tool_support}
                    onChange={handleSwitchChange}
                    name="tool_support"
                    color="primary"
                  />
                }
                label="Enable Tool Support"
              />
            </Grid>

            <Grid item xs={12}>
              <TextField
                fullWidth
                label="RAG Results or Source to Include for Model"
                name="rag_n"
                type="number"
                value={chat.rag_n}
                onChange={handleChange}
                inputProps={{ min: 0 }}
              />
            </Grid>

            <Grid item xs={12}>
              <Alert severity="info" sx={{ mb: 2, marginBottom: 4 }}>
                Setting a default data source will automatically include this
                vector database in all conversations in this chat room, allowing
                the model to reference its contents when generating responses.
              </Alert>
              <FormControl fullWidth>
                <InputLabel>Default Data Source</InputLabel>
                <Select
                  name="default_data_source_id"
                  value={chat.default_data_source_id}
                  onChange={handleChange}
                >
                  <MenuItem value="">
                    <em>None</em>
                  </MenuItem>
                  {datasources.map((datasource) => (
                    <MenuItem key={datasource.id} value={datasource.id}>
                      {datasource.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>

            <Grid item xs={12}>
              <Alert severity="info" sx={{ mb: 2 }}>
                Select default tools that will be available in this chat room.
                These tools will be automatically accessible to the AI when
                responding to user queries.
              </Alert>
              <FormControl fullWidth>
                <InputLabel>Default Tools</InputLabel>
                <Select
                  multiple
                  value={chat.default_tool_ids}
                  onChange={handleToolChange}
                  renderValue={(selected) => (
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                      {selected.map((value) => {
                        const tool = allTools.find((t) => t.id === value);
                        return (
                          <Chip
                            key={value}
                            label={tool ? tool.attributes.name : "Unknown"}
                          />
                        );
                      })}
                    </Box>
                  )}
                >
                  {allTools.map((tool) => (
                    <MenuItem key={tool.id} value={tool.id}>
                      {tool.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
          </Grid>

          <StyledAccordion>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Typography>Extra Context</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography variant="body2" color="text.secondary" paragraph>
                Upload additional files to provide extra context for this chat
                room. These files will be used to enhance the model's
                understanding and responses.
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

              {id && ( // Only show upload button if editing an existing chat
                <Button
                  variant="contained"
                  startIcon={<CloudUploadIcon />}
                  onClick={() => fileInputRef.current.click()}
                  sx={{ mt: 2 }}
                >
                  Upload Context File
                </Button>
              )}

              {!id && (
                <Typography variant="caption" color="text.secondary">
                  You can add extra context files after creating the chat room.
                </Typography>
              )}
            </AccordionDetails>
          </StyledAccordion>

          <Box mt={4}>
            <StyledButton variant="contained" type="submit">
              {id ? "Update Chat Room" : "Add Chat Room"}
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

export default ChatForm;
