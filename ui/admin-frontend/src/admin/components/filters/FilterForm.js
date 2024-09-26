import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
  Box,
  Typography,
  Grid,
  Snackbar,
  Alert,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
} from "../../styles/sharedStyles";

const FilterForm = () => {
  const [filter, setFilter] = useState({
    name: "",
    description: "",
    script: "",
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
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">
          {id ? "Edit Filter" : "Add Filter"}
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/filters"
          color="white"
        >
          Back to Filters
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
          <Box mt={4}>
            <StyledButton variant="contained" type="submit">
              {id ? "Update Filter" : "Add Filter"}
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

export default FilterForm;
