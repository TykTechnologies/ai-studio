import React, { useState, useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
  Box,
  Typography,
  CircularProgress,
  Snackbar,
  Alert,
  Chip,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
} from "../../styles/sharedStyles";

const ToolCatalogueForm = () => {
  const [catalogue, setCatalogue] = useState({
    name: "",
    short_description: "",
    long_description: "",
    icon: "",
  });
  const [tools, setTools] = useState([]);
  const [removedTools, setRemovedTools] = useState([]);
  const [availableTools, setAvailableTools] = useState([]);
  const [selectedTool, setSelectedTool] = useState("");
  const [tags, setTags] = useState([]);
  const [removedTags, setRemovedTags] = useState([]);
  const [availableTags, setAvailableTags] = useState([]);
  const [selectedTag, setSelectedTag] = useState("");
  const [loading, setLoading] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    if (id) {
      fetchCatalogueDetails();
    }
    fetchAvailableToolsAndTags();
  }, [id]);

  const fetchCatalogueDetails = async () => {
    try {
      const response = await apiClient.get(`/tool-catalogues/${id}`);
      console.log("Catalogue details response:", response.data);
      if (response.data && response.data.attributes) {
        setCatalogue(response.data.attributes);
        setTools(response.data.attributes.tools || []);
        setTags(response.data.attributes.tags || []);
      } else {
        throw new Error("Unexpected API response structure");
      }
    } catch (error) {
      console.error("Error fetching catalogue details", error);
      setSnackbar({
        open: true,
        message: "Error fetching catalogue details: " + error.message,
        severity: "error",
      });
    }
  };

  const fetchAvailableToolsAndTags = async () => {
    try {
      const [toolsResponse, tagsResponse] = await Promise.all([
        apiClient.get("/tools"),
        apiClient.get("/tags"),
      ]);
      setAvailableTools(toolsResponse.data.data || []);
      setAvailableTags(tagsResponse.data.data || []);
    } catch (error) {
      console.error("Error fetching available tools and tags", error);
    }
  };

  const handleChange = (e) => {
    setCatalogue({ ...catalogue, [e.target.name]: e.target.value });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);

    const catalogueData = {
      data: {
        type: "ToolCatalogue",
        attributes: catalogue,
      },
    };

    try {
      let response;
      if (id) {
        response = await apiClient.patch(
          `/tool-catalogues/${id}`,
          catalogueData,
        );
      } else {
        response = await apiClient.post("/tool-catalogues", catalogueData);
      }

      console.log("API Response:", response.data);

      if (!response.data || !response.data.id) {
        throw new Error(
          `Unexpected API response structure: ${JSON.stringify(response.data)}`,
        );
      }

      const newCatalogueId = response.data.id;

      // Update tools and tags
      await updateTools(newCatalogueId);
      await updateTags(newCatalogueId);

      setSnackbar({
        open: true,
        message: `Tool catalogue ${id ? "updated" : "created"} successfully`,
        severity: "success",
      });

      setTimeout(() => navigate("/admin/catalogs/tools"), 2000);
    } catch (error) {
      console.error("Error saving tool catalogue", error);
      setSnackbar({
        open: true,
        message: `Error saving tool catalogue: ${error.message}`,
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  };

  const updateTools = async (catalogueId) => {
    try {
      // Remove tools
      for (const toolId of removedTools) {
        await apiClient.delete(
          `/tool-catalogues/${catalogueId}/tools/${toolId}`,
        );
      }

      // Add new tools
      for (const tool of tools) {
        if (!tool.id.startsWith("temp_")) {
          await apiClient.post(`/tool-catalogues/${catalogueId}/tools`, {
            data: { id: tool.id, type: "Tool" },
          });
        }
      }
    } catch (error) {
      console.error(`Error updating tools for catalogue ${catalogueId}`, error);
      throw error;
    }
  };

  const updateTags = async (catalogueId) => {
    try {
      // Remove tags
      for (const tagId of removedTags) {
        await apiClient.delete(`/tool-catalogues/${catalogueId}/tags/${tagId}`);
      }

      // Add new tags
      for (const tag of tags) {
        if (!tag.id.startsWith("temp_")) {
          await apiClient.post(`/tool-catalogues/${catalogueId}/tags`, {
            data: { id: tag.id, type: "Tag" },
          });
        }
      }
    } catch (error) {
      console.error(`Error updating tags for catalogue ${catalogueId}`, error);
      throw error;
    }
  };

  const handleAddTool = () => {
    if (selectedTool) {
      const toolToAdd = availableTools.find((t) => t.id === selectedTool);
      setTools([...tools, toolToAdd]);
      setSelectedTool("");
    }
  };

  const handleRemoveTool = (toolId) => {
    setTools(tools.filter((t) => t.id !== toolId));
    if (!toolId.startsWith("temp_")) {
      setRemovedTools([...removedTools, toolId]);
    }
  };

  const handleAddTag = () => {
    if (selectedTag) {
      const tagToAdd = availableTags.find((t) => t.id === selectedTag);
      setTags([...tags, tagToAdd]);
      setSelectedTag("");
    }
  };

  const handleRemoveTag = (tagId) => {
    setTags(tags.filter((t) => t.id !== tagId));
    if (!tagId.startsWith("temp_")) {
      setRemovedTags([...removedTags, tagId]);
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
          {id ? "Edit Tool Catalog" : "Create Tool Catalog"}
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/catalogs/tools")}
          color="inherit"
        >
          Back to Tool Catalogs
        </Button>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <TextField
            fullWidth
            margin="normal"
            label="Name"
            name="name"
            value={catalogue.name}
            onChange={handleChange}
            required
          />
          <TextField
            fullWidth
            margin="normal"
            label="Short Description"
            name="short_description"
            value={catalogue.short_description}
            onChange={handleChange}
          />
          <TextField
            fullWidth
            margin="normal"
            label="Long Description"
            name="long_description"
            value={catalogue.long_description}
            onChange={handleChange}
            multiline
            rows={4}
          />
          <TextField
            fullWidth
            margin="normal"
            label="Icon"
            name="icon"
            value={catalogue.icon}
            onChange={handleChange}
          />

          <Typography variant="h6" sx={{ mt: 2 }}>
            Tools
          </Typography>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mb: 2 }}>
            {tools.map((tool) => (
              <Chip
                key={tool.id}
                label={tool.attributes.name}
                onDelete={() => handleRemoveTool(tool.id)}
              />
            ))}
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
            <FormControl fullWidth sx={{ mr: 1 }}>
              <InputLabel>Add Tool</InputLabel>
              <Select
                value={selectedTool}
                onChange={(e) => setSelectedTool(e.target.value)}
                label="Add Tool"
              >
                {availableTools
                  .filter((t) => !tools.find((tool) => tool.id === t.id))
                  .map((tool) => (
                    <MenuItem key={tool.id} value={tool.id}>
                      {tool.attributes.name}
                    </MenuItem>
                  ))}
              </Select>
            </FormControl>
            <Button variant="contained" onClick={handleAddTool}>
              Add
            </Button>
          </Box>

          <Typography variant="h6" sx={{ mt: 2 }}>
            Tags
          </Typography>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mb: 2 }}>
            {tags.map((tag) => (
              <Chip
                key={tag.id}
                label={tag.attributes.name}
                onDelete={() => handleRemoveTag(tag.id)}
              />
            ))}
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
            <FormControl fullWidth sx={{ mr: 1 }}>
              <InputLabel>Add Tag</InputLabel>
              <Select
                value={selectedTag}
                onChange={(e) => setSelectedTag(e.target.value)}
                label="Add Tag"
              >
                {availableTags
                  .filter((t) => !tags.find((tag) => tag.id === t.id))
                  .map((tag) => (
                    <MenuItem key={tag.id} value={tag.id}>
                      {tag.attributes.name}
                    </MenuItem>
                  ))}
              </Select>
            </FormControl>
            <Button variant="contained" onClick={handleAddTag}>
              Add
            </Button>
          </Box>

          <StyledButton
            type="submit"
            variant="contained"
            color="primary"
            disabled={loading}
          >
            {loading ? (
              <CircularProgress size={24} />
            ) : id ? (
              "Update Tool Catalog"
            ) : (
              "Create Tool Catalog"
            )}
          </StyledButton>
        </Box>
      </ContentBox>
      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
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

export default ToolCatalogueForm;
