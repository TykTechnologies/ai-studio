import React, { useState, useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Box,
  Typography,
  CircularProgress,
  Snackbar,
  Alert,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import RelationshipPicker from "../common/relationship-picker";
import {
  SecondaryLinkButton,
  SecondaryOutlineButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../../styles/sharedStyles";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../../components/unsaved-changes";

const ToolCatalogueForm = () => {
  const [catalogue, setCatalogue] = useState({
    name: "",
    short_description: "",
    long_description: "",
    icon: "",
  });
  const [tools, setTools] = useState([]);
  const [availableTools, setAvailableTools] = useState([]);
  const [tags, setTags] = useState([]);
  const [availableTags, setAvailableTags] = useState([]);
  // The membership as loaded from the server, so save can diff against it
  // instead of tracking removals as they happen.
  const [loadedTools, setLoadedTools] = useState([]);
  const [loadedTags, setLoadedTags] = useState([]);
  const [loading, setLoading] = useState(false);
  // True once an existing catalog is on screen (`loading` here is the save
  // spinner, not the fetch).
  const [loaded, setLoaded] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  const navigate = useNavigate();
  const { id } = useParams();

  // Unsaved-changes tracking: the editable fields plus both pickers.
  const { markSaved } = useUnsavedForm(
    {
      name: catalogue.name,
      short_description: catalogue.short_description,
      long_description: catalogue.long_description,
      icon: catalogue.icon,
      toolIds: tools.map((tool) => String(tool.id)).sort(),
      tagIds: tags.map((tag) => String(tag.id)).sort(),
    },
    { ready: !id || loaded }
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/catalogs/tools"));

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
      if (response.data?.data?.attributes) {
        setCatalogue(response.data.data.attributes);
        setTools(response.data.data.attributes.tools || []);
        setTags(response.data.data.attributes.tags || []);
        setLoadedTools(response.data.data.attributes.tools || []);
        setLoadedTags(response.data.data.attributes.tags || []);
        setLoaded(true);
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
        apiClient.get("/tools", { params: { all: true } }),
        apiClient.get("/tags", { params: { all: true } }),
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

    // The pickers add to their lists the moment an option is chosen; there is
    // no pending "+" step, so what is on screen is exactly what gets saved.
    const desiredTools = tools;
    const desiredTags = tags;

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

      if (!response.data?.data?.id) {
        throw new Error(
          `Unexpected API response structure: ${JSON.stringify(response.data)}`,
        );
      }

      const newCatalogueId = response.data.data.id;

      // Update tools and tags
      await updateTools(newCatalogueId, desiredTools);
      await updateTags(newCatalogueId, desiredTags);

      markSaved();
      navigate("/admin/catalogs/tools", {
        state: { snackbar: { message: `Tool catalogue ${id ? "updated" : "created"} successfully`, severity: "success" } },
      });
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

  // Diffs the desired list against what was loaded: removes what is gone and
  // adds what is new. Re-posting an existing member is avoided so the save is
  // idempotent.
  const updateTools = async (catalogueId, desiredTools) => {
    try {
      // Remove tools
      for (const tool of loadedTools) {
        if (!desiredTools.find((t) => t.id === tool.id)) {
          await apiClient.delete(
            `/tool-catalogues/${catalogueId}/tools/${tool.id}`,
          );
        }
      }

      // Add new tools
      for (const tool of desiredTools) {
        if (!loadedTools.find((t) => t.id === tool.id)) {
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

  const updateTags = async (catalogueId, desiredTags) => {
    try {
      // Remove tags
      for (const tag of loadedTags) {
        if (!desiredTags.find((t) => t.id === tag.id)) {
          await apiClient.delete(`/tool-catalogues/${catalogueId}/tags/${tag.id}`);
        }
      }

      // Add new tags
      for (const tag of desiredTags) {
        if (!loadedTags.find((t) => t.id === tag.id)) {
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
          {id ? "Edit tool catalog" : "Add tool catalog"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={handleCancel}
          color="inherit"
        >
          Back to catalogs
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of tools that you can assign to specific teams to manage access easily.</Typography>  
      </Box>
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

          <Box sx={{ mt: 3 }}>
            <RelationshipPicker
              label="Tools in this catalog"
              itemLabel="tool"
              value={tools}
              onChange={setTools}
              options={availableTools}
              getOptionLabel={(tool) => tool.attributes?.name ?? ""}
            />
          </Box>

          <Box sx={{ mt: 3, mb: 3 }}>
            <RelationshipPicker
              label="Tags"
              itemLabel="tag"
              value={tags}
              onChange={setTags}
              options={availableTags}
              getOptionLabel={(tag) => tag.attributes?.name ?? ""}
            />
          </Box>

          <Box display="flex" gap={2}>
            <SecondaryOutlineButton onClick={handleCancel} disabled={loading}>
              Cancel
            </SecondaryOutlineButton>
            <PrimaryButton
              type="submit"
              variant="contained"
              color="primary"
              disabled={loading}
            >
              {loading ? (
                <CircularProgress size={24} />
              ) : id ? (
                "Update catalog"
              ) : (
                "Create catalog"
              )}
            </PrimaryButton>
          </Box>
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
    </>
  );
};

export default ToolCatalogueForm;
