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

const CatalogueDetails = () => {
  const [catalogue, setCatalogue] = useState(null);
  const [llms, setLLMs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchCatalogueDetails();
    fetchCatalogueLLMs();
  }, [id]);

  const fetchCatalogueDetails = async () => {
    try {
      const response = await apiClient.get(`/catalogues/${id}`);
      setCatalogue(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching catalog details", error);
      setError("Failed to load catalog details");
      setLoading(false);
    }
  };

  const fetchCatalogueLLMs = async () => {
    try {
      const response = await apiClient.get(`/catalogues/${id}/llms`);
      setLLMs(response.data.data || []);
    } catch (error) {
      console.error("Error fetching catalog LLMs", error);
      setError("Failed to load LLMs for this catalog");
    }
  };

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!catalogue) return <Typography>Catalog not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">LLM catalog details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/catalogs/llms"
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
            <FieldValue>{catalogue.attributes.name}</FieldValue>
          </Grid>
          {catalogue.attributes.description && (
            <>
              <Grid item xs={3}>
                <FieldLabel>Description:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>{catalogue.attributes.description}</FieldValue>
              </Grid>
            </>
          )}
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>LLMs in this Catalog</SectionTitle>
        <List>
          {llms.length > 0 ? (
            llms.map((llm) => (
              <React.Fragment key={llm.id}>
                <ListItem>
                  <ListItemText
                    primary={llm.attributes.name}
                    secondary={llm.attributes.short_description}
                  />
                </ListItem>
                <Divider />
              </React.Fragment>
            ))
          ) : (
            <ListItem>
              <ListItemText primary="No LLMs in this catalog" />
            </ListItem>
          )}
        </List>

        <Box mt={4}>
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/catalogs/llms/edit/${id}`)}
          >
            Edit catalog
          </PrimaryButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default CatalogueDetails;
