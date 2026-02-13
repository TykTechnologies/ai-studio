import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
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
  TextField,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Slider,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import CancelIcon from "@mui/icons-material/Cancel";
import EditIcon from "@mui/icons-material/Edit";
import PlayArrowIcon from "@mui/icons-material/PlayArrow";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  PrimaryOutlineButton,
  SecondaryLinkButton,
} from "../styles/sharedStyles";
import StatusChip from "../components/submissions/StatusChip";

const SubmissionReview = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [submission, setSubmission] = useState(null);
  const [loading, setLoading] = useState(true);
  const [testResult, setTestResult] = useState(null);
  const [testing, setTesting] = useState(false);
  const [versions, setVersions] = useState([]);
  const [activities, setActivities] = useState([]);

  // Dialog state
  const [approveDialogOpen, setApproveDialogOpen] = useState(false);
  const [rejectDialogOpen, setRejectDialogOpen] = useState(false);
  const [changesDialogOpen, setChangesDialogOpen] = useState(false);
  const [finalPrivacyScore, setFinalPrivacyScore] = useState(50);
  const [reviewNotes, setReviewNotes] = useState("");
  const [feedback, setFeedback] = useState("");
  const [actionLoading, setActionLoading] = useState(false);

  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  useEffect(() => {
    fetchSubmission();
  }, [id]);

  const fetchSubmission = async () => {
    try {
      setLoading(true);
      const response = await apiClient.get(`/submissions/${id}`);
      const data = response.data.data;
      setSubmission(data);
      setFinalPrivacyScore(data.suggested_privacy || 50);

      // Fetch versions if resource exists
      if (data.resource_id) {
        try {
          const versionsResponse = await apiClient.get(
            `/submissions/${id}/versions`
          );
          setVersions(versionsResponse.data.data || []);
        } catch (e) {
          // Versions may not exist
        }
      }

      // Fetch activity audit trail
      try {
        const activitiesResponse = await apiClient.get(
          `/submissions/${id}/activities`
        );
        setActivities(activitiesResponse.data.data || []);
      } catch (e) {
        // Activities may not exist yet
      }
    } catch (err) {
      setSnackbar({
        open: true,
        message: "Failed to load submission",
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  };

  const handleTest = async () => {
    try {
      setTesting(true);
      setTestResult(null);
      const response = await apiClient.post(`/submissions/${id}/test`);
      setTestResult(response.data.data);
    } catch (err) {
      setTestResult({
        error: err.response?.data?.errors?.[0]?.detail || "Test failed",
      });
    } finally {
      setTesting(false);
    }
  };

  const handleApprove = async () => {
    try {
      setActionLoading(true);
      await apiClient.post(`/submissions/${id}/approve`, {
        data: {
          attributes: {
            final_privacy_score: finalPrivacyScore,
            review_notes: reviewNotes,
          },
        },
      });
      setSnackbar({
        open: true,
        message: "Submission approved — resource created",
        severity: "success",
      });
      setApproveDialogOpen(false);
      setTimeout(() => navigate("/admin/submissions"), 1500);
    } catch (err) {
      setSnackbar({
        open: true,
        message:
          err.response?.data?.errors?.[0]?.detail || "Failed to approve",
        severity: "error",
      });
    } finally {
      setActionLoading(false);
    }
  };

  const handleReject = async () => {
    try {
      setActionLoading(true);
      await apiClient.post(`/submissions/${id}/reject`, {
        data: {
          attributes: {
            feedback: feedback,
            review_notes: reviewNotes,
          },
        },
      });
      setSnackbar({
        open: true,
        message: "Submission rejected",
        severity: "success",
      });
      setRejectDialogOpen(false);
      setTimeout(() => navigate("/admin/submissions"), 1500);
    } catch (err) {
      setSnackbar({
        open: true,
        message:
          err.response?.data?.errors?.[0]?.detail || "Failed to reject",
        severity: "error",
      });
    } finally {
      setActionLoading(false);
    }
  };

  const handleRequestChanges = async () => {
    try {
      setActionLoading(true);
      await apiClient.post(`/submissions/${id}/request-changes`, {
        data: {
          attributes: {
            feedback: feedback,
            review_notes: reviewNotes,
          },
        },
      });
      setSnackbar({
        open: true,
        message: "Changes requested — submitter notified",
        severity: "success",
      });
      setChangesDialogOpen(false);
      setTimeout(() => navigate("/admin/submissions"), 1500);
    } catch (err) {
      setSnackbar({
        open: true,
        message:
          err.response?.data?.errors?.[0]?.detail ||
          "Failed to request changes",
        severity: "error",
      });
    } finally {
      setActionLoading(false);
    }
  };

  const handleStartReview = async () => {
    try {
      await apiClient.post(`/submissions/${id}/review`);
      fetchSubmission();
      setSnackbar({
        open: true,
        message: "Review started",
        severity: "success",
      });
    } catch (err) {
      setSnackbar({
        open: true,
        message:
          err.response?.data?.errors?.[0]?.detail ||
          "Failed to start review",
        severity: "error",
      });
    }
  };

  if (loading) {
    return (
      <ContentBox
        sx={{ display: "flex", justifyContent: "center", mt: 4 }}
      >
        <CircularProgress />
      </ContentBox>
    );
  }

  if (!submission) {
    return (
      <ContentBox>
        <Alert severity="error">Submission not found</Alert>
      </ContentBox>
    );
  }

  const payload = submission.resource_payload || {};
  const canReview =
    submission.status === "submitted" || submission.status === "in_review";

  return (
    <>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <Typography variant="headingXLarge">
            Review: {payload.name || "Untitled"}
          </Typography>
          <StatusChip status={submission.status} size="medium" />
        </Box>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/submissions")}
          color="inherit"
        >
          Back to Queue
        </SecondaryLinkButton>
      </TitleBox>

      <ContentBox>
        {/* Claim review banner */}
        {submission.status === "submitted" && (
          <Alert
            severity="info"
            sx={{ mb: 3 }}
            action={
              <Button color="inherit" size="small" onClick={handleStartReview}>
                Claim Review
              </Button>
            }
          >
            This submission is waiting for review. Click "Claim Review" to
            start.
          </Alert>
        )}

        <Grid container spacing={3}>
          {/* Left column: metadata */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 3 }}>
              <Typography variant="h6" gutterBottom>
                Submission Info
              </Typography>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <Typography variant="body2">
                  <strong>Type:</strong>{" "}
                  {submission.resource_type === "datasource"
                    ? "Data Source"
                    : "Tool"}
                  {submission.is_update && " (Update)"}
                </Typography>
                <Typography variant="body2">
                  <strong>Submitter:</strong>{" "}
                  {submission.submitter?.name || `User #${submission.submitter_id}`}
                </Typography>
                <Typography variant="body2">
                  <strong>Submitted:</strong>{" "}
                  {submission.submitted_at
                    ? new Date(submission.submitted_at).toLocaleString()
                    : "Not yet submitted"}
                </Typography>
                {submission.reviewer && (
                  <Typography variant="body2">
                    <strong>Reviewer:</strong> {submission.reviewer.name}
                  </Typography>
                )}
              </Box>

              <Divider sx={{ my: 2 }} />

              <Typography variant="h6" gutterBottom>
                Privacy
              </Typography>
              <Typography variant="body2">
                <strong>Suggested score:</strong> {submission.suggested_privacy}
              </Typography>
              {submission.privacy_justification && (
                <Typography variant="body2" sx={{ mt: 0.5 }}>
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
                  <strong>Primary:</strong> {submission.primary_contact}
                </Typography>
              )}
              {submission.secondary_contact && (
                <Typography variant="body2">
                  <strong>Secondary:</strong> {submission.secondary_contact}
                </Typography>
              )}
              {submission.documentation_url && (
                <Typography variant="body2">
                  <strong>Docs:</strong>{" "}
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
              {submission.notes && (
                <Typography variant="body2" sx={{ mt: 1 }}>
                  <strong>Notes:</strong> {submission.notes}
                </Typography>
              )}
            </Paper>
          </Grid>

          {/* Right column: resource config + test */}
          <Grid item xs={12} md={8}>
            <Paper sx={{ p: 3, mb: 3 }}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  mb: 2,
                }}
              >
                <Typography variant="h6">Resource Configuration</Typography>
                <PrimaryOutlineButton
                  startIcon={
                    testing ? (
                      <CircularProgress size={16} />
                    ) : (
                      <PlayArrowIcon />
                    )
                  }
                  onClick={handleTest}
                  disabled={testing}
                  size="small"
                >
                  Test Connection
                </PrimaryOutlineButton>
              </Box>

              {/* Resource payload display */}
              {submission.resource_type === "datasource" && (
                <Grid container spacing={1}>
                  {[
                    ["Name", payload.name],
                    ["Description", payload.short_description],
                    ["Vector DB Type", payload.db_source_type],
                    ["Database Name", payload.db_name],
                    ["Connection String", payload.db_conn_string ? "***" : "—"],
                    ["Embed Vendor", payload.embed_vendor],
                    ["Embed Model", payload.embed_model],
                    ["Embed URL", payload.embed_url],
                  ].map(([label, value]) => (
                    <React.Fragment key={label}>
                      <Grid item xs={4}>
                        <Typography
                          variant="body2"
                          color="text.secondary"
                          fontWeight="bold"
                        >
                          {label}
                        </Typography>
                      </Grid>
                      <Grid item xs={8}>
                        <Typography variant="body2">
                          {value || "—"}
                        </Typography>
                      </Grid>
                    </React.Fragment>
                  ))}
                </Grid>
              )}

              {submission.resource_type === "tool" && (
                <Grid container spacing={1}>
                  {[
                    ["Name", payload.name],
                    ["Description", payload.description],
                    ["Tool Type", payload.tool_type],
                    ["Auth Scheme", payload.auth_schema_name],
                    ["Operations", payload.available_operations],
                  ].map(([label, value]) => (
                    <React.Fragment key={label}>
                      <Grid item xs={4}>
                        <Typography
                          variant="body2"
                          color="text.secondary"
                          fontWeight="bold"
                        >
                          {label}
                        </Typography>
                      </Grid>
                      <Grid item xs={8}>
                        <Typography variant="body2">
                          {value || "—"}
                        </Typography>
                      </Grid>
                    </React.Fragment>
                  ))}
                </Grid>
              )}

              {/* Test results */}
              {testResult && (
                <Box sx={{ mt: 2 }}>
                  <Divider sx={{ mb: 2 }} />
                  <Typography variant="subtitle2" gutterBottom>
                    Test Results
                  </Typography>
                  {testResult.error ? (
                    <Alert severity="error">{testResult.error}</Alert>
                  ) : testResult.type === "tool" &&
                    testResult.spec_validation ? (
                    <Alert
                      severity={
                        testResult.spec_validation.valid
                          ? "success"
                          : "error"
                      }
                    >
                      {testResult.spec_validation.valid
                        ? `Spec valid — ${testResult.spec_validation.extracted?.operations?.length || 0} operations`
                        : testResult.spec_validation.errors
                            ?.map((e) => `[${e.field}] ${e.message}`)
                            .join("; ")}
                    </Alert>
                  ) : testResult.type === "datasource" &&
                    testResult.connectivity ? (
                    <Alert
                      severity={
                        testResult.connectivity.embed_test_passed
                          ? "success"
                          : "error"
                      }
                    >
                      {testResult.connectivity.embed_test_passed
                        ? "Embedder connection successful"
                        : testResult.connectivity.embedder_error ||
                          testResult.connectivity.embed_test_error}
                    </Alert>
                  ) : (
                    <Alert severity="info">
                      {JSON.stringify(testResult)}
                    </Alert>
                  )}
                </Box>
              )}
            </Paper>

            {/* Activity audit trail */}
            {activities.length > 0 && (
              <Paper sx={{ p: 3, mb: 3 }}>
                <Typography variant="h6" gutterBottom>
                  Review History
                </Typography>
                {activities.map((activity) => (
                  <Box
                    key={activity.id || activity.ID}
                    sx={{
                      py: 1.5,
                      borderBottom: "1px solid rgba(0,0,0,0.08)",
                      "&:last-child": { borderBottom: "none" },
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                      <Chip
                        label={activity.activity_type?.replace("_", " ")}
                        size="small"
                        sx={{ textTransform: "capitalize", fontWeight: "bold" }}
                        color={
                          activity.activity_type === "approved" ? "success" :
                          activity.activity_type === "rejected" ? "error" :
                          activity.activity_type === "changes_requested" ? "warning" :
                          "default"
                        }
                      />
                      <Typography variant="caption" color="text.secondary">
                        {activity.actor_name || `User #${activity.actor_id}`} —{" "}
                        {new Date(activity.created_at || activity.CreatedAt).toLocaleString()}
                      </Typography>
                    </Box>
                    {activity.feedback && (
                      <Alert severity="info" variant="outlined" sx={{ mt: 0.5, py: 0 }}>
                        <Typography variant="body2">
                          <strong>Feedback:</strong> {activity.feedback}
                        </Typography>
                      </Alert>
                    )}
                    {activity.internal_note && (
                      <Alert severity="warning" variant="outlined" sx={{ mt: 0.5, py: 0 }}>
                        <Typography variant="body2">
                          <strong>Internal note:</strong> {activity.internal_note}
                        </Typography>
                      </Alert>
                    )}
                  </Box>
                ))}
              </Paper>
            )}

            {/* Version history */}
            {versions.length > 0 && (
              <Paper sx={{ p: 3 }}>
                <Typography variant="h6" gutterBottom>
                  Version History
                </Typography>
                {versions.map((v) => (
                  <Box
                    key={v.ID}
                    sx={{
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      py: 1,
                      borderBottom: "1px solid rgba(0,0,0,0.08)",
                    }}
                  >
                    <Box>
                      <Typography variant="body2" fontWeight="bold">
                        Version {v.version_number}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {new Date(v.CreatedAt).toLocaleString()} —{" "}
                        {v.change_notes || "No notes"}
                      </Typography>
                    </Box>
                  </Box>
                ))}
              </Paper>
            )}
          </Grid>
        </Grid>

        {/* Action buttons */}
        {canReview && (
          <Box sx={{ mt: 3, display: "flex", gap: 2 }}>
            <PrimaryButton
              startIcon={<CheckCircleIcon />}
              onClick={() => setApproveDialogOpen(true)}
            >
              Approve
            </PrimaryButton>
            <PrimaryOutlineButton
              startIcon={<EditIcon />}
              onClick={() => setChangesDialogOpen(true)}
            >
              Request Changes
            </PrimaryOutlineButton>
            <Button
              variant="outlined"
              color="error"
              startIcon={<CancelIcon />}
              onClick={() => setRejectDialogOpen(true)}
            >
              Reject
            </Button>
          </Box>
        )}
      </ContentBox>

      {/* Approve Dialog */}
      <Dialog
        open={approveDialogOpen}
        onClose={() => setApproveDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Approve Submission</DialogTitle>
        <DialogContent>
          <Typography gutterBottom sx={{ mt: 1 }}>
            Set final privacy score: {finalPrivacyScore}
          </Typography>
          <Slider
            value={finalPrivacyScore}
            onChange={(e, val) => setFinalPrivacyScore(val)}
            min={0}
            max={100}
            valueLabelDisplay="auto"
          />
          <TextField
            fullWidth
            label="Review Notes (internal)"
            value={reviewNotes}
            onChange={(e) => setReviewNotes(e.target.value)}
            multiline
            rows={2}
            sx={{ mt: 2 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setApproveDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleApprove}
            variant="contained"
            color="success"
            disabled={actionLoading}
          >
            {actionLoading ? <CircularProgress size={20} /> : "Approve"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Reject Dialog */}
      <Dialog
        open={rejectDialogOpen}
        onClose={() => setRejectDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Reject Submission</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            label="Feedback for submitter"
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            multiline
            rows={3}
            required
            sx={{ mt: 1 }}
            helperText="This will be visible to the submitter"
          />
          <TextField
            fullWidth
            label="Internal notes (admin only)"
            value={reviewNotes}
            onChange={(e) => setReviewNotes(e.target.value)}
            multiline
            rows={2}
            sx={{ mt: 2 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRejectDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleReject}
            variant="contained"
            color="error"
            disabled={actionLoading || !feedback.trim()}
          >
            {actionLoading ? <CircularProgress size={20} /> : "Reject"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Request Changes Dialog */}
      <Dialog
        open={changesDialogOpen}
        onClose={() => setChangesDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Request Changes</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            label="Feedback for submitter"
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            multiline
            rows={3}
            required
            sx={{ mt: 1 }}
            helperText="Explain what needs to be changed"
          />
          <TextField
            fullWidth
            label="Internal notes (admin only)"
            value={reviewNotes}
            onChange={(e) => setReviewNotes(e.target.value)}
            multiline
            rows={2}
            sx={{ mt: 2 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setChangesDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleRequestChanges}
            variant="contained"
            color="warning"
            disabled={actionLoading || !feedback.trim()}
          >
            {actionLoading ? (
              <CircularProgress size={20} />
            ) : (
              "Request Changes"
            )}
          </Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default SubmissionReview;
