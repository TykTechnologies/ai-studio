import React, { useState, useEffect } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Box,
  Typography,
  Snackbar,
  Alert,
  CircularProgress,
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

const DataCatalogForm = () => {
  const [catalog, setCatalog] = useState({
    name: "",
    short_description: "",
    long_description: "",
    icon: "",
  });
  const [datasources, setDatasources] = useState([]);
  const [availableDatasources, setAvailableDatasources] = useState([]);
  const [tags, setTags] = useState([]);
  const [availableTags, setAvailableTags] = useState([]);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const { id } = useParams();

  // Unsaved-changes tracking: the editable fields plus both pickers.
  const { markSaved } = useUnsavedForm(
    {
      name: catalog.name,
      short_description: catalog.short_description,
      long_description: catalog.long_description,
      icon: catalog.icon,
      datasourceIds: datasources.map((ds) => String(ds.id)).sort(),
      tagIds: tags.map((tag) => String(tag.id)).sort(),
    },
    { ready: !loading }
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/catalogs/data"));

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [
          catalogResponse,
          availableDatasourcesResponse,
          availableTagsResponse,
        ] = await Promise.all([
          id ? apiClient.get(`/data-catalogues/${id}`) : Promise.resolve(null),
          apiClient.get("/datasources", { params: { all: true } }),
          apiClient.get("/tags", { params: { all: true } }),
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

    // The pickers add to their lists the moment an option is chosen; there is
    // no pending "+" step, so what is on screen is exactly what gets saved.
    const desiredDatasources = datasources;
    const desiredTags = tags;

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

      markSaved();
      navigate("/admin/catalogs/data", {
        state: { snackbar: { message: `Data catalog ${id ? "updated" : "created"} successfully`, severity: "success" } },
      });
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

  // Takes the desired list explicitly rather than reading state from the closure.
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

          <Box sx={{ mt: 3 }}>
            <RelationshipPicker
              label="Data sources in this catalog"
              itemLabel="data source"
              value={datasources}
              onChange={setDatasources}
              options={availableDatasources}
              getOptionLabel={(ds) => ds.attributes?.name ?? ""}
            />
          </Box>

          <Box sx={{ mt: 3 }}>
            <RelationshipPicker
              label="Tags"
              itemLabel="tag"
              value={tags}
              onChange={setTags}
              options={availableTags}
              getOptionLabel={(tag) => tag.attributes?.name ?? ""}
            />
          </Box>
          <Box mt={3} display="flex" gap={2}>
            <SecondaryOutlineButton onClick={handleCancel}>
              Cancel
            </SecondaryOutlineButton>
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
