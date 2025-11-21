import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Box,
  Typography,
  Grid,
  Snackbar,
  Alert,
  FormControlLabel,
  Checkbox,
  FormHelperText,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../../styles/sharedStyles";
import EdgeAvailabilitySection from "../common/EdgeAvailabilitySection";

const FilterForm = () => {
  const [filter, setFilter] = useState({
    name: "",
    description: "",
    script: "",
    response_filter: false, // Response filter checkbox
    namespace: "", // Added for edge availability
  });
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    if (id) {
      fetchFilter();
    }
  }, [id]);

  const fetchFilter = async () => {
    try {
      const response = await apiClient.get(`/filters/${id}`);
      const filterData = response.data.attributes; // Remove .data here
      setFilter({
        ...filterData,
        script: atob(filterData.script), // Decode base64
        response_filter: filterData.response_filter || false, // Response filter flag
        namespace: filterData.namespace || "",
      });
    } catch (error) {
      console.error("Error fetching filter", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch filter details",
        severity: "error",
      });
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFilter({ ...filter, [name]: value });
  };

  const handleCheckboxChange = (e) => {
    const { name, checked } = e.target;
    setFilter({ ...filter, [name]: checked });
  };

  const handleNamespaceChange = (namespaces) => {
    // Convert array to comma-delimited string, or empty string for global
    const namespaceString = Array.isArray(namespaces) ? namespaces.join(', ') : namespaces;
    setFilter({ ...filter, namespace: namespaceString });
  };

  const validateForm = () => {
    const newErrors = {};
    if (!filter.name.trim()) newErrors.name = "Name is required";
    if (!filter.script.trim()) newErrors.script = "Script is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const filterData = {
      data: {
        type: "filter", // Changed to lowercase "filter"
        attributes: {
          ...filter,
          script: btoa(filter.script), // Encode to base64
        },
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/filters/${id}`, filterData);
      } else {
        await apiClient.post("/filters", filterData);
      }

      setSnackbar({
        open: true,
        message: id
          ? "Filter updated successfully"
          : "Filter created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/filters"), 2000);
    } catch (error) {
      console.error("Error saving filter", error);
      setSnackbar({
        open: true,
        message: "Failed to save filter. Please try again.",
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
          {id ? "Edit filter" : "Add filter"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/filters"
          color="inherit"
        >
          Back to filters
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Filters are used as a security layer to process and modify data before it is passed to the LLM. For example, filters can remove personally identifiable information to ensure privacy.</Typography>  
      </Box>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                name="name"
                value={filter.name}
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
                value={filter.description}
                onChange={handleChange}
                multiline
                rows={3}
              />
            </Grid>
            <Grid item xs={12}>
              <Box sx={{ mb: 2 }}>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={filter.response_filter || false}
                      onChange={handleCheckboxChange}
                      name="response_filter"
                    />
                  }
                  label="Is this a Response Filter?"
                />
                <FormHelperText sx={{ ml: 4, mt: 0 }}>
                  Response filters run on LLM responses only (not tools). They can only block responses, not modify them. Streaming responses will be interrupted if blocked.
                </FormHelperText>
              </Box>
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Script"
                name="script"
                value={filter.script}
                onChange={handleChange}
                error={!!errors.script}
                helperText={errors.script}
                required
                multiline
                rows={10}
              />
            </Grid>
          </Grid>

          {/* Edge Availability Section */}
          <EdgeAvailabilitySection
            value={filter.namespace}
            onChange={handleNamespaceChange}
            defaultExpanded={false}
          />

          <Box mt={4}>
            <PrimaryButton variant="contained" type="submit">
              {id ? "Update filter" : "Add filter"}
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

export default FilterForm;
