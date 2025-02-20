import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Divider,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
} from "../../styles/sharedStyles";

const FilterDetails = () => {
  const [filter, setFilter] = useState(null);
  const [loading, setLoading] = useState(true);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchFilterDetails();
  }, [id]);

  const fetchFilterDetails = async () => {
    try {
      const response = await apiClient.get(`/filters/${id}`);
      const filterData = response.data; // Remove .data here
      setFilter({
        ...filterData,
        attributes: {
          ...filterData.attributes,
          script: atob(filterData.attributes.script), // Decode base64
        },
      });
      setLoading(false);
    } catch (error) {
      console.error("Error fetching filter details", error);
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (!filter) return <Typography>Filter not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h1">Filter Details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/filters")}
          color="inherit"
        >
          Back to Filters
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{filter.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{filter.attributes.description}</FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <Typography variant="h6" gutterBottom>
          Script
        </Typography>
        <Box
          sx={{
            backgroundColor: "#f5f5f5",
            padding: 2,
            borderRadius: 1,
            whiteSpace: "pre-wrap",
            fontFamily: "monospace",
          }}
        >
          {filter.attributes.script}
        </Box>

        <Box mt={4} display="flex" justifyContent="flex-end">
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/filters/edit/${id}`)}
          >
            Edit Filter
          </PrimaryButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default FilterDetails;
