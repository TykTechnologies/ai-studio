import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Typography,
  Box,
  Table,
  TableBody,
  TableHead,
  TableRow,
  TableContainer,
  CircularProgress,
  Alert,
  Snackbar,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
} from "@mui/material";
import {
  TitleBox,
  ContentBox,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
import StatusChip from "../components/submissions/StatusChip";
import usePagination from "../hooks/usePagination";
import PaginationControls from "../components/common/PaginationControls";

const SubmissionReviewQueue = () => {
  const navigate = useNavigate();
  const [submissions, setSubmissions] = useState([]);
  const [statusCounts, setStatusCounts] = useState({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [typeFilter, setTypeFilter] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchSubmissions = useCallback(async () => {
    try {
      setLoading(true);
      const params = { page_size: pageSize, page_number: page };
      if (statusFilter) params.status = statusFilter;
      if (typeFilter) params.resource_type = typeFilter;

      const response = await apiClient.get("/submissions", { params });
      setSubmissions(response.data.data || []);
      setStatusCounts(response.data.status_counts || {});
      const totalCount = response.data.total_count || 0;
      const totalPages = response.data.total_pages || 0;
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (err) {
      console.error("Error fetching submissions:", err);
      setError("Failed to load submissions");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, statusFilter, typeFilter, updatePaginationData]);

  useEffect(() => {
    fetchSubmissions();
  }, [fetchSubmissions]);

  const getResourceName = (submission) => {
    return submission.resource_payload?.name || "Untitled";
  };

  const getWaitingTime = (submission) => {
    if (!submission.submitted_at) return "—";
    const submitted = new Date(submission.submitted_at);
    const now = new Date();
    const diffHours = Math.floor((now - submitted) / (1000 * 60 * 60));
    if (diffHours < 1) return "< 1 hour";
    if (diffHours < 24) return `${diffHours}h`;
    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays}d`;
  };

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Submission Queue</Typography>
      </TitleBox>

      <ContentBox sx={{ pt: 0 }}>
        {/* Status count chips */}
        <Box sx={{ display: "flex", gap: 1, mb: 3, mt: 2, flexWrap: "wrap" }}>
          {Object.entries(statusCounts).map(([status, count]) => (
            <Chip
              key={status}
              label={`${status.replace("_", " ")}: ${count}`}
              size="small"
              variant={statusFilter === status ? "filled" : "outlined"}
              color={statusFilter === status ? "primary" : "default"}
              onClick={() =>
                setStatusFilter(statusFilter === status ? "" : status)
              }
              sx={{ textTransform: "capitalize" }}
            />
          ))}
        </Box>

        {/* Filters */}
        <Box sx={{ display: "flex", gap: 2, mb: 3 }}>
          <FormControl size="small" sx={{ minWidth: 180 }}>
            <InputLabel>Status</InputLabel>
            <Select
              value={statusFilter}
              label="Status"
              onChange={(e) => setStatusFilter(e.target.value)}
            >
              <MenuItem value="">All Statuses</MenuItem>
              <MenuItem value="submitted">Pending Review</MenuItem>
              <MenuItem value="in_review">In Review</MenuItem>
              <MenuItem value="approved">Approved</MenuItem>
              <MenuItem value="rejected">Rejected</MenuItem>
              <MenuItem value="changes_requested">Changes Requested</MenuItem>
            </Select>
          </FormControl>
          <FormControl size="small" sx={{ minWidth: 180 }}>
            <InputLabel>Resource Type</InputLabel>
            <Select
              value={typeFilter}
              label="Resource Type"
              onChange={(e) => setTypeFilter(e.target.value)}
            >
              <MenuItem value="">All Types</MenuItem>
              <MenuItem value="datasource">Data Source</MenuItem>
              <MenuItem value="tool">Tool</MenuItem>
            </Select>
          </FormControl>
        </Box>

        {loading && <CircularProgress />}
        {error && <Alert severity="error">{error}</Alert>}
        {!loading && !error && (
          <TableContainer component={StyledPaper}>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Type</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Submitter</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Submitted</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Waiting</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {submissions.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={6} align="center">
                      <Typography color="text.secondary" sx={{ py: 3 }}>
                        No submissions found
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  submissions.map((submission) => (
                    <StyledTableRow
                      key={submission.id}
                      onClick={() =>
                        navigate(`/admin/submissions/${submission.id}`)
                      }
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>
                        <Chip
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
                            sx={{ ml: 0.5, fontSize: "0.65rem" }}
                            color="info"
                          />
                        )}
                      </StyledTableCell>
                      <StyledTableCell>
                        <Typography variant="body2" fontWeight="medium">
                          {getResourceName(submission)}
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell>
                        {submission.submitter?.name ||
                          submission.submitter?.email ||
                          `User #${submission.submitter_id}`}
                      </StyledTableCell>
                      <StyledTableCell>
                        <StatusChip status={submission.status} />
                      </StyledTableCell>
                      <StyledTableCell>
                        {submission.submitted_at
                          ? new Date(
                              submission.submitted_at
                            ).toLocaleDateString()
                          : "—"}
                      </StyledTableCell>
                      <StyledTableCell>
                        {submission.status === "submitted" ||
                        submission.status === "in_review"
                          ? getWaitingTime(submission)
                          : "—"}
                      </StyledTableCell>
                    </StyledTableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
        )}
        <PaginationControls
          page={page}
          pageSize={pageSize}
          totalPages={totalPages}
          onPageChange={handlePageChange}
          onPageSizeChange={handlePageSizeChange}
        />
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
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default SubmissionReviewQueue;
