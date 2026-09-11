import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  Grid,
  IconButton,
  MenuItem,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import DownloadIcon from "@mui/icons-material/Download";
import RefreshIcon from "@mui/icons-material/Refresh";
import ReplayIcon from "@mui/icons-material/Replay";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import WebhookIcon from "@mui/icons-material/Webhook";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import apiClient from "../utils/apiClient";
import {
  TitleBox,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
import DateRangePicker from "../components/common/DateRangePicker";
import {
  DELIVERY_STATUSES,
  StatusChip,
  HealthPill,
  EnterpriseUpsell,
  DisabledNotice,
  BusWarning,
  formatTime,
  apiErrorDetail,
  humanStatus,
  preStyle,
} from "./webhookShared";

const isoDate = (d) => d.toISOString().split("T")[0];

const codeColor = (code) => {
  if (!code) return "default";
  if (code >= 500) return "error";
  if (code >= 400) return "warning";
  if (code >= 300) return "info";
  return "success";
};

const SummaryTile = ({ label, value, color }) => (
  <Grid item xs={6} sm={3}>
    <StyledPaper sx={{ p: 2, textAlign: "center" }}>
      <Typography variant="h4" color={color}>
        {value ?? 0}
      </Typography>
      <Typography variant="body2" color="text.secondary">
        {label}
      </Typography>
    </StyledPaper>
  </Grid>
);

const DeliveryDetails = ({ delivery, detail, onReplay, onCancel }) => {
  const d = detail?.delivery || delivery;
  const terminal = ["succeeded", "dead_lettered", "cancelled"].includes(d.status);
  const active = ["queued", "retrying"].includes(d.status);
  return (
    <Box sx={{ p: 2, backgroundColor: "action.hover" }} data-testid="delivery-details">
      <Grid container spacing={2}>
        <Grid item xs={12} md={6}>
          <Typography variant="caption" color="text.secondary" display="block">
            Delivery
          </Typography>
          <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
            {d.id}
          </Typography>
          <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 1 }}>
            Event
          </Typography>
          <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
            {d.event_id}
          </Typography>
          <Typography variant="body2" sx={{ mt: 1 }}>
            {d.kind} · {d.attempt_count}/{d.max_attempts} attempts
            {d.status === "retrying" && ` · next at ${formatTime(d.next_attempt_at)}`}
            {d.replay_of_id && ` · replay of ${d.replay_of_id}`}
          </Typography>
          {d.last_error && (
            <Alert severity="warning" sx={{ mt: 1, py: 0 }}>
              {d.last_error}
            </Alert>
          )}
        </Grid>
        <Grid item xs={12} md={6} sx={{ display: "flex", gap: 1, alignItems: "flex-start", justifyContent: "flex-end" }}>
          {terminal && (
            <Button size="small" variant="outlined" startIcon={<ReplayIcon />} onClick={() => onReplay(d)}>
              Replay
            </Button>
          )}
          {active && (
            <Button size="small" color="error" variant="outlined" onClick={() => onCancel(d)}>
              Cancel
            </Button>
          )}
        </Grid>
        {detail === undefined && (
          <Grid item xs={12} sx={{ textAlign: "center" }}>
            <CircularProgress size={18} />
          </Grid>
        )}
        {detail && (
          <>
            <Grid item xs={12}>
              <Typography variant="subtitle2">Attempts</Typography>
              <Table size="small" data-testid="delivery-attempts">
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell>#</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Started</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Outcome</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Code</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Latency</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Error / response</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {(detail.attempts || []).length === 0 && (
                    <TableRow>
                      <StyledTableCell colSpan={6}>
                        <Typography variant="body2" color="text.secondary">
                          No attempts yet.
                        </Typography>
                      </StyledTableCell>
                    </TableRow>
                  )}
                  {(detail.attempts || []).map((a) => (
                    <TableRow key={a.id}>
                      <StyledTableCell>{a.attempt}</StyledTableCell>
                      <StyledTableCell sx={{ whiteSpace: "nowrap" }}>{formatTime(a.started_at)}</StyledTableCell>
                      <StyledTableCell>{humanStatus(a.outcome)}</StyledTableCell>
                      <StyledTableCell>{a.status_code ? <Chip label={a.status_code} size="small" color={codeColor(a.status_code)} /> : "—"}</StyledTableCell>
                      <StyledTableCell>{a.duration_ms} ms</StyledTableCell>
                      <StyledTableCell sx={{ wordBreak: "break-word" }}>
                        {a.error && <div>{a.error}</div>}
                        {a.response_snippet && (
                          <Typography variant="caption" sx={{ fontFamily: "monospace" }}>
                            {a.response_snippet}
                          </Typography>
                        )}
                      </StyledTableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Grid>
            {detail.delivery?.rendered_payload && (
              <Grid item xs={12} md={6}>
                <Typography variant="subtitle2">Payload sent</Typography>
                <Box component="pre" sx={preStyle} data-testid="rendered-payload">
                  {detail.delivery.rendered_payload}
                </Box>
              </Grid>
            )}
            {detail.event?.payload && (
              <Grid item xs={12} md={6}>
                <Typography variant="subtitle2">Event (redacted)</Typography>
                <Box component="pre" sx={preStyle}>
                  {JSON.stringify(detail.event.payload, null, 2)}
                </Box>
              </Grid>
            )}
          </>
        )}
      </Grid>
    </Box>
  );
};

