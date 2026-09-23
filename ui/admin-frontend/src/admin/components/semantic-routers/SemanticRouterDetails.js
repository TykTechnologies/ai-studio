import React, { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import Section from "../common/Section";
import UsedBySection from "../common/UsedBySection";
import {
  Box,
  Typography,
  Grid,
  CircularProgress,
  Alert,
  Chip,
  IconButton,
  Snackbar,
  Table,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
  Tooltip,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import EditIcon from "@mui/icons-material/Edit";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryLinkButton,
} from "../../styles/sharedStyles";
import Can from "../rbac/Can";
import { P } from "../../rbac/permissions";
import { listAll } from "../../utils/listAll";
import SemanticRouterTestPanel, { describeTarget } from "./SemanticRouterTestPanel";
import {
  DEFAULT_AFFINITY_HEADER,
  DEFAULT_THRESHOLD,
  INPUT_SCOPE_LABELS,
  JUDGE_WHEN_LABELS,
  MODE_LABELS,
} from "./semanticRouterModel";

const copyText = (value) => {
  if (navigator.clipboard?.writeText) {
    navigator.clipboard.writeText(value).catch(() => undefined);
  }
};

const Fact = ({ label, children, md = 4 }) => (
  <Grid item xs={12} md={md}>
    <Typography variant="body2" color="text.secondary">
      {label}
    </Typography>
    <Typography variant="body1" component="div">
      {children}
    </Typography>
  </Grid>
);

const nameOf = (rows, id, fallback) =>
  (rows || []).find((row) => String(row.id) === String(id))?.attributes?.name || fallback;

const SemanticRouterDetails = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [router, setRouter] = useState(null);
  const [llms, setLLMs] = useState([]);
  const [modelRouters, setModelRouters] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  useEffect(() => {
    fetchRouter();
    // Names for the route targets; the router stores ids only.
    listAll(apiClient, "/llms")
      .then((response) => setLLMs(response.data.data || []))
      .catch(() => setLLMs([]));
    listAll(apiClient, "/model-routers")
      .then((response) => setModelRouters(response.data.data || []))
      .catch(() => setModelRouters([]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const fetchRouter = async () => {
    try {
      setLoading(true);
      const response = await apiClient.get(`/semantic-routers/${id}`);
      setRouter(response.data.data);
      setError("");
    } catch (error) {
      console.error("Error fetching semantic router:", error);
      setError("Failed to load Semantic Router");
    } finally {
      setLoading(false);
    }
  };

  const handleToggleActive = async () => {
    const active = !router.attributes.active;
    try {
      await apiClient.patch(`/semantic-routers/${id}/toggle`, { active });
      setSnackbar({
        open: true,
        message: `Semantic Router ${active ? "activated" : "deactivated"} successfully`,
        severity: "success",
      });
      fetchRouter();
    } catch (error) {
      console.error("Error toggling semantic router:", error);
      setSnackbar({
        open: true,
        message: "Failed to toggle router status",
        severity: "error",
      });
    }
  };

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", p: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return <Alert severity="error">{error}</Alert>;
  }

  if (!router) {
    return <Alert severity="warning">Semantic Router not found</Alert>;
  }

  const { attributes } = router;
  const settings = attributes.settings || {};
  const routes = attributes.routes || [];
  const catalogues = attributes.catalogues || [];
  const models = attributes.models?.length ? attributes.models : [`${attributes.slug}/auto`];
  const routeNames = routes.map((route) => route.name);
  const embedding = settings.embedding;
  const judge = settings.judge || {};
  const affinity = settings.affinity || {};

  return (
    <Box sx={{ p: 0 }}>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <SecondaryLinkButton
            component={Link}
            to="/admin/semantic-routers"
            startIcon={<ArrowBackIcon />}
            color="inherit"
          >
            Back
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">{attributes.name}</Typography>
          <Chip
            icon={
              <FiberManualRecordIcon
                sx={{ fontSize: 12, color: attributes.active ? "green" : "red" }}
              />
            }
            label={attributes.active ? "Active" : "Inactive"}
            size="small"
            variant="outlined"
          />
          {settings.mode === "shadow" && <Chip label="Shadow" size="small" color="warning" variant="outlined" />}
        </Box>
        <Box sx={{ display: "flex", gap: 2 }}>
          {/* The toggle is publish-gated (PATCH /semantic-routers/:id/toggle);
              the edit form is write-gated, and publish never implies write. */}
          <Can permission={P.SEMANTIC_ROUTERS_PUBLISH}>
            <PrimaryButton variant="outlined" onClick={handleToggleActive}>
              {attributes.active ? "Deactivate" : "Activate"}
            </PrimaryButton>
          </Can>
          <Can permission={P.SEMANTIC_ROUTERS_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<EditIcon />}
              onClick={() => navigate(`/admin/semantic-routers/edit/${id}`)}
            >
              Edit
            </PrimaryButton>
          </Can>
        </Box>
      </TitleBox>

      <ContentBox>
        <Grid container spacing={3}>
          {/* Basic Information */}
          <Grid item xs={12}>
            <Section title="Basic Information" sx={{ mb: 0 }}>
              <Grid container spacing={2}>
                <Fact label="Name" md={6}>
                  {attributes.name}
                </Fact>
                <Fact label="Slug" md={6}>
                  <Chip label={attributes.slug} size="small" />
                </Fact>
                <Fact label="Description" md={12}>
                  {attributes.description || "No description"}
                </Fact>
                <Fact label="Namespace" md={6}>
                  {attributes.namespace || "Global"}
                </Fact>
              </Grid>
            </Section>
          </Grid>

          {/* Endpoint: the unified ingress and the model strings to send */}
          <Grid item xs={12}>
            <Section title="Endpoint" sx={{ mb: 0 }}>
              <Typography variant="body2" sx={{ mb: 1 }}>
                Send OpenAI-compatible chat completion requests to the gateway&apos;s unified endpoint{" "}
                <code>/v1/chat/completions</code>, naming this router in the model field.
              </Typography>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }} data-testid="router-model-strings">
                {models.map((model) => (
                  <Box key={model} sx={{ display: "flex", alignItems: "center" }}>
                    <Chip label={model} size="small" variant="outlined" sx={{ fontFamily: "monospace" }} />
                    <Tooltip title={`Copy ${model}`}>
                      <IconButton size="small" aria-label={`Copy ${model}`} onClick={() => copyText(model)}>
                        <ContentCopyIcon fontSize="inherit" />
                      </IconButton>
                    </Tooltip>
                  </Box>
                ))}
              </Box>
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: "block" }}>
                <code>{attributes.slug}/auto</code> classifies the prompt.
                {settings.allow_explicit_route
                  ? ` ${attributes.slug}/<route> picks a route directly.`
                  : " Explicit routes are off, so only /auto is accepted."}
              </Typography>
            </Section>
          </Grid>

          {/* Routes */}
          <Grid item xs={12}>
            <Section title={`Routes (${routes.length})`} sx={{ mb: 0 }}>
              <Table size="small" aria-label="Routes">
                <TableHead>
                  <TableRow>
                    <TableCell>Name</TableCell>
                    <TableCell>Description</TableCell>
                    <TableCell>Target</TableCell>
                    <TableCell align="right">Keywords</TableCell>
                    <TableCell align="right">Examples</TableCell>
                    <TableCell align="right">Threshold</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {routes.map((route) => (
                    <TableRow key={route.name} data-testid="route-row">
                      <TableCell>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                          <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                            {route.name}
                          </Typography>
                          {route.name === settings.default_route && <Chip label="Default" size="small" color="primary" />}
                        </Box>
                      </TableCell>
                      <TableCell>{route.description || "—"}</TableCell>
                      <TableCell>{describeTarget(route.target, { llms, modelRouters })}</TableCell>
                      <TableCell align="right">{route.keywords?.length || 0}</TableCell>
                      <TableCell align="right">{route.utterances?.length || 0}</TableCell>
                      <TableCell align="right">{route.threshold || DEFAULT_THRESHOLD}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Section>
          </Grid>

          {/* Settings */}
          <Grid item xs={12}>
            <Section title="Classification" sx={{ mb: 0 }}>
              <Grid container spacing={2} data-testid="router-settings">
                <Fact label="Mode">{MODE_LABELS[settings.mode] || MODE_LABELS.enforce}</Fact>
                <Fact label="Default route">{settings.default_route || "—"}</Fact>
                <Fact label="Text classified">
                  {INPUT_SCOPE_LABELS[settings.input_scope] || INPUT_SCOPE_LABELS.last_user}
                  {settings.max_input_chars ? ` (first ${settings.max_input_chars} characters)` : ""}
                </Fact>
                <Fact label="Explicit routes">{settings.allow_explicit_route ? "Allowed" : "Off"}</Fact>
                <Fact label="Embeddings">
                  {embedding?.llm_id
                    ? `${embedding.model} on ${nameOf(llms, embedding.llm_id, `LLM #${embedding.llm_id}`)}`
                    : "Off"}
                </Fact>
                <Fact label="LLM judge">
                  {judge.enabled
                    ? `${judge.model_ref?.model} on ${nameOf(llms, judge.model_ref?.llm_id, `LLM #${judge.model_ref?.llm_id}`)}, ${(
                        JUDGE_WHEN_LABELS[judge.when] || JUDGE_WHEN_LABELS.low_confidence
                      ).toLowerCase()}`
                    : "Off"}
                </Fact>
                <Fact label="Session affinity">
                  {affinity.enabled
                    ? `${affinity.header || DEFAULT_AFFINITY_HEADER}, ${affinity.ttl_seconds || 1800}s`
                    : "Off"}
                </Fact>
              </Grid>
            </Section>
          </Grid>

          {/* Portal */}
          <Grid item xs={12}>
            <Section title="Portal" sx={{ mb: 0 }}>
              <Typography variant="body2" color="text.secondary" gutterBottom>
                Published in catalogs
              </Typography>
              {catalogues.length > 0 ? (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }} data-testid="router-catalogues">
                  {catalogues.map((catalogue) => (
                    <Chip
                      key={catalogue.id}
                      label={catalogue.name}
                      size="small"
                      component={Link}
                      to={`/admin/catalogs/llms/${catalogue.id}`}
                      clickable
                    />
                  ))}
                </Box>
              ) : (
                <Typography variant="body1">
                  Not in any catalog. Portal users cannot add it to their Apps.
                </Typography>
              )}
            </Section>
          </Grid>

          {/* Used by */}
          <Grid item xs={12}>
            <UsedBySection resourcePath="semantic-routers" id={id} objectLabel="semantic router" sx={{ mb: 0 }} />
          </Grid>

          {/* Test the saved router; write-gated like the endpoint. */}
          <Can permission={P.SEMANTIC_ROUTERS_WRITE}>
            <Grid item xs={12}>
              <Section
                title="Test"
                description="Classify a prompt with the saved configuration."
                sx={{ mb: 0 }}
              >
                <SemanticRouterTestPanel
                  routerId={id}
                  routeNames={routeNames}
                  allowExplicit={Boolean(settings.allow_explicit_route)}
                  llms={llms}
                  modelRouters={modelRouters}
                />
              </Section>
            </Grid>
          </Can>
        </Grid>
      </ContentBox>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          severity={snackbar.severity}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default SemanticRouterDetails;
