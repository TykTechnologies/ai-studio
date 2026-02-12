import React, { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import pubClient from "../../admin/utils/pubClient";
import {
  Container,
  Typography,
  Box,
  Paper,
  Grid,
  Divider,
  CircularProgress,
  Alert,
  Chip,
  Snackbar,
  Button,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import StorageIcon from "@mui/icons-material/Storage";
import BuildIcon from "@mui/icons-material/Build";
import { PrimaryButton, PrimaryOutlineButton } from "../../admin/styles/sharedStyles";
import StatusChip from "../../admin/components/submissions/StatusChip";

const SubmissionDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [submission, setSubmission] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    const fetchSubmission = async () => {
      try {
        const response = await pubClient.get(`/common/submissions/${id}`);
        setSubmission(response.data.data);
      } catch (err) {
        setError("Failed to load submission details");
      } finally {
        setLoading(false);
      }
    };
    fetchSubmission();
  }, [id]);

  if (loading) {
    return (
      <Container sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Container>
    );
  }

  if (error || !submission) {
    return (
      <Container sx={{ mt: 4 }}>
        <Alert severity="error">{error || "Submission not found"}</Alert>
      </Container>
    );
  }

  const payload = submission.resource_payload || {};
  const canEdit =
    submission.status === "draft" ||
    submission.status === "changes_requested";

  return (
    <Container maxWidth={false} sx={{ px: 3, py: 3, width: "100%" }}>
      <Box sx={{ display: "flex", alignItems: "center", mb: 3, gap: 1 }}>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/portal/contributions")}
          color="inherit"
        >
          Back to My Contributions
        </Button>
      </Box>

      <Box sx={{ display: "flex", alignItems: "center", gap: 2, mb: 3 }}>
        <Typography variant="h4">{payload.name || "Untitled"}</Typography>
        <Chip
          icon={
            submission.resource_type === "datasource" ? (
              <StorageIcon />
            ) : (
              <BuildIcon />
            )
          }
          label={
            submission.resource_type === "datasource" ? "Data Source" : "Tool"
          }
          variant="outlined"
        />
        <StatusChip status={submission.status} />
        {submission.is_update && (
          <Chip label="Update" size="small" color="info" />
        )}
      </Box>

      {/* Admin feedback */}
      {submission.submitter_feedback &&
        (submission.status === "rejected" ||
          submission.status === "changes_requested") && (
          <Alert
            severity={
              submission.status === "rejected" ? "error" : "warning"
            }
            sx={{ mb: 3 }}
          >
            <Typography variant="subtitle2" gutterBottom>
              Admin Feedback
            </Typography>
            <Typography variant="body2">
              {submission.submitter_feedback}
            </Typography>
          </Alert>
        )}

      <Grid container spacing={3}>
        {/* Timeline */}
        <Grid item xs={12} md={4}>
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Timeline
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              <Typography variant="body2">
                <strong>Created:</strong>{" "}
                {new Date(submission.created_at).toLocaleString()}
              </Typography>
              {submission.submitted_at && (
                <Typography variant="body2">
                  <strong>Submitted:</strong>{" "}
                  {new Date(submission.submitted_at).toLocaleString()}
                </Typography>
              )}
              {submission.review_started_at && (
                <Typography variant="body2">
                  <strong>Review started:</strong>{" "}
                  {new Date(submission.review_started_at).toLocaleString()}
                </Typography>
              )}
              {submission.review_completed_at && (
                <Typography variant="body2">
                  <strong>Review completed:</strong>{" "}
                  {new Date(submission.review_completed_at).toLocaleString()}
                </Typography>
              )}
            </Box>

            <Divider sx={{ my: 2 }} />

            <Typography variant="h6" gutterBottom>
              Governance
            </Typography>
            <Typography variant="body2">
              <strong>Suggested privacy score:</strong>{" "}
              {submission.suggested_privacy}
            </Typography>
            {submission.final_privacy_score != null && (
              <Typography variant="body2">
                <strong>Final privacy score:</strong>{" "}
                {submission.final_privacy_score}
              </Typography>
            )}
            {submission.privacy_justification && (
              <Typography variant="body2" sx={{ mt: 1 }}>
                <strong>Justification:</strong>{" "}
                {submission.privacy_justification}
              </Typography>
            )}

            <Divider sx={{ my: 2 }} />

            <Typography variant="h6" gutterBottom>
              Support
            </Typography>
            {submission.primary_contact && (
              <Typography variant="body2">
                <strong>Primary contact:</strong> {submission.primary_contact}
              </Typography>
            )}
            {submission.secondary_contact && (
              <Typography variant="body2">
                <strong>Secondary contact:</strong>{" "}
                {submission.secondary_contact}
              </Typography>
            )}
            {submission.documentation_url && (
              <Typography variant="body2">
                <strong>Documentation:</strong>{" "}
                {/^https?:\/\//i.test(submission.documentation_url) ? (
                  <a
                    href={submission.documentation_url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {submission.documentation_url}
                  </a>
                ) : (
                  submission.documentation_url
                )}
              </Typography>
            )}
          </Paper>
        </Grid>

        {/* Resource details */}
        <Grid item xs={12} md={8}>
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Resource Configuration
            </Typography>

            {submission.resource_type === "datasource" && (
              <Grid container spacing={2}>
                <Grid item xs={6}>
                  <Typography variant="body2" color="text.secondary">
                    Vector DB Type
                  </Typography>
                  <Typography>{payload.db_source_type || "—"}</Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="body2" color="text.secondary">
                    Database Name
                  </Typography>
                  <Typography>{payload.db_name || "—"}</Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="body2" color="text.secondary">
                    Embed Vendor
                  </Typography>
                  <Typography>{payload.embed_vendor || "—"}</Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="body2" color="text.secondary">
                    Embed Model
                  </Typography>
                  <Typography>{payload.embed_model || "—"}</Typography>
                </Grid>
                {payload.short_description && (
                  <Grid item xs={12}>
                    <Typography variant="body2" color="text.secondary">
                      Description
                    </Typography>
                    <Typography>{payload.short_description}</Typography>
                  </Grid>
                )}
              </Grid>
            )}

            {submission.resource_type === "tool" && (
              <Grid container spacing={2}>
                <Grid item xs={6}>
                  <Typography variant="body2" color="text.secondary">
                    Tool Type
                  </Typography>
                  <Typography>{payload.tool_type || "—"}</Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="body2" color="text.secondary">
                    Auth Scheme
                  </Typography>
                  <Typography>{payload.auth_schema_name || "—"}</Typography>
                </Grid>
                {payload.description && (
                  <Grid item xs={12}>
                    <Typography variant="body2" color="text.secondary">
                      Description
                    </Typography>
                    <Typography>{payload.description}</Typography>
                  </Grid>
                )}
                {payload.available_operations && (
                  <Grid item xs={12}>
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{ mb: 0.5 }}
                    >
                      Operations
                    </Typography>
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                      {payload.available_operations
                        .split(",")
                        .map((op) => (
                          <Chip key={op} label={op.trim()} size="small" variant="outlined" />
                        ))}
                    </Box>
                  </Grid>
                )}
              </Grid>
            )}

            {submission.notes && (
              <>
                <Divider sx={{ my: 2 }} />
                <Typography variant="h6" gutterBottom>
                  Notes
                </Typography>
                <Typography variant="body2">{submission.notes}</Typography>
              </>
            )}
          </Paper>
        </Grid>
      </Grid>

      {/* Actions */}
      <Box sx={{ mt: 3, display: "flex", gap: 2 }}>
        {canEdit && (
          <PrimaryButton
            onClick={() =>
              navigate(`/portal/submissions/edit/${submission.id}`)
            }
          >
            {submission.status === "changes_requested"
              ? "Edit & Resubmit"
              : "Edit Draft"}
          </PrimaryButton>
        )}
        {submission.status === "approved" && submission.resource_id && (
          <PrimaryOutlineButton
            onClick={() => navigate("/portal/dashboard")}
          >
            View in Catalogue
          </PrimaryOutlineButton>
        )}
      </Box>
    </Container>
  );
};

export default SubmissionDetail;
