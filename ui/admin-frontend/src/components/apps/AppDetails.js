import React, { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import { Typography, Button, CircularProgress, Box, Grid } from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
} from "../../styles/sharedStyles";

const AppDetails = () => {
  const [app, setApp] = useState(null);
  const [loading, setLoading] = useState(true);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchAppDetails();
  }, [id]);

  const fetchAppDetails = async () => {
    try {
      const response = await apiClient.get(`/apps/${id}`);
      setApp(response.data.data);
    } catch (error) {
      console.error("Error fetching app details", error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;

  if (!app) return <Typography>App not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h4" color="white">
          App Details
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
        <Grid container spacing={2} mb={4}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{app.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{app.attributes.description}</FieldValue>
          </Grid>
          {/* Add more fields as needed */}
        </Grid>
        <Box mt={2}>
          <Button
            variant="contained"
            color="primary"
            onClick={() => navigate(`/apps/edit/${id}`)}
          >
            Edit App
          </Button>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default AppDetails;
