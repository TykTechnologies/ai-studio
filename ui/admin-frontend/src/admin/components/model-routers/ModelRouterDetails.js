import React, { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Box,
  Typography,
  Grid,
  CircularProgress,
  Alert,
  Chip,
  Snackbar,
  Card,
  CardContent,
  Table,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import EditIcon from "@mui/icons-material/Edit";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryLinkButton,
  StyledPaper,
} from "../../styles/sharedStyles";

const ModelRouterDetails = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [router, setRouter] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  useEffect(() => {
    fetchRouter();
  }, [id]);

  const fetchRouter = async () => {
    try {
      setLoading(true);
      const response = await apiClient.get(`/model-routers/${id}`);
      setRouter(response.data.data);
      setError("");
    } catch (error) {
      console.error("Error fetching router:", error);
      setError("Failed to load Model Router");
    } finally {
      setLoading(false);
    }
  };

  const handleToggleActive = async () => {
    try {
      await apiClient.patch(`/model-routers/${id}/toggle`);
      setSnackbar({
        open: true,
        message: `Model Router ${!router.attributes.active ? "activated" : "deactivated"} successfully`,
        severity: "success",
      });
      fetchRouter();
    } catch (error) {
      console.error("Error toggling router:", error);
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
    return <Alert severity="warning">Model Router not found</Alert>;
  }

  const { attributes } = router;
  const endpointUrl = `/router/${attributes.slug}/v1/chat/completions`;

  return (
    <Box sx={{ p: 0 }}>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <SecondaryLinkButton
            component={Link}
            to="/admin/model-routers"
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
        </Box>
        <Box sx={{ display: "flex", gap: 2 }}>
          <PrimaryButton
            variant="outlined"
            onClick={handleToggleActive}
          >
            {attributes.active ? "Deactivate" : "Activate"}
          </PrimaryButton>
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/model-routers/edit/${id}`)}
          >
            Edit
          </PrimaryButton>
        </Box>
      </TitleBox>

      <ContentBox>
        <Grid container spacing={3}>
          {/* Basic Information */}
          <Grid item xs={12}>
            <StyledPaper sx={{ p: 3 }}>
              <Typography variant="h6" gutterBottom>
                Basic Information
              </Typography>
              <Grid container spacing={2}>
                <Grid item xs={12} md={6}>
                  <Typography variant="body2" color="text.secondary">
                    Name
                  </Typography>
                  <Typography variant="body1">{attributes.name}</Typography>
                </Grid>
                <Grid item xs={12} md={6}>
                  <Typography variant="body2" color="text.secondary">
                    Slug
                  </Typography>
                  <Chip label={attributes.slug} size="small" />
                </Grid>
                <Grid item xs={12}>
                  <Typography variant="body2" color="text.secondary">
                    Description
                  </Typography>
                  <Typography variant="body1">
                    {attributes.description || "No description"}
                  </Typography>
                </Grid>
                <Grid item xs={12} md={6}>
                  <Typography variant="body2" color="text.secondary">
                    API Compatibility
                  </Typography>
                  <Typography variant="body1">{attributes.api_compat}</Typography>
                </Grid>
                <Grid item xs={12} md={6}>
                  <Typography variant="body2" color="text.secondary">
                    Namespace
                  </Typography>
                  <Typography variant="body1">
                    {attributes.namespace || "Global"}
                  </Typography>
                </Grid>
              </Grid>
            </StyledPaper>
          </Grid>

          {/* Endpoint Information */}
          <Grid item xs={12}>
            <StyledPaper sx={{ p: 3 }}>
              <Typography variant="h6" gutterBottom>
                Endpoint
              </Typography>
              <Box
                sx={{
                  bgcolor: "grey.100",
                  p: 2,
                  borderRadius: 1,
                  fontFamily: "monospace",
                }}
              >
                <Typography variant="body2" color="text.secondary" gutterBottom>
                  POST
                </Typography>
                <Typography variant="body1">{endpointUrl}</Typography>
              </Box>
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: "block" }}>
                Send OpenAI-compatible chat completion requests to this endpoint.
                The model name in your request will be matched against pool patterns.
              </Typography>
            </StyledPaper>
          </Grid>

          {/* Pools */}
          <Grid item xs={12}>
            <Typography variant="h6" gutterBottom>
              Model Pools ({attributes.pools?.length || 0})
            </Typography>
            {attributes.pools?.map((pool, index) => (
              <Card key={index} variant="outlined" sx={{ mb: 2 }}>
                <CardContent>
                  <Grid container spacing={2}>
                    <Grid item xs={12}>
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                        <Typography variant="subtitle1">{pool.name}</Typography>
                        <Box sx={{ display: "flex", gap: 1 }}>
                          <Chip
                            label={`Pattern: ${pool.model_pattern}`}
                            size="small"
                            variant="outlined"
                          />
                          <Chip
                            label={pool.selection_algorithm === "round_robin" ? "Round Robin" : "Weighted"}
                            size="small"
                            color="primary"
                          />
                          <Chip
                            label={`Priority: ${pool.priority}`}
                            size="small"
                          />
                        </Box>
                      </Box>
                    </Grid>

                    {/* Vendors Table */}
                    <Grid item xs={12}>
                      <Typography variant="body2" color="text.secondary" gutterBottom>
                        Vendors ({pool.vendors?.length || 0})
                      </Typography>
                      <Table size="small">
                        <TableHead>
                          <TableRow>
                            <TableCell>LLM</TableCell>
                            <TableCell>Vendor</TableCell>
                            {pool.selection_algorithm === "weighted" && (
                              <TableCell>Weight</TableCell>
                            )}
                            <TableCell>Status</TableCell>
                            <TableCell>Mappings</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {pool.vendors?.map((vendor, vIndex) => (
                            <TableRow key={vIndex}>
                              <TableCell>
                                {vendor.llm?.name || `LLM #${vendor.llm_id}`}
                              </TableCell>
                              <TableCell>
                                <Chip label={vendor.llm?.vendor || "unknown"} size="small" />
                              </TableCell>
                              {pool.selection_algorithm === "weighted" && (
                                <TableCell>{vendor.weight}</TableCell>
                              )}
                              <TableCell>
                                <Chip
                                  icon={
                                    <FiberManualRecordIcon
                                      sx={{
                                        fontSize: 10,
                                        color: vendor.active ? "green" : "red",
                                      }}
                                    />
                                  }
                                  label={vendor.active ? "Active" : "Inactive"}
                                  size="small"
                                  variant="outlined"
                                />
                              </TableCell>
                              <TableCell>
                                {vendor.mappings?.length > 0 ? (
                                  <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
                                    {vendor.mappings.map((mapping, mIndex) => (
                                      <Box key={mIndex} sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
                                        <Chip label={mapping.source_model} size="small" sx={{ fontSize: "0.7rem" }} />
                                        <Typography variant="caption">→</Typography>
                                        <Chip label={mapping.target_model} size="small" color="primary" sx={{ fontSize: "0.7rem" }} />
                                      </Box>
                                    ))}
                                  </Box>
                                ) : (
                                  <Typography variant="caption" color="text.secondary">None</Typography>
                                )}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </Grid>

                  </Grid>
                </CardContent>
              </Card>
            ))}
          </Grid>
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

export default ModelRouterDetails;
