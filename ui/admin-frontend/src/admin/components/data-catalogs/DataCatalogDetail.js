import React, { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  List,
  ListItem,
  ListItemText,
  Divider,
  Chip,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import EditIcon from "@mui/icons-material/Edit";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
} from "../../styles/sharedStyles";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const DataCatalogDetail = () => {
  const [catalog, setCatalog] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchDataCatalogDetails();
  }, [id]);

  const fetchDataCatalogDetails = async () => {
    try {
      const response = await apiClient.get(`/data-catalogues/${id}`);
      setCatalog(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching data catalog details", error);
      setError("Failed to load data catalog details");
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!catalog) return <Typography>Data catalog not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Data catalog details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/catalogs/data"
          color="inherit"
        >
          Back to catalogs
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Catalog Information</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{catalog.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Short Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{catalog.attributes.short_description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Long Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{catalog.attributes.long_description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Icon:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{catalog.attributes.icon}</FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Data Sources</SectionTitle>
        <List>
          {catalog.attributes.datasources &&
          catalog.attributes.datasources.length > 0 ? (
            catalog.attributes.datasources.map((datasource) => (
              <React.Fragment key={datasource.id}>
                <ListItem>
                  <ListItemText
                    primary={datasource.attributes.name}
                    secondary={datasource.attributes.short_description}
                  />
                </ListItem>
                <Divider />
              </React.Fragment>
            ))
          ) : (
            <ListItem>
              <ListItemText primary="No data sources in this catalog" />
            </ListItem>
          )}
        </List>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Tags</SectionTitle>
        <Box>
          {catalog.attributes.tags && catalog.attributes.tags.length > 0 ? (
            catalog.attributes.tags.map((tag) => (
              <Chip
                key={tag.id}
                label={tag.attributes.name}
                sx={{ mr: 1, mb: 1 }}
              />
            ))
          ) : (
            <Typography>No tags for this catalog</Typography>
          )}
        </Box>

        <Box mt={4}>
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/catalogs/data/edit/${id}`)}
          >
            Edit catalog
          </PrimaryButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default DataCatalogDetail;
