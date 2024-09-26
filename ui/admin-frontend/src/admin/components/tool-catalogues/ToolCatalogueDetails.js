import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Chip,
  List,
  ListItem,
  ListItemText,
  Divider,
  Tooltip,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import EditIcon from "@mui/icons-material/Edit";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const ToolCatalogueDetails = () => {
  const [catalogue, setCatalogue] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchToolCatalogueDetails();
  }, [id]);

  const fetchToolCatalogueDetails = async () => {
    try {
      const response = await apiClient.get(`/tool-catalogues/${id}`);
      setCatalogue(response.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching tool catalogue details", error);
      setError("Failed to load tool catalogue details");
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!catalogue) return <Typography>Tool catalogue not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">Tool Catalog Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/catalogs/tools")}
          color="inherit"
        >
          Back to Tool Catalogs
        </Button>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Catalog Description</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{catalogue.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Short Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{catalogue.attributes.short_description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Long Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{catalogue.attributes.long_description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Icon:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{catalogue.attributes.icon}</FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Tools</SectionTitle>
        <List>
          {catalogue.attributes.tools &&
          catalogue.attributes.tools.length > 0 ? (
            catalogue.attributes.tools.map((tool) => (
              <React.Fragment key={tool.id}>
                <ListItem>
                  <ListItemText
                    primary={tool.attributes.name}
                    secondary={tool.attributes.description}
                  />
                </ListItem>
                <Divider />
              </React.Fragment>
            ))
          ) : (
            <ListItem>
              <ListItemText primary="No tools in this catalog" />
            </ListItem>
          )}
        </List>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Tags</SectionTitle>
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
          {catalogue.attributes.tags && catalogue.attributes.tags.length > 0 ? (
            catalogue.attributes.tags.map((tag) => (
              <Chip key={tag.id} label={tag.attributes.name} />
            ))
          ) : (
            <Typography>No tags for this catalog</Typography>
          )}
        </Box>

        <Box
          mt={4}
          display="flex"
          justifyContent="flex-end"
          alignItems="center"
        >
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/catalogs/tools/edit/${id}`)}
          >
            Edit Tool Catalog
          </StyledButton>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default ToolCatalogueDetails;
