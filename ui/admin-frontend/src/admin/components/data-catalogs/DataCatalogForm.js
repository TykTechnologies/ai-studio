import React, { useState, useEffect } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
  Box,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Chip,
  Snackbar,
  Alert,
  CircularProgress,
} from "@mui/material";
import DeleteIcon from "@mui/icons-material/Delete";
import AddIcon from "@mui/icons-material/Add";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import { commitPendingSelection } from "../../utils/pendingSelection";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../../styles/sharedStyles";

const DataCatalogForm = () => {
  const [catalog, setCatalog] = useState({
    name: "",
    short_description: "",
    long_description: "",
    icon: "",
  });
  const [datasources, setDatasources] = useState([]);
  const [availableDatasources, setAvailableDatasources] = useState([]);
  const [selectedDatasource, setSelectedDatasource] = useState("");
  const [tags, setTags] = useState([]);
  const [availableTags, setAvailableTags] = useState([]);
  const [selectedTag, setSelectedTag] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [
          catalogResponse,
          availableDatasourcesResponse,
          availableTagsResponse,
        ] = await Promise.all([
          id ? apiClient.get(`/data-catalogues/${id}`) : Promise.resolve(null),
          apiClient.get("/datasources"),
          apiClient.get("/tags"),
        ]);

        if (catalogResponse) {
          setCatalog(catalogResponse.data.data.attributes);
          setDatasources(
            catalogResponse.data.data.attributes.datasources || [],
          );
          setTags(catalogResponse.data.data.attributes.tags || []);
        }
        setAvailableDatasources(availableDatasourcesResponse.data.data);
        setAvailableTags(availableTagsResponse.data.data);
      } catch (error) {
        console.error("Error fetching data", error);
        setSnackbar({
          open: true,
          message: "Error fetching data",
          severity: "error",
        });
      }
      setLoading(false);
    };

    fetchData();
  }, [id]);

  const handleChange = (e) => {
    setCatalog({ ...catalog, [e.target.name]: e.target.value });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);

    // The + buttons are accelerators, not the commit. Fold in anything left in
    // either picker rather than silently saving a catalog without it.
    const desiredDatasources = commitPendingSelection(
      datasources,
      selectedDatasource,
      availableDatasources,
    );
    const desiredTags = commitPendingSelection(tags, selectedTag, availableTags);
    if (desiredDatasources !== datasources) {
      setDatasources(desiredDatasources);
      setSelectedDatasource("");
    }
    if (desiredTags !== tags) {
      setTags(desiredTags);
      setSelectedTag("");
    }

    const catalogData = {
      data: {
        type: "DataCatalogue",
        attributes: catalog,
      },
    };

    try {
      let catalogId;
      if (id) {
        await apiClient.patch(`/data-catalogues/${id}`, catalogData);
        catalogId = id;
      } else {
        const response = await apiClient.post("/data-catalogues", catalogData);
        catalogId = response.data.data.id;
      }

      // Update datasources and tags
      await updateDatasources(catalogId, desiredDatasources);
      await updateTags(catalogId, desiredTags);

      setSnackbar({
        open: true,
        message: `Data catalog ${id ? "updated" : "created"} successfully`,
        severity: "success",
      });

      setTimeout(() => navigate("/admin/catalogs/data"), 2000);
    } catch (error) {
      console.error("Error saving data catalog", error);
      setSnackbar({
        open: true,
        message: "Error saving data catalog",
        severity: "error",
      });
      setLoading(false);
    }
  };

  // Takes the desired list explicitly so the caller can include a selection the
  // user picked but never added with the + button.
  const updateDatasources = async (catalogId, desiredDatasources) => {
    const originalDatasources = catalog.datasources || [];

    // Remove datasources
    for (let ds of originalDatasources) {
      if (!desiredDatasources.find((d) => d.id === ds.id)) {
        await apiClient.delete(
          `/data-catalogues/${catalogId}/datasources/${ds.id}`,
        );
      }
    }

    // Add new datasources
    for (let ds of desiredDatasources) {
      if (!originalDatasources.find((d) => d.id === ds.id)) {
        await apiClient.post(`/data-catalogues/${catalogId}/datasources`, {
          data: { id: ds.id, type: "Datasource" },
        });
      }
    }
  };

  const updateTags = async (catalogId, desiredTags) => {
    const originalTags = catalog.tags || [];

    // Remove tags
    for (let tag of originalTags) {
      if (!desiredTags.find((t) => t.id === tag.id)) {
        await apiClient.delete(`/data-catalogues/${catalogId}/tags/${tag.id}`);
      }
    }

    // Add new tags
    for (let tag of desiredTags) {
      if (!originalTags.find((t) => t.id === tag.id)) {
        await apiClient.post(`/data-catalogues/${catalogId}/tags`, {
          data: { id: tag.id, type: "Tag" },
        });
      }
    }
  };

  const handleAddDatasource = () => {
    const next = commitPendingSelection(
      datasources,
      selectedDatasource,
      availableDatasources,
    );
    if (next !== datasources) {
      setDatasources(next);
      setSelectedDatasource("");
    }
  };

  const handleRemoveDatasource = (datasourceId) => {
    setDatasources(datasources.filter((ds) => ds.id !== datasourceId));
  };

  const handleAddTag = () => {
    const next = commitPendingSelection(tags, selectedTag, availableTags);
    if (next !== tags) {
      setTags(next);
      setSelectedTag("");
    }
  };

  const handleRemoveTag = (tagId) => {
    setTags(tags.filter((t) => t.id !== tagId));
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading) {
    return <CircularProgress />;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          {id ? "Edit data catalog" : "Add data catalog"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/catalogs/data"
          color="inherit"
        >
          Back to catalogs
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of data sources that you can assign to specific teams to manage access easily.</Typography>  
      </Box>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <TextField
            fullWidth
            margin="normal"
            label="Catalog Name"
            name="name"
            value={catalog.name}
            onChange={handleChange}
            required
          />
          <TextField
            fullWidth
            margin="normal"
            label="Short Description"
            name="short_description"
            value={catalog.short_description}
            onChange={handleChange}
            required
          />
          <TextField
            fullWidth
            margin="normal"
            label="Long Description"
            name="long_description"
            value={catalog.long_description}
            onChange={handleChange}
            multiline
            rows={4}
          />
          <TextField
            fullWidth
            margin="normal"
            label="Icon"
            name="icon"
            value={catalog.icon}
            onChange={handleChange}
          />

          <Typography variant="h6" sx={{ mt: 2 }}>
            Data Sources:
          </Typography>
          <List>
            {datasources.map((ds) => (
              <ListItem key={ds.id}>
                <ListItemText primary={ds.attributes.name} />
                <ListItemSecondaryAction>
                  <IconButton
                    edge="end"
                    aria-label="delete"
                    onClick={() => handleRemoveDatasource(ds.id)}
                  >
                    <DeleteIcon />
                  </IconButton>
                </ListItemSecondaryAction>
              </ListItem>
            ))}
          </List>

          <Box sx={{ display: "flex", alignItems: "center", mt: 2 }}>
            <FormControl fullWidth sx={{ mr: 1 }}>
              <InputLabel id="datacatalogform-add-data-source-label">Add Data Source</InputLabel>
              <Select
              labelId="datacatalogform-add-data-source-label"
              id="datacatalogform-add-data-source"
                value={selectedDatasource}
                onChange={(e) => setSelectedDatasource(e.target.value)}
                label="Add Data Source"
              >
                {availableDatasources
                  .filter((ds) => !datasources.find((d) => d.id === ds.id))
                  .map((ds) => (
                    <MenuItem key={ds.id} value={ds.id}>
                      {ds.attributes.name}
                    </MenuItem>
                  ))}
              </Select>
            </FormControl>
            <Button
              variant="contained"
              color="secondary"
              onClick={handleAddDatasource}
              disabled={!selectedDatasource}
              aria-label="Add data source to catalog"
            >
              <AddIcon />
            </Button>
          </Box>

          <Typography variant="h6" sx={{ mt: 2 }}>
            Tags:
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

          <Box sx={{ display: "flex", alignItems: "center", mt: 2 }}>
            <FormControl fullWidth sx={{ mr: 1 }}>
              <InputLabel id="datacatalogform-add-tag-label">Add Tag</InputLabel>
              <Select
              labelId="datacatalogform-add-tag-label"
              id="datacatalogform-add-tag"
                value={selectedTag}
                onChange={(e) => setSelectedTag(e.target.value)}
                label="Add Tag"
              >
                {availableTags
                  .filter((t) => !tags.find((tag) => tag.id === t.id))
                  .map((t) => (
                    <MenuItem key={t.id} value={t.id}>
                      {t.attributes.name}
                    </MenuItem>
                  ))}
              </Select>
            </FormControl>
            <Button
              variant="contained"
              color="secondary"
              onClick={handleAddTag}
              disabled={!selectedTag}
              aria-label="Add tag to catalog"
            >
              <AddIcon />
            </Button>
          </Box>
          <Box mt={3}>
            <PrimaryButton type="submit" variant="contained" color="primary">
              {id ? "Update catalog" : "Create catalog"}
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

export default DataCatalogForm;
