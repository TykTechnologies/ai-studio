import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import pubClient from "../../admin/utils/pubClient";
import {
  Container,
  Typography,
  Tabs,
  Tab,
  Box,
  Grid,
  Card,
  CardContent,
  CardActions,
  IconButton,
  CircularProgress,
  Snackbar,
  Alert,
  Chip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Button,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import DeleteIcon from "@mui/icons-material/Delete";
import VisibilityIcon from "@mui/icons-material/Visibility";
import AddIcon from "@mui/icons-material/Add";
import StorageIcon from "@mui/icons-material/Storage";
import BuildIcon from "@mui/icons-material/Build";
import { PrimaryButton } from "../../admin/styles/sharedStyles";
import StatusChip from "../../admin/components/submissions/StatusChip";

const statusTabs = [
  { label: "All", value: "" },
  { label: "Published", value: "approved" },
  { label: "Pending Review", value: "submitted" },
  { label: "Changes Requested", value: "changes_requested" },
  { label: "Drafts", value: "draft" },
  { label: "Rejected", value: "rejected" },
];

const MyContributions = () => {
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState(0);
  const [submissions, setSubmissions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [totalCount, setTotalCount] = useState(0);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [submissionToDelete, setSubmissionToDelete] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  const fetchSubmissions = useCallback(async () => {
    try {
      setLoading(true);
      const status = statusTabs[activeTab].value;
      const params = { page_size: 50, page_number: 1 };
      if (status) params.status = status;

      const response = await pubClient.get("/common/submissions", { params });
      setSubmissions(response.data.data || []);
      setTotalCount(response.data.total_count || 0);
    } catch (error) {
      console.error("Error fetching submissions:", error);
      setSnackbar({
        open: true,
        message: "Failed to load submissions",
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [activeTab]);

  useEffect(() => {
    fetchSubmissions();
  }, [fetchSubmissions]);

  const handleTabChange = (event, newValue) => {
    setActiveTab(newValue);
  };

  const handleDelete = async () => {
    if (!submissionToDelete) return;
    try {
      await pubClient.delete(`/common/submissions/${submissionToDelete.id}`);
      setSnackbar({
        open: true,
        message: "Draft deleted",
        severity: "success",
      });
      setDeleteDialogOpen(false);
      setSubmissionToDelete(null);
      fetchSubmissions();
    } catch (error) {
      setSnackbar({
        open: true,
        message: error.response?.data?.errors?.[0]?.detail || "Failed to delete",
        severity: "error",
      });
    }
  };

  const getResourceName = (submission) => {
    return submission.resource_payload?.name || "Untitled";
  };

  const getResourceDescription = (submission) => {
    return (
      submission.resource_payload?.short_description ||
      submission.resource_payload?.description ||
      ""
    );
  };

  const canEdit = (status) =>
    status === "draft" || status === "changes_requested";
  const canDelete = (status) => status === "draft";

  return (
    <Container maxWidth={false} sx={{ px: 3, py: 3, width: "100%" }}>
      <Box
        sx={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          mb: 3,
        }}
      >
        <Typography variant="h4" component="h1">
          My Contributions
        </Typography>
        <PrimaryButton
          startIcon={<AddIcon />}
          onClick={() => navigate("/portal/submissions/new")}
        >
          Submit Resource
        </PrimaryButton>
      </Box>

      <Tabs
        value={activeTab}
        onChange={handleTabChange}
        sx={{ mb: 3, borderBottom: 1, borderColor: "divider" }}
      >
        {statusTabs.map((tab) => (
          <Tab key={tab.value} label={tab.label} />
        ))}
      </Tabs>

      {loading ? (
        <Box sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
          <CircularProgress />
        </Box>
      ) : submissions.length === 0 ? (
        <Box sx={{ textAlign: "center", mt: 4 }}>
          <Typography variant="h6" color="text.secondary" gutterBottom>
            No submissions found
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Share your data sources and tools with the community
          </Typography>
          <PrimaryButton
            startIcon={<AddIcon />}
            onClick={() => navigate("/portal/submissions/new")}
          >
            Submit Resource
          </PrimaryButton>
        </Box>
      ) : (
        <>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {totalCount} submission{totalCount !== 1 ? "s" : ""}
          </Typography>
          <Grid container spacing={3}>
            {submissions.map((submission) => (
              <Grid item xs={12} sm={6} md={4} key={submission.id}>
                <Card
                  sx={{
                    height: "100%",
                    display: "flex",
                    flexDirection: "column",
                    cursor: "pointer",
                    "&:hover": { boxShadow: 4 },
                  }}
                  onClick={() =>
                    navigate(`/portal/submissions/${submission.id}`)
                  }
                >
                  <CardContent sx={{ flexGrow: 1 }}>
                    <Box
                      sx={{
                        display: "flex",
                        justifyContent: "space-between",
                        alignItems: "flex-start",
                        mb: 1,
                      }}
                    >
                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        <Chip
                          icon={
                            submission.resource_type === "datasource" ? (
                              <StorageIcon />
                            ) : (
                              <BuildIcon />
                            )
                          }
                          label={
                            submission.resource_type === "datasource"
                              ? "Data Source"
                              : "Tool"
                          }
                          size="small"
                          variant="outlined"
                        />
                        {submission.is_update && (
                          <Chip
                            label="Update"
                            size="small"
                            sx={{
                              bgcolor: "#e3f2fd",
                              color: "#1565c0",
                              fontSize: "0.7rem",
                            }}
                          />
                        )}
                      </Box>
                      <StatusChip status={submission.status} />
                    </Box>

                    <Typography variant="h6" sx={{ mb: 0.5 }}>
                      {getResourceName(submission)}
                    </Typography>
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{
                        mb: 1,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        display: "-webkit-box",
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: "vertical",
                      }}
                    >
                      {getResourceDescription(submission)}
                    </Typography>

                    {submission.submitted_at && (
                      <Typography variant="caption" color="text.secondary">
                        Submitted{" "}
                        {new Date(submission.submitted_at).toLocaleDateString()}
                      </Typography>
                    )}

                    {submission.submitter_feedback &&
                      (submission.status === "rejected" ||
                        submission.status === "changes_requested") && (
                        <Alert severity="info" sx={{ mt: 1 }} variant="outlined">
                          <Typography variant="caption">
                            {submission.submitter_feedback}
                          </Typography>
                        </Alert>
                      )}
                  </CardContent>

                  <CardActions
                    sx={{ justifyContent: "flex-end", px: 2, pb: 1.5 }}
                  >
                    <IconButton
                      size="small"
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate(`/portal/submissions/${submission.id}`);
                      }}
                    >
                      <VisibilityIcon fontSize="small" />
                    </IconButton>
                    {canEdit(submission.status) && (
                      <IconButton
                        size="small"
                        onClick={(e) => {
                          e.stopPropagation();
                          navigate(
                            `/portal/submissions/edit/${submission.id}`
                          );
                        }}
                      >
                        <EditIcon fontSize="small" />
                      </IconButton>
                    )}
                    {canDelete(submission.status) && (
                      <IconButton
                        size="small"
                        color="error"
                        onClick={(e) => {
                          e.stopPropagation();
                          setSubmissionToDelete(submission);
                          setDeleteDialogOpen(true);
                        }}
                      >
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    )}
                  </CardActions>
                </Card>
              </Grid>
            ))}
          </Grid>
        </>
      )}

      <Dialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
      >
        <DialogTitle>Delete Draft</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to delete this draft submission? This cannot be
            undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDelete} color="error">
            Delete
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
    </Container>
  );
};

export default MyContributions;