const WebhookDeliveries = () => {
  const navigate = useNavigate();
  const [status, setStatus] = useState(null);
  const [statusError, setStatusError] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);

  const [targets, setTargets] = useState([]);
  const [startDate, setStartDate] = useState(isoDate(new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)));
  const [endDate, setEndDate] = useState(isoDate(new Date()));
  const [draft, setDraft] = useState({ targetId: "", topic: "", status: "", kind: "", search: "" });
  const [filters, setFilters] = useState(draft);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [data, setData] = useState({ deliveries: [], total: 0 });
  const [stats, setStats] = useState(null);
  const [expanded, setExpanded] = useState(null);
  const [details, setDetails] = useState({});
  const [exporting, setExporting] = useState(false);

  const params = useMemo(() => {
    const p = { start_date: startDate, end_date: endDate };
    if (filters.targetId) p.target_id = filters.targetId;
    if (filters.topic) p.topic = filters.topic;
    if (filters.status) p.status = filters.status;
    if (filters.kind) p.kind = filters.kind;
    if (filters.search) p.search = filters.search;
    return p;
  }, [startDate, endDate, filters]);

  useEffect(() => {
    let cancelled = false;
    apiClient
      .get("/webhooks/status")
      .then((res) => {
        if (!cancelled) setStatus(res.data);
      })
      .catch(() => {
        if (!cancelled) setStatusError("Failed to check webhook availability");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const usable = status?.available && status?.enabled;

  useEffect(() => {
    if (!usable) return;
    apiClient
      .get("/webhooks/targets")
      .then((res) => setTargets(res.data.targets || []))
      .catch(() => {});
  }, [usable]);

  const fetchAll = useCallback(async () => {
    if (!usable) return;
    setLoading(true);
    setError(null);
    try {
      const [list, st] = await Promise.all([
        apiClient.get("/webhooks/deliveries", { params: { ...params, page: page + 1, page_size: rowsPerPage } }),
        apiClient.get("/webhooks/stats", { params: { window: "24h" } }),
      ]);
      setData({ deliveries: list.data.deliveries || [], total: list.data.total || 0 });
      setStats(st.data);
    } catch (err) {
      if (err.response?.status === 403) {
        setStatus((s) => ({ ...(s || {}), available: false }));
      } else {
        setError(apiErrorDetail(err, "Failed to load deliveries"));
      }
    } finally {
      setLoading(false);
    }
  }, [usable, params, page, rowsPerPage]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  const applyFilters = () => {
    setPage(0);
    setExpanded(null);
    setFilters(draft);
  };

  const clearFilters = () => {
    const empty = { targetId: "", topic: "", status: "", kind: "", search: "" };
    setDraft(empty);
    setFilters(empty);
    setPage(0);
  };

  const showDeadLetters = () => {
    const next = { ...draft, status: "dead_lettered" };
    setDraft(next);
    setFilters(next);
    setPage(0);
  };

  const toggleExpand = async (d) => {
    if (expanded === d.id) {
      setExpanded(null);
      return;
    }
    setExpanded(d.id);
    if (details[d.id] === undefined) {
      try {
        const res = await apiClient.get(`/webhooks/deliveries/${d.id}`);
        setDetails((x) => ({ ...x, [d.id]: res.data }));
      } catch (err) {
        setDetails((x) => ({ ...x, [d.id]: null }));
      }
    }
  };

  const replay = async (d) => {
    setError(null);
    try {
      const res = await apiClient.post(`/webhooks/deliveries/${d.id}/replay`, {});
      setNotice(`Replay queued as delivery ${res.data.delivery_id}`);
      fetchAll();
    } catch (err) {
      setError(apiErrorDetail(err, "Replay failed"));
    }
  };

  const cancel = async (d) => {
    setError(null);
    try {
      await apiClient.post(`/webhooks/deliveries/${d.id}/cancel`, {});
      setNotice("Delivery cancelled");
      setDetails((x) => ({ ...x, [d.id]: undefined }));
      fetchAll();
    } catch (err) {
      setError(apiErrorDetail(err, "Cancel failed"));
    }
  };

  const replayAllDeadLetters = async () => {
    setError(null);
    try {
      const body = { max: 500 };
      if (filters.targetId) body.target_id = filters.targetId;
      const res = await apiClient.post("/webhooks/deliveries/replay", body);
      setNotice(`Replayed ${res.data.replayed} dead letter(s), skipped ${res.data.skipped}`);
      fetchAll();
    } catch (err) {
      setError(apiErrorDetail(err, "Bulk replay failed"));
    }
  };

  const handleExport = async (format) => {
    setExporting(true);
    try {
      const res = await apiClient.get("/webhooks/deliveries/export", {
        params: { ...params, format },
        responseType: "blob",
      });
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement("a");
      link.href = url;
      link.setAttribute("download", `webhook_deliveries_${startDate}_${endDate}.${format}`);
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.URL.revokeObjectURL(url);
    } catch (err) {
      setError("Export failed");
    } finally {
      setExporting(false);
    }
  };

  const targetName = (id) => targets.find((t) => t.id === id)?.name;

  const header = (
    <TitleBox top="64px">
      <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
        <IconButton onClick={() => navigate("/admin/webhooks")} aria-label="back to targets">
          <ArrowBackIcon />
        </IconButton>
        <WebhookIcon sx={{ fontSize: 32 }} />
        <Typography variant="headingXLarge">Webhook Deliveries</Typography>
        <Chip label="Enterprise" color="primary" size="small" />
        {usable && <HealthPill status={status} />}
      </Box>
      {usable && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <DateRangePicker
            startDate={startDate}
            endDate={endDate}
            onStartDateChange={(v) => {
              setStartDate(v);
              setPage(0);
            }}
            onEndDateChange={(v) => {
              setEndDate(v);
              setPage(0);
            }}
            updateMode="immediate"
            label=""
          />
          <Tooltip title="Refresh">
            <IconButton onClick={fetchAll} disabled={loading} aria-label="refresh">
              <RefreshIcon />
            </IconButton>
          </Tooltip>
          <Button variant="outlined" startIcon={<DownloadIcon />} onClick={() => handleExport("csv")} disabled={exporting || loading}>
            CSV
          </Button>
          <Button variant="outlined" startIcon={<DownloadIcon />} onClick={() => handleExport("json")} disabled={exporting || loading}>
            JSON
          </Button>
        </Box>
      )}
    </TitleBox>
  );

  if (loading && status === null) {
    return (
      <Box>
        {header}
        <Box sx={{ display: "flex", justifyContent: "center", p: 5 }}>
          <CircularProgress />
        </Box>
      </Box>
    );
  }
  if (statusError) {
    return (
      <Box>
        {header}
        <Box sx={{ p: 3 }}>
          <Alert severity="error">{statusError}</Alert>
        </Box>
      </Box>
    );
  }
  if (!status?.available) {
    return (
      <Box>
        {header}
        <EnterpriseUpsell />
      </Box>
    );
  }
  if (!status.enabled) {
    return (
      <Box>
        {header}
        <DisabledNotice />
      </Box>
    );
  }

  const by = stats?.by_status || {};

  return (
    <Box>
      {header}
      <Box sx={{ p: 3 }}>
        <BusWarning status={status} />
        {status.dropped_events > 0 && (
          <Alert severity="warning" sx={{ mb: 2 }}>
            {status.dropped_events} event(s) could not be persisted on this node and were dropped. Check
            database health; those events were not delivered.
          </Alert>
        )}
        {notice && (
          <Alert severity="success" sx={{ mb: 2 }} onClose={() => setNotice(null)}>
            {notice}
          </Alert>
        )}
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        <Grid container spacing={2} sx={{ mb: 3 }}>
          <SummaryTile label="Succeeded (24h)" value={by.succeeded} color="success.main" />
          <SummaryTile label="Dead-lettered (24h)" value={by.dead_lettered} color="error.main" />
          <SummaryTile label="Waiting" value={stats?.queue_depth} />
          <SummaryTile label="In flight" value={by.in_flight} />
        </Grid>

        <StyledPaper sx={{ p: 2, mb: 3 }}>
          <Grid container spacing={2} alignItems="center">
            <Grid item xs={12} sm={6} md={3}>
              <TextField
                select
                fullWidth
                size="small"
                label="Target"
                value={draft.targetId}
                onChange={(e) => setDraft({ ...draft, targetId: e.target.value })}
              >
                <MenuItem value="">All targets</MenuItem>
                {targets.map((t) => (
                  <MenuItem key={t.id} value={t.id}>
                    {t.name}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={12} sm={6} md={3}>
              <TextField
                fullWidth
                size="small"
                label="Topic"
                placeholder="system.llm.created"
                value={draft.topic}
                onChange={(e) => setDraft({ ...draft, topic: e.target.value })}
                onKeyDown={(e) => e.key === "Enter" && applyFilters()}
              />
            </Grid>
            <Grid item xs={6} sm={3} md={2}>
              <TextField
                select
                fullWidth
                size="small"
                label="Status"
                value={draft.status}
                onChange={(e) => setDraft({ ...draft, status: e.target.value })}
              >
                <MenuItem value="">Any</MenuItem>
                {DELIVERY_STATUSES.map((s) => (
                  <MenuItem key={s} value={s}>
                    {humanStatus(s)}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={6} sm={3} md={2}>
              <TextField
                select
                fullWidth
                size="small"
                label="Kind"
                value={draft.kind}
                onChange={(e) => setDraft({ ...draft, kind: e.target.value })}
              >
                <MenuItem value="">Any</MenuItem>
                <MenuItem value="event">event</MenuItem>
                <MenuItem value="test">test</MenuItem>
                <MenuItem value="replay">replay</MenuItem>
              </TextField>
            </Grid>
            <Grid item xs={12} sm={8} md={8}>
              <TextField
                fullWidth
                size="small"
                label="Search"
                placeholder="delivery or event ID, target URL, topic, error, response"
                value={draft.search}
                onChange={(e) => setDraft({ ...draft, search: e.target.value })}
                onKeyDown={(e) => e.key === "Enter" && applyFilters()}
              />
            </Grid>
            <Grid item xs={12} sm={4} md={4} sx={{ display: "flex", gap: 1, justifyContent: "flex-end", flexWrap: "wrap" }}>
              <Button onClick={showDeadLetters} color="error" disabled={loading}>
                Dead letters
              </Button>
              <Button onClick={clearFilters} disabled={loading}>
                Clear
              </Button>
              <Button variant="contained" onClick={applyFilters} disabled={loading}>
                Apply filters
              </Button>
            </Grid>
            {filters.status === "dead_lettered" && (
              <Grid item xs={12} sx={{ display: "flex", justifyContent: "flex-end" }}>
                <Button variant="outlined" color="error" startIcon={<ReplayIcon />} onClick={replayAllDeadLetters} disabled={loading}>
                  Replay all dead letters{filters.targetId ? ` for ${targetName(filters.targetId) || "target"}` : ""}
                </Button>
              </Grid>
            )}
          </Grid>
        </StyledPaper>

        <StyledPaper>
          <TableContainer>
            <Table size="small" aria-label="webhook deliveries">
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell sx={{ width: 40 }} />
                  <StyledTableHeaderCell>Time</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Topic</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Target</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Attempts</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Last code</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Last error</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <StyledTableCell colSpan={8} align="center">
                      <CircularProgress size={24} />
                    </StyledTableCell>
                  </TableRow>
                ) : data.deliveries.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={8} align="center">
                      <Typography color="text.secondary" sx={{ py: 3 }}>
                        No deliveries match these filters.
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  data.deliveries.map((d) => (
                    <React.Fragment key={d.id}>
                      <StyledTableRow
                        hover
                        sx={{ cursor: "pointer" }}
                        onClick={() => toggleExpand(d)}
                        data-testid={`delivery-row-${d.id}`}
                      >
                        <StyledTableCell>
                          <IconButton size="small" aria-label="expand">
                            {expanded === d.id ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                          </IconButton>
                        </StyledTableCell>
                        <StyledTableCell sx={{ whiteSpace: "nowrap" }}>{formatTime(d.created_at)}</StyledTableCell>
                        <StyledTableCell sx={{ fontFamily: "monospace" }}>{d.topic}</StyledTableCell>
                        <StyledTableCell>
                          <Typography variant="body2">{targetName(d.target_id) || "(deleted)"}</Typography>
                          <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
                            {d.target_url}
                          </Typography>
                        </StyledTableCell>
                        <StyledTableCell>
                          <StatusChip status={d.status} />
                        </StyledTableCell>
                        <StyledTableCell>
                          {d.attempt_count}/{d.max_attempts}
                        </StyledTableCell>
                        <StyledTableCell>
                          {d.last_status_code ? <Chip label={d.last_status_code} size="small" color={codeColor(d.last_status_code)} /> : "—"}
                        </StyledTableCell>
                        <StyledTableCell sx={{ maxWidth: 320 }}>
                          <Typography variant="body2" noWrap title={d.last_error}>
                            {d.last_error}
                          </Typography>
                        </StyledTableCell>
                      </StyledTableRow>
                      <TableRow>
                        <StyledTableCell colSpan={8} sx={{ p: 0, border: 0 }}>
                          <Collapse in={expanded === d.id} timeout="auto" unmountOnExit>
                            <DeliveryDetails delivery={d} detail={details[d.id]} onReplay={replay} onCancel={cancel} />
                          </Collapse>
                        </StyledTableCell>
                      </TableRow>
                    </React.Fragment>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
          <TablePagination
            component="div"
            count={data.total}
            page={page}
            onPageChange={(_, p) => {
              setPage(p);
              setExpanded(null);
            }}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
            }}
            rowsPerPageOptions={[25, 50, 100, 250]}
          />
        </StyledPaper>
      </Box>
    </Box>
  );
};

export default WebhookDeliveries;
