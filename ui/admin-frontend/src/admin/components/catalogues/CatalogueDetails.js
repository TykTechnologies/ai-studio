import React, { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import Section from "../common/Section";
import { CatalogueTeamsSection } from "../common/UsedBySection";
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
import useSystemFeatures from "../../hooks/useSystemFeatures";

// One kind of router published in the catalogue, linking to its page.
const RouterList = ({ title, routers, path, emptyText }) => (
  <Section title={title}>
    <List>
      {routers.length > 0 ? (
        routers.map((r) => (
          <React.Fragment key={r.id}>
            <ListItem component={Link} to={`/admin/${path}/${r.id}`} sx={{ color: "inherit" }}>
              <ListItemText
                primary={r.name}
                secondary={`${r.slug}${r.active ? "" : " (inactive)"}`}
              />
            </ListItem>
            <Divider />
          </React.Fragment>
        ))
      ) : (
        <ListItem>
          <ListItemText primary={emptyText} />
        </ListItem>
      )}
    </List>
  </Section>
);

const CatalogueDetails = () => {
  const [catalogue, setCatalogue] = useState(null);
  const [llms, setLLMs] = useState([]);
  const [routers, setRouters] = useState({ model_routers: [], semantic_routers: [] });
  const { features } = useSystemFeatures();
  const showModelRouters = Boolean(features?.feature_model_router);
  const showSemanticRouters = Boolean(features?.feature_semantic_router);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchCatalogueDetails();
    fetchCatalogueLLMs();
  }, [id]);

  useEffect(() => {
    if (!showModelRouters && !showSemanticRouters) return;
    apiClient
      .get(`/catalogues/${id}/routers`)
      .then((response) =>
        setRouters({
          model_routers: response.data.data.model_routers || [],
          semantic_routers: response.data.data.semantic_routers || [],
        })
      )
      .catch((err) => console.error("Error fetching catalog routers", err));
  }, [id, showModelRouters, showSemanticRouters]);

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
        <Section title="Catalog Information">
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
        </Section>

        <Section title="LLM providers in this catalog">
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
                <ListItemText primary="No LLM providers in this catalog" />
              </ListItem>
            )}
          </List>
        </Section>

        {showModelRouters && (
          <RouterList
            title="Model routers in this catalog"
            routers={routers.model_routers}
            path="model-routers"
            emptyText="No model routers in this catalog"
          />
        )}

        {showSemanticRouters && (
          <RouterList
            title="Semantic routers in this catalog"
            routers={routers.semantic_routers}
            path="semantic-routers"
            emptyText="No semantic routers in this catalog"
          />
        )}

        <CatalogueTeamsSection resourcePath="catalogues" id={id} />

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
