import React, { useEffect, useState, useCallback } from "react";
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Chip,
  Grid,
  CircularProgress,
  TextField,
  MenuItem,
  Tooltip,
  TablePagination,
  Collapse,
  IconButton,
  Link,
} from "@mui/material";
import {
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
  StyledPaper,
} from "../../styles/sharedStyles";
import { Link as RouterLink } from "react-router-dom";
import { MemoizedLineChart } from "../common/MemoizedChart";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import apiClient from "../../utils/apiClient";

const SEVERITY_COLORS = {
  critical: "error",
  warning: "warning",
  info: "info",
};

const FilterEventsTab = ({ startDate, endDate }) => {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  const [severity, setSeverity] = useState("");
  const [eventType, setEventType] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [expanded, setExpanded] = useState({});

  const fetchEvents = useCallback(async () => {
    if (!startDate || !endDate) return;
    setLoading(true);
    setError(null);
    try {
      const params = {
        start_date: startDate,
        end_date: endDate,
        limit: rowsPerPage,
        offset: page * rowsPerPage,
      };
      if (severity) params.severity = severity;
      if (eventType) params.event_type = eventType;

      const response = await apiClient.get("/compliance/events", { params });
      setData(response.data);
    } catch (err) {
      setError("Failed to load filter events");
    } finally {
      setLoading(false);
    }
  }, [startDate, endDate, severity, eventType, page, rowsPerPage]);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  // Reset to first page when filters change.
  useEffect(() => {
    setPage(0);
  }, [severity, eventType, startDate, endDate]);

  const totals = data?.by_severity || {};
  const totalCount = data?.total_count || 0;

  const chartData = {
    labels: (data?.timeline || []).map((t) => t.date),
    datasets: [
      {
        label: "Compliance Events",
        data: (data?.timeline || []).map((t) => t.count),
        borderColor: "rgb(75, 192, 192)",
        backgroundColor: "rgba(75, 192, 192, 0.5)",
        tension: 0.1,
      },
    ],
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: { legend: { position: "top" } },
    scales: { y: { beginAtZero: true } },
  };

  const eventTypeOptions = Object.keys(data?.by_type || {}).sort();

  return (
    <Box>
      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Grid item xs={12} sm={3}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4">{totalCount}</Typography>
            <Typography variant="body2" color="text.secondary">
              Total Events
            </Typography>
          </StyledPaper>
        </Grid>
        <Grid item xs={12} sm={3}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4" color="error.main">
              {totals.critical || 0}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Critical
            </Typography>
          </StyledPaper>
        </Grid>
        <Grid item xs={12} sm={3}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4" color="warning.main">
              {totals.warning || 0}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Warning
            </Typography>
          </StyledPaper>
        </Grid>
        <Grid item xs={12} sm={3}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4" color="info.main">
              {totals.info || 0}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Info
            </Typography>
          </StyledPaper>
        </Grid>
      </Grid>

      <StyledPaper sx={{ p: 2, mb: 3, height: 280 }}>
        <Typography variant="subtitle2" gutterBottom>
          Filter Events Over Time
        </Typography>
        {data?.timeline && data.timeline.length > 0 ? (
          <MemoizedLineChart options={chartOptions} data={chartData} />
        ) : (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              height: "80%",
            }}
          >
            <Typography color="text.secondary">
              No timeline data available
            </Typography>
          </Box>
        )}
      </StyledPaper>

      <Box sx={{ display: "flex", gap: 2, mb: 2, flexWrap: "wrap" }}>
        <TextField
          select
          label="Severity"
          size="small"
          value={severity}
          onChange={(e) => setSeverity(e.target.value)}
          sx={{ minWidth: 160 }}
        >
          <MenuItem value="">All</MenuItem>
          <MenuItem value="critical">Critical</MenuItem>
          <MenuItem value="warning">Warning</MenuItem>
          <MenuItem value="info">Info</MenuItem>
        </TextField>
        <TextField
          select
          label="Event Type"
          size="small"
          value={eventType}
          onChange={(e) => setEventType(e.target.value)}
          sx={{ minWidth: 220 }}
        >
          <MenuItem value="">All</MenuItem>
          {eventTypeOptions.map((t) => (
            <MenuItem key={t} value={t}>
              {t} ({data.by_type[t]})
            </MenuItem>
          ))}
        </TextField>
      </Box>

      {error && (
        <Typography color="error" sx={{ mb: 2 }}>
          {error}
        </Typography>
      )}

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <StyledTableHeaderCell width={40}></StyledTableHeaderCell>
              <StyledTableHeaderCell>Timestamp</StyledTableHeaderCell>
              <StyledTableHeaderCell>Application</StyledTableHeaderCell>
              <StyledTableHeaderCell>Filter</StyledTableHeaderCell>
              <StyledTableHeaderCell>Scope</StyledTableHeaderCell>
              <StyledTableHeaderCell>Event Type</StyledTableHeaderCell>
              <StyledTableHeaderCell>Severity</StyledTableHeaderCell>
              <StyledTableHeaderCell>Description</StyledTableHeaderCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading ? (
              <StyledTableRow>
                <StyledTableCell colSpan={8} align="center">
                  <Box sx={{ display: "flex", justifyContent: "center", py: 3 }}>
                    <CircularProgress size={24} />
                  </Box>
                </StyledTableCell>
              </StyledTableRow>
            ) : data?.events && data.events.length > 0 ? (
              data.events.map((e) => {
                const hasMeta = e.metadata && Object.keys(e.metadata).length > 0;
                const isExpanded = !!expanded[e.id];
                return (
                  <React.Fragment key={e.id}>
                    <StyledTableRow>
                      <StyledTableCell>
                        {hasMeta && (
                          <IconButton
                            size="small"
                            onClick={() =>
                              setExpanded((prev) => ({
                                ...prev,
                                [e.id]: !prev[e.id],
                              }))
                            }
                          >
                            {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                          </IconButton>
                        )}
                      </StyledTableCell>
                      <StyledTableCell>
                        <Typography variant="body2">
                          {new Date(e.timestamp).toLocaleString()}
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell>
                        {e.app_id ? (
                          <Link
                            component={RouterLink}
                            to={`/admin/apps/${e.app_id}`}
                          >
                            {e.app_name || `App ${e.app_id}`}
                          </Link>
                        ) : (
                          <Typography color="text.secondary">-</Typography>
                        )}
                      </StyledTableCell>
                      <StyledTableCell>{e.filter_name || "-"}</StyledTableCell>
                      <StyledTableCell>
                        <Typography variant="caption" color="text.secondary">
                          {e.filter_scope}
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Chip
                          label={e.event_type}
                          size="small"
                          variant="outlined"
                        />
                      </StyledTableCell>
                      <StyledTableCell>
                        <Chip
                          label={e.severity}
                          size="small"
                          color={SEVERITY_COLORS[e.severity] || "default"}
                        />
                      </StyledTableCell>
                      <StyledTableCell>
                        <Tooltip title={e.description || ""}>
                          <Typography
                            variant="body2"
                            sx={{
                              maxWidth: 320,
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                            }}
                          >
                            {e.description || "-"}
                          </Typography>
                        </Tooltip>
                      </StyledTableCell>
                    </StyledTableRow>
                    {hasMeta && (
                      <TableRow>
                        <StyledTableCell colSpan={8} sx={{ py: 0, px: 0 }}>
                          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                            <Box sx={{ px: 4, py: 2, bgcolor: "action.hover" }}>
                              <Typography variant="caption" color="text.secondary">
                                Metadata
                              </Typography>
                              <Box
                                component="pre"
                                sx={{
                                  m: 0,
                                  mt: 1,
                                  fontSize: "0.8rem",
                                  fontFamily: "monospace",
                                  whiteSpace: "pre-wrap",
                                  wordBreak: "break-all",
                                }}
                              >
                                {JSON.stringify(e.metadata, null, 2)}
                              </Box>
                            </Box>
                          </Collapse>
                        </StyledTableCell>
                      </TableRow>
                    )}
                  </React.Fragment>
                );
              })
            ) : (
              <StyledTableRow>
                <StyledTableCell colSpan={8} align="center">
                  <Typography color="text.secondary">
                    No filter events recorded in the selected period.
                  </Typography>
                </StyledTableCell>
              </StyledTableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <TablePagination
        component="div"
        count={totalCount}
        page={page}
        onPageChange={(_, p) => setPage(p)}
        rowsPerPage={rowsPerPage}
        onRowsPerPageChange={(e) => {
          setRowsPerPage(parseInt(e.target.value, 10));
          setPage(0);
        }}
        rowsPerPageOptions={[10, 25, 50, 100]}
      />
    </Box>
  );
};

export default FilterEventsTab;
