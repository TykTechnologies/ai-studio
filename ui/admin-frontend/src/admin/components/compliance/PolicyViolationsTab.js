import React, { useState, useCallback } from "react";
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Chip,
  Link,
  Grid,
  Collapse,
  IconButton,
  Tooltip,
  CircularProgress,
} from "@mui/material";
import { StyledTableHeaderCell, StyledTableCell, StyledTableRow, StyledPaper } from "../../styles/sharedStyles";
import { Link as RouterLink } from "react-router-dom";
import { MemoizedLineChart } from "../common/MemoizedChart";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import apiClient from "../../utils/apiClient";

const PolicyViolationsTab = ({ data, onAppClick, startDate, endDate }) => {
  // Track expanded rows and their loaded records
  const [expandedRows, setExpandedRows] = useState({});
  const [rowRecords, setRowRecords] = useState({});
  const [loadingRows, setLoadingRows] = useState({});

  const fetchRecordsForRow = useCallback(async (appId) => {
    if (!startDate || !endDate) return;

    const rowKey = `${appId}`;
    setLoadingRows(prev => ({ ...prev, [rowKey]: true }));

    try {
      const response = await apiClient.get("/compliance/violations", {
        params: {
          start_date: startDate,
          end_date: endDate,
          app_id: appId,
          limit: 25,
        },
      });
      const allRecords = response.data || [];
      setRowRecords(prev => ({ ...prev, [rowKey]: allRecords }));
    } catch (error) {
      console.error("Failed to fetch violation records:", error);
      setRowRecords(prev => ({ ...prev, [rowKey]: [] }));
    } finally {
      setLoadingRows(prev => ({ ...prev, [rowKey]: false }));
    }
  }, [startDate, endDate]);

  const handleRowExpand = (appId) => {
    const rowKey = `${appId}`;
    const isExpanded = expandedRows[rowKey];

    if (!isExpanded && !rowRecords[rowKey]) {
      // Fetch records when expanding for the first time
      fetchRecordsForRow(appId);
    }

    setExpandedRows(prev => ({ ...prev, [rowKey]: !isExpanded }));
  };

  if (!data) {
    return (
      <Box sx={{ p: 3, textAlign: "center" }}>
        <Typography color="text.secondary">No policy violations data available.</Typography>
      </Box>
    );
  }

  const chartData = {
    labels: data.timeline?.map((t) => t.date) || [],
    datasets: [
      {
        label: "Policy Violations",
        data: data.timeline?.map((t) => t.count) || [],
        borderColor: "rgb(255, 159, 64)",
        backgroundColor: "rgba(255, 159, 64, 0.5)",
        tension: 0.1,
      },
    ],
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: "top",
      },
    },
    scales: {
      y: {
        beginAtZero: true,
      },
    },
  };

  const getViolationTypeLabel = (type) => {
    switch (type) {
      case "filter_block":
        return "Filter Block";
      case "model_access":
        return "Model Access";
      case "privacy_score":
        return "Privacy Score";
      case "budget_exceeded":
        return "Budget Exceeded";
      case "auth_failure":
        return "Auth Failure";
      case "policy_violation":
        return "Policy Violation";
      default:
        return type;
    }
  };

  const getViolationTypeColor = (type) => {
    switch (type) {
      case "auth_failure":
        return "error";
      case "budget_exceeded":
        return "info";
      case "filter_block":
        return "warning";
      case "model_access":
        return "secondary";
      default:
        return "warning";
    }
  };

  // Count unique apps (not unique app+type combinations)
  const uniqueApps = new Set(data.filter_blocks?.map(v => v.app_id) || []).size;

  return (
    <Box>
      {/* Summary Stats */}
      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Grid item xs={6}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4" color="warning.main">
              {data.total_blocks || 0}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Total Blocked Requests
            </Typography>
          </StyledPaper>
        </Grid>
        <Grid item xs={6}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4">
              {uniqueApps}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Affected Applications
            </Typography>
          </StyledPaper>
        </Grid>
      </Grid>

      {/* Timeline Chart */}
      <StyledPaper sx={{ p: 2, mb: 3, height: 300 }}>
        <Typography variant="subtitle2" gutterBottom>
          Policy Violations Over Time
        </Typography>
        {data.timeline && data.timeline.length > 0 ? (
          <MemoizedLineChart options={chartOptions} data={chartData} />
        ) : (
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", height: "80%" }}>
            <Typography color="text.secondary">No timeline data available</Typography>
          </Box>
        )}
      </StyledPaper>

      {/* Violations Table with Expandable Rows */}
      <Typography variant="subtitle2" gutterBottom>
        Violations by Application
      </Typography>
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <StyledTableHeaderCell width={40}></StyledTableHeaderCell>
              <StyledTableHeaderCell>Application</StyledTableHeaderCell>
              <StyledTableHeaderCell>Violation Types</StyledTableHeaderCell>
              <StyledTableHeaderCell align="right">Count</StyledTableHeaderCell>
              <StyledTableHeaderCell>Last Occurred</StyledTableHeaderCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {data.filter_blocks && data.filter_blocks.length > 0 ? (
              data.filter_blocks.map((violation, idx) => {
                const rowKey = `${violation.app_id}`;
                const isExpanded = expandedRows[rowKey];
                const records = rowRecords[rowKey] || [];
                const isLoading = loadingRows[rowKey];

                return (
                  <React.Fragment key={idx}>
                    <StyledTableRow
                      sx={{
                        cursor: "pointer",
                        "& > *": { borderBottom: isExpanded ? "none" : undefined }
                      }}
                    >
                      <StyledTableCell>
                        <IconButton
                          size="small"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleRowExpand(violation.app_id);
                          }}
                        >
                          {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                        </IconButton>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Link
                          component={RouterLink}
                          to={`/admin/apps/${violation.app_id}`}
                          onClick={(e) => e.stopPropagation()}
                        >
                          {violation.app_name}
                        </Link>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                          {(violation.violation_types || []).map((type) => (
                            <Chip
                              key={type}
                              label={getViolationTypeLabel(type)}
                              color={getViolationTypeColor(type)}
                              size="small"
                              variant="outlined"
                            />
                          ))}
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <Typography fontWeight="bold">{violation.count}</Typography>
                      </StyledTableCell>
                      <StyledTableCell>
                        {new Date(violation.last_occurred).toLocaleString()}
                      </StyledTableCell>
                    </StyledTableRow>

                    {/* Expanded Detail Row */}
                    <TableRow>
                      <StyledTableCell colSpan={5} sx={{ py: 0, px: 0 }}>
                        <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                          <Box sx={{ py: 2, px: 4, bgcolor: "action.hover" }}>
                            <Typography variant="subtitle2" gutterBottom sx={{ mb: 2 }}>
                              Individual Violations
                            </Typography>
                            {isLoading ? (
                              <Box sx={{ display: "flex", alignItems: "center", gap: 1, py: 2 }}>
                                <CircularProgress size={16} />
                                <Typography variant="body2" color="text.secondary">
                                  Loading details...
                                </Typography>
                              </Box>
                            ) : records.length > 0 ? (
                              <Table size="small">
                                <TableHead>
                                  <TableRow>
                                    <StyledTableHeaderCell>Timestamp</StyledTableHeaderCell>
                                    <StyledTableHeaderCell>Type</StyledTableHeaderCell>
                                    <StyledTableHeaderCell>Filter</StyledTableHeaderCell>
                                    <StyledTableHeaderCell>Details</StyledTableHeaderCell>
                                    <StyledTableHeaderCell>Vendor</StyledTableHeaderCell>
                                  </TableRow>
                                </TableHead>
                                <TableBody>
                                  {records.map((record) => (
                                    <TableRow key={record.id}>
                                      <StyledTableCell>
                                        <Typography variant="body2">
                                          {new Date(record.timestamp).toLocaleString()}
                                        </Typography>
                                      </StyledTableCell>
                                      <StyledTableCell>
                                        <Chip
                                          label={getViolationTypeLabel(record.violation_type)}
                                          color={getViolationTypeColor(record.violation_type)}
                                          size="small"
                                          variant="outlined"
                                        />
                                      </StyledTableCell>
                                      <StyledTableCell>
                                        {record.filter_name ? (
                                          <Chip
                                            label={record.filter_name}
                                            size="small"
                                            color="primary"
                                            variant="outlined"
                                          />
                                        ) : (
                                          <Typography variant="body2" color="text.secondary">
                                            -
                                          </Typography>
                                        )}
                                      </StyledTableCell>
                                      <StyledTableCell>
                                        <Tooltip title={record.error_detail || "No details available"}>
                                          <Typography
                                            variant="body2"
                                            sx={{
                                              maxWidth: 350,
                                              overflow: "hidden",
                                              textOverflow: "ellipsis",
                                              whiteSpace: "nowrap",
                                            }}
                                          >
                                            {record.error_detail || "No details"}
                                          </Typography>
                                        </Tooltip>
                                      </StyledTableCell>
                                      <StyledTableCell>
                                        <Typography variant="body2">{record.vendor}</Typography>
                                      </StyledTableCell>
                                    </TableRow>
                                  ))}
                                </TableBody>
                              </Table>
                            ) : (
                              <Typography variant="body2" color="text.secondary">
                                No detailed records available
                              </Typography>
                            )}
                          </Box>
                        </Collapse>
                      </StyledTableCell>
                    </TableRow>
                  </React.Fragment>
                );
              })
            ) : (
              <StyledTableRow>
                <StyledTableCell colSpan={5} align="center">
                  <Typography color="text.secondary">No policy violations found</Typography>
                </StyledTableCell>
              </StyledTableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default PolicyViolationsTab;
