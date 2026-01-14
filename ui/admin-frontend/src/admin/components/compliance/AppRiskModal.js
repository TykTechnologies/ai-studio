import React, { useEffect, useState } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Typography,
  Box,
  Chip,
  Button,
  CircularProgress,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Grid,
  IconButton,
  LinearProgress,
} from "@mui/material";
import { StyledTableHeaderCell, StyledTableCell, StyledTableRow, StyledPaper } from "../../styles/sharedStyles";
import { Link as RouterLink } from "react-router-dom";
import CloseIcon from "@mui/icons-material/Close";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import apiClient from "../../utils/apiClient";

const AppRiskModal = ({ open, onClose, appId, startDate, endDate }) => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [profile, setProfile] = useState(null);

  useEffect(() => {
    if (open && appId) {
      fetchProfile();
    }
  }, [open, appId, startDate, endDate]);

  const fetchProfile = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await apiClient.get(`/compliance/app/${appId}/risk-profile`, {
        params: {
          start_date: startDate,
          end_date: endDate,
        },
      });
      setProfile(response.data);
    } catch (err) {
      setError("Failed to load app risk profile");
    } finally {
      setLoading(false);
    }
  };

  const getRiskChipColor = (level) => {
    switch (level) {
      case "HIGH":
        return "error";
      case "MEDIUM":
        return "warning";
      case "LOW":
        return "success";
      default:
        return "default";
    }
  };

  const getViolationTypeLabel = (type) => {
    switch (type) {
      case "auth_failure":
        return { label: "Auth Failure", color: "error" };
      case "policy_violation":
        return { label: "Policy Violation", color: "warning" };
      case "budget_exceeded":
        return { label: "Budget Exceeded", color: "error" };
      case "error":
        return { label: "Error", color: "default" };
      default:
        return { label: type, color: "default" };
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <Typography variant="h6">Application Risk Profile</Typography>
          <IconButton onClick={onClose} size="small">
            <CloseIcon />
          </IconButton>
        </Box>
      </DialogTitle>
      <DialogContent dividers>
        {loading ? (
          <Box sx={{ display: "flex", justifyContent: "center", p: 5 }}>
            <CircularProgress />
          </Box>
        ) : error ? (
          <Typography color="error">{error}</Typography>
        ) : profile ? (
          <Box>
            {/* Header */}
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", mb: 3 }}>
              <Box>
                <Typography variant="h5" gutterBottom>
                  {profile.app_name}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Owner: {profile.owner_email || "N/A"}
                </Typography>
              </Box>
              <Box sx={{ textAlign: "right" }}>
                <Chip
                  label={profile.risk_level}
                  color={getRiskChipColor(profile.risk_level)}
                  sx={{ mb: 1 }}
                />
                <Typography variant="h4" fontWeight="bold">
                  {profile.risk_score}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  Risk Score
                </Typography>
              </Box>
            </Box>

            {/* Summary Stats */}
            <Grid container spacing={2} sx={{ mb: 3 }}>
              <Grid item xs={3}>
                <StyledPaper sx={{ p: 2, textAlign: "center" }}>
                  <Typography variant="h5" color="error.main">
                    {profile.summary?.auth_failures || 0}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Auth Failures
                  </Typography>
                </StyledPaper>
              </Grid>
              <Grid item xs={3}>
                <StyledPaper sx={{ p: 2, textAlign: "center" }}>
                  <Typography variant="h5" color="warning.main">
                    {profile.summary?.policy_violations || 0}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Policy Violations
                  </Typography>
                </StyledPaper>
              </Grid>
              <Grid item xs={3}>
                <StyledPaper sx={{ p: 2, textAlign: "center" }}>
                  <Typography variant="h5">
                    {profile.summary?.error_rate?.toFixed(2) || 0}%
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Error Rate
                  </Typography>
                </StyledPaper>
              </Grid>
              <Grid item xs={3}>
                <StyledPaper sx={{ p: 2, textAlign: "center" }}>
                  <Typography variant="h5">
                    {profile.summary?.total_requests || 0}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Total Requests
                  </Typography>
                </StyledPaper>
              </Grid>
            </Grid>

            {/* Budget Status */}
            {profile.budget_status && (
              <StyledPaper sx={{ p: 2, mb: 3 }}>
                <Typography variant="subtitle2" gutterBottom>
                  Budget Status
                </Typography>
                <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
                  <Box sx={{ flexGrow: 1 }}>
                    <LinearProgress
                      variant="determinate"
                      value={Math.min(profile.budget_status.percentage, 100)}
                      color={
                        profile.budget_status.percentage >= 95
                          ? "error"
                          : profile.budget_status.percentage >= 80
                          ? "warning"
                          : "success"
                      }
                      sx={{ height: 10, borderRadius: 1 }}
                    />
                  </Box>
                  <Typography variant="body2" sx={{ minWidth: 100, textAlign: "right" }}>
                    ${profile.budget_status.spent.toFixed(2)} / ${profile.budget_status.budget.toFixed(2)}
                  </Typography>
                  <Typography variant="body2" fontWeight="bold">
                    ({profile.budget_status.percentage.toFixed(1)}%)
                  </Typography>
                </Box>
              </StyledPaper>
            )}

            {/* Recent Violations */}
            <Typography variant="subtitle2" gutterBottom>
              Recent Violations
            </Typography>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell>Timestamp</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Type</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Details</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {profile.recent_violations && profile.recent_violations.length > 0 ? (
                    profile.recent_violations.map((violation, idx) => {
                      const typeInfo = getViolationTypeLabel(violation.type);
                      return (
                        <StyledTableRow key={idx}>
                          <StyledTableCell>
                            {new Date(violation.timestamp).toLocaleString()}
                          </StyledTableCell>
                          <StyledTableCell>
                            <Chip
                              label={typeInfo.label}
                              color={typeInfo.color}
                              size="small"
                            />
                          </StyledTableCell>
                          <StyledTableCell>
                            <Typography variant="body2">{violation.details}</Typography>
                          </StyledTableCell>
                        </StyledTableRow>
                      );
                    })
                  ) : (
                    <StyledTableRow>
                      <StyledTableCell colSpan={3} align="center">
                        <Typography color="text.secondary">No recent violations</Typography>
                      </StyledTableCell>
                    </StyledTableRow>
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </Box>
        ) : (
          <Typography color="text.secondary">No data available</Typography>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
        {profile && (
          <Button
            component={RouterLink}
            to={`/admin/apps/${appId}`}
            variant="contained"
            endIcon={<OpenInNewIcon />}
          >
            View Application
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default AppRiskModal;
