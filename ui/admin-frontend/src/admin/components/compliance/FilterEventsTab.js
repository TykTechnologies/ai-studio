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
  Autocomplete,
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
import { eventSourceLabel } from "./eventSource";
import FlaggedText from "./FlaggedText";
import EventGuide from "./EventGuide";
import { JUDGE_EVENT_TYPE, agreementByFilter, judgeIndex } from "./judgeOpinions";

// JUDGE_SAMPLE is how many of the period's judge opinions the agreement
// summary reads.
export const JUDGE_SAMPLE = 500;

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
  // The Application filter: the chosen app ({id, name}) and the picker's
  // search, which loads matching apps only while the picker is open.
  const [app, setApp] = useState(null);
  const [appOpen, setAppOpen] = useState(false);
  const [appSearch, setAppSearch] = useState("");
  const [appOptions, setAppOptions] = useState([]);
  // The period's guardrail.judge events, for the agreement summary and the
  // opinions shown on flagged segments; loaded only when there are some.
  const [judgeEvents, setJudgeEvents] = useState([]);

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
      if (app) params.app_id = app.id;

      const response = await apiClient.get("/compliance/events", { params });
      setData(response.data);
    } catch (err) {
      setError("Failed to load filter events");
    } finally {
      setLoading(false);
    }
  }, [startDate, endDate, severity, eventType, app, page, rowsPerPage]);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  const judged = (data?.by_type || {})[JUDGE_EVENT_TYPE] > 0;
  useEffect(() => {
    if (!judged || !startDate || !endDate) {
      setJudgeEvents([]);
      return undefined;
    }
    let cancelled = false;
    const params = { start_date: startDate, end_date: endDate, event_type: JUDGE_EVENT_TYPE, limit: JUDGE_SAMPLE, offset: 0 };
    if (app) params.app_id = app.id;
    apiClient
      .get("/compliance/events", { params })
      .then((response) => {
        if (!cancelled) setJudgeEvents(Array.isArray(response?.data?.events) ? response.data.events : []);
      })
      .catch(() => {
        if (!cancelled) setJudgeEvents([]);
      });
    return () => {
      cancelled = true;
    };
  }, [judged, startDate, endDate, app]);

  const agreement = agreementByFilter(judgeEvents);
  const opinions = judgeIndex([...judgeEvents, ...(data?.events || [])]);

  // Reset to first page when filters change.
  useEffect(() => {
    setPage(0);
  }, [severity, eventType, app, startDate, endDate]);

  useEffect(() => {
    if (!appOpen) return undefined;
    let cancelled = false;
    const timer = setTimeout(async () => {
      try {
        const params = { page_size: 20 };
        if (appSearch) params.search = appSearch;
        const response = await apiClient.get("/apps", { params });
        if (cancelled) return;
        const rows = Array.isArray(response?.data?.data) ? response.data.data : [];
        setAppOptions(rows.map((a) => ({ id: Number(a.id), name: a.attributes?.name || `App ${a.id}` })));
      } catch (err) {
        if (!cancelled) setAppOptions([]);
      }
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [appOpen, appSearch]);

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
        <Autocomplete
          size="small"
          sx={{ minWidth: 240 }}
          open={appOpen}
          onOpen={() => setAppOpen(true)}
          onClose={() => setAppOpen(false)}
          options={appOptions}
          value={app}
          onChange={(_, v) => setApp(v)}
          onInputChange={(_, v, reason) => {
            if (reason === "input") setAppSearch(v);
            if (reason === "clear") setAppSearch("");
          }}
          getOptionLabel={(o) => o?.name || ""}
          isOptionEqualToValue={(o, v) => o.id === v.id}
          filterOptions={(x) => x}
          noOptionsText="No matching applications"
          renderInput={(params) => (
            <TextField {...params} label="Application" placeholder="All" inputProps={{ ...params.inputProps, "data-testid": "app-filter-input" }} />
          )}
        />
      </Box>

      {error && (
        <Typography color="error" sx={{ mb: 2 }}>
          {error}
        </Typography>
      )}

      {agreement.length > 0 && (
        <StyledPaper sx={{ p: 2, mb: 2 }} data-testid="judge-agreement">
          <Typography variant="subtitle2" gutterBottom>
            Second opinion (LLM judge)
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            How often the judge found harmless what a filter blocked or quarantined
            {judgeEvents.length >= JUDGE_SAMPLE ? ` (latest ${JUDGE_SAMPLE} opinions)` : ""}. Look at these before letting
            a judge soften blocks.
          </Typography>
          {agreement.map((row) => (
            <Typography variant="body2" key={row.filter} data-testid={`judge-agreement-${row.filter}`}>
              <Box component="span" sx={{ fontWeight: 500 }}>
                {row.filter}
              </Box>
              {`: judge said benign on ${row.benign} of ${row.judged} flags`}
              {row.benign > row.benignFull ? ` (${row.benign - row.benignFull} not shown in full)` : ""}
              {row.noOpinion > 0 ? `; no opinion on ${row.noOpinion}` : ""}
            </Typography>
          ))}
        </StyledPaper>
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
              <StyledTableHeaderCell>Source</StyledTableHeaderCell>
              <StyledTableHeaderCell>Event Type</StyledTableHeaderCell>
              <StyledTableHeaderCell>Severity</StyledTableHeaderCell>
              <StyledTableHeaderCell>Description</StyledTableHeaderCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading ? (
              <StyledTableRow>
                <StyledTableCell colSpan={9} align="center">
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
                      <StyledTableCell data-testid={`event-source-${e.id}`}>
                        <Typography variant="body2">{eventSourceLabel(e.metadata) || "-"}</Typography>
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
                        <StyledTableCell colSpan={9} sx={{ py: 0, px: 0 }}>
                          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                            <Box sx={{ px: 4, py: 2, bgcolor: "action.hover" }}>
                              <EventGuide event={e} />
                              <FlaggedText metadata={e.metadata} appId={e.app_id} eventType={e.event_type} opinions={opinions} />
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
                <StyledTableCell colSpan={9} align="center">
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
