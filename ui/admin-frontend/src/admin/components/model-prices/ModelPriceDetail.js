import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Divider,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../../utils/vendorLogos";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const ModelPriceDetail = () => {
  const [price, setPrice] = useState(null);
  const [loading, setLoading] = useState(true);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchPriceDetails();
  }, [id]);

  const fetchPriceDetails = async () => {
    try {
      const response = await apiClient.get(`/model-prices/${id}`);
      setPrice(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching Model Price details", error);
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (!price) return <Typography>Model Price not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">Model Price Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/model-prices")}
          color="white"
        >
          Back to Model Prices
        </Button>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Basic Information</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={4}>
            <FieldLabel>Model Name:</FieldLabel>
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{price.attributes.model_name}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <FieldLabel>Vendor:</FieldLabel>
          </Grid>
          <Grid item xs={8}>
            <Box display="flex" alignItems="center">
              <img
                src={getVendorLogo(price.attributes.vendor)}
                alt={price.attributes.vendor}
                style={{ width: 24, height: 24, marginRight: 8 }}
              />
              <FieldValue>{getVendorName(price.attributes.vendor)}</FieldValue>
            </Box>
          </Grid>
          <Grid item xs={4}>
            <FieldLabel>Cost per Input Token:</FieldLabel>
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{price.attributes.cpit}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <FieldLabel>Cost per Output Token:</FieldLabel>
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{price.attributes.cpt}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <FieldLabel>Currency:</FieldLabel>
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{price.attributes.currency}</FieldValue>
          </Grid>
        </Grid>

        <Box mt={4} display="flex" justifyContent="flex-end">
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/model-prices/edit/${id}`)}
          >
            Edit Model Price
          </StyledButton>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default ModelPriceDetail;
