import React, { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Button,
  List,
  ListItem,
  ListItemText,
  Divider,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import EditIcon from "@mui/icons-material/Edit";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";

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
    } catch (error) {
      console.error("Error fetching catalog details", error);
      setError("Failed to load catalog details");
    }
  };

  const fetchCatalogueLLMs = async () => {
    try {
      const response = await apiClient.get(`/catalogues/${id}/llms`);
      setLLMs(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching catalog LLMs", error);
      setError("Failed to load catalog LLMs");
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!catalogue) return <Typography>Catalog not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">Catalog Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/catalogs/llms"
          color="inherit"
        >
          Back to Catalogs
        </Button>
      </TitleBox>
      <ContentBox>
        <Box mb={3}>
          <FieldLabel>Name:</FieldLabel>
          <FieldValue>{catalogue.attributes.name}</FieldValue>
        </Box>

        <Typography variant="h6" gutterBottom>
          LLMs in this Catalog:
        </Typography>
        <List>
          {llms.length > 0 ? (
            llms.map((llm) => (
              <React.Fragment key={llm.id}>
                <ListItem>
                  <ListItemText primary={llm.attributes.name} />
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
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/catalogs/llms/edit/${id}`)}
          >
            Edit Catalog
          </StyledButton>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default CatalogueDetails;
