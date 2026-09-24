import React, { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Alert, Box, Chip, CircularProgress, Grid, Typography } from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import EditIcon from "@mui/icons-material/Edit";
import apiClient from "../../utils/apiClient";
import Section from "../common/Section";
import UsedBySection from "../common/UsedBySection";
import Can from "../rbac/Can";
import { P } from "../../rbac/permissions";
import { ContentBox, PrimaryButton, SecondaryLinkButton, TitleBox } from "../../styles/sharedStyles";
import { getEmbedderName } from "../../utils/vendorUtils";

const Fact = ({ label, children, md = 6 }) => (
  <Grid item xs={12} md={md}>
    <Typography variant="body2" color="text.secondary">
      {label}
    </Typography>
    <Typography variant="body1" component="div">
      {children}
    </Typography>
  </Grid>
);

const EmbedderDetails = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [embedder, setEmbedder] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    apiClient
      .get(`/embedders/${id}`)
      .then((response) => setEmbedder(response.data.data))
      .catch(() => setError("Failed to load embedder"));
  }, [id]);

  if (error) return <Alert severity="error">{error}</Alert>;
  if (!embedder) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", p: 4 }}>
        <CircularProgress />
      </Box>
    );
  }
  const a = embedder.attributes;

  return (
    <Box sx={{ p: 0 }}>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <SecondaryLinkButton component={Link} to="/admin/embedders" startIcon={<ArrowBackIcon />} color="inherit">
            Back
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">{a.name}</Typography>
          <Chip size="small" variant="outlined" label={a.linked ? "Uses an LLM provider" : "Standalone"} />
        </Box>
        <Can permission={P.EMBEDDERS_WRITE}>
          <PrimaryButton variant="contained" startIcon={<EditIcon />} onClick={() => navigate(`/admin/embedders/edit/${id}`)}>
            Edit
          </PrimaryButton>
        </Can>
      </TitleBox>
      <ContentBox>
        <Grid container spacing={3}>
          <Grid item xs={12}>
            <Section title="Embedding" sx={{ mb: 0 }}>
              <Grid container spacing={2}>
                <Fact label="Model">{a.model || "Not set"}</Fact>
                <Fact label="Privacy level">{a.privacy_score}</Fact>
                {a.linked ? (
                  <Fact label="LLM provider">
                    <Link to={`/admin/llms/${a.llm_id}`}>{a.llm_name || `LLM #${a.llm_id}`}</Link> ({getEmbedderName(a.vendor)})
                  </Fact>
                ) : (
                  <>
                    <Fact label="API compatibility">{getEmbedderName(a.vendor)}</Fact>
                    <Fact label={a.vendor === "vertex" ? "Project and location" : "Endpoint"}>{a.endpoint || "Vendor default"}</Fact>
                    <Fact label="API key">{a.has_api_key ? a.api_key : "Not set"}</Fact>
                  </>
                )}
                <Fact label="Description" md={12}>
                  {a.description || "No description"}
                </Fact>
              </Grid>
            </Section>
          </Grid>
          <Grid item xs={12}>
            <UsedBySection resourcePath="embedders" id={id} objectLabel="embedder" />
          </Grid>
        </Grid>
      </ContentBox>
    </Box>
  );
};

export default EmbedderDetails;
