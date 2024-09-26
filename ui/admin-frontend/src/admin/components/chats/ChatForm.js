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
  Chip,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
} from "../../styles/sharedStyles";

const ChatForm = () => {
  const [chat, setChat] = useState({
    name: "",
    llm_settings_id: "",
    llm_id: "",
    groups: [], // Change this line
  });
  const [llms, setLLMs] = useState([]);
  const [llmSettings, setLLMSettings] = useState([]);
  const [allGroups, setAllGroups] = useState([]);
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    fetchLLMs();
    fetchLLMSettings();
    fetchGroups();
    if (id) {
      fetchChat();
    }
  }, [id]);

  const fetchChat = async () => {
    try {
      const response = await apiClient.get(`/chats/${id}`);
      const chatData = response.data.data.attributes;
      setChat({
        name: chatData.name,
        llm_settings_id: chatData.llm_settings_id,
        llm_id: chatData.llm_id,
        groups: chatData.groups.map((group) => group.id.toString()), // Change this line
      });
    } catch (error) {
      console.error("Error fetching chat", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch chat details",
        severity: "error",
      });
    }
  };

  const fetchLLMs = async () => {
    try {
      const response = await apiClient.get("/llms");
      setLLMs(response.data.data);
    } catch (error) {
      console.error("Error fetching LLMs", error);
    }
  };

  const fetchLLMSettings = async () => {
    try {
      const response = await apiClient.get("/llm-settings");
      setLLMSettings(response.data.data);
    } catch (error) {
      console.error("Error fetching LLM settings", error);
    }
  };

  const fetchGroups = async () => {
    try {
      const response = await apiClient.get("/groups");
      setAllGroups(response.data.data);
    } catch (error) {
      console.error("Error fetching groups", error);
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setChat({ ...chat, [name]: value });
  };

  const handleGroupChange = (event) => {
    setChat({ ...chat, groups: event.target.value });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!chat.name.trim()) newErrors.name = "Name is required";
    if (!chat.llm_settings_id)
      newErrors.llm_settings_id = "LLM Settings is required";
    if (!chat.llm_id) newErrors.llm_id = "LLM is required";
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
          group_ids: chat.groups.map((groupId) => parseInt(groupId, 10)), // Change this line
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
          </Grid>

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
