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
import { StyledPaper, TitleBox, ContentBox } from "../../styles/sharedStyles";

const AppForm = () => {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
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
      fetchApp();
    }
  }, [id]);

  const fetchApp = async () => {
    try {
      const response = await apiClient.get(`/apps/${id}`);
      const appData = response.data.data;
      setName(appData.attributes.name);
      setDescription(appData.attributes.description);
    } catch (error) {
      console.error("Error fetching app", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch app details",
        severity: "error",
      });
    }
  };

  const validateForm = () => {
    const newErrors = {};
    if (!name.trim()) newErrors.name = "Name is required";
    if (!description.trim()) newErrors.description = "Description is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const appData = {
      data: {
        type: "App",
        attributes: {
          name,
          description,
        },
      },
    };

    try {
      let response;
      if (id) {
        response = await apiClient.patch(`/apps/${id}`, appData);
      } else {
        response = await apiClient.post("/apps", appData);
      }

      setSnackbar({
        open: true,
        message: id ? "App updated successfully" : "App created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/apps"), 2000);
    } catch (error) {
      console.error("Error saving app", error);
      setSnackbar({
        open: true,
        message: "Failed to save app. Please try again.",
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
        <Typography variant="h4" color="white">
          {id ? "Edit App" : "Add App"}
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/apps"
          color="inherit"
        >
          Back to Apps
        </Button>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                error={!!errors.name}
                helperText={errors.name}
                required
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                error={!!errors.description}
                helperText={errors.description}
                required
                multiline
                rows={4}
              />
            </Grid>
            <Grid item xs={12}>
              <Button variant="contained" color="primary" type="submit">
                {id ? "Update App" : "Add App"}
              </Button>
            </Grid>
          </Grid>
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

export default AppForm;
