import React, { useCallback, useEffect, useMemo, useState } from "react";
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
import HistoryIcon from "@mui/icons-material/History";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import RefreshIcon from "@mui/icons-material/Refresh";
import apiClient from "../utils/apiClient";
import {
  TitleBox,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
import DateRangePicker from "../components/common/DateRangePicker";
import { isEnterpriseFeature, isPermissionDenied } from "../utils/apiErrors";

const METHODS = ["POST", "PUT", "PATCH", "DELETE", "GET"];
const STATUS_CLASSES = [
  { value: "2xx", label: "2xx Success" },
  { value: "3xx", label: "3xx Redirect" },
  { value: "4xx", label: "4xx Client error" },
  { value: "5xx", label: "5xx Server error" },
];

const isoDate = (d) => d.toISOString().split("T")[0];

const statusColor = (status) => {
  if (status >= 500) return "error";
  if (status >= 400) return "warning";
  if (status >= 300) return "info";
  return "success";
};

const methodColor = (method) => {
  switch (method) {
    case "DELETE":
      return "error";
    case "POST":
      return "success";
    case "PUT":
    case "PATCH":
      return "info";
    default:
      return "default";
  }
};

const formatTime = (iso) => {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
};

const formatValue = (v) => {
  if (v === null || v === undefined) return "—";
  if (typeof v === "object") return JSON.stringify(v, null, 2);
  return String(v);
};

// DiffTable renders {"field": {"old": x, "new": y}} as rows.
export const DiffTable = ({ diff }) => {
  if (!diff || typeof diff !== "object") return null;
  const fields = Object.keys(diff)
    .filter((k) => k !== "_truncated_fields")
    .sort();
  if (fields.length === 0) return null;
  return (
    <Box>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        Changed fields
      </Typography>
      <Table size="small" data-testid="audit-diff">
        <TableHead>
          <TableRow>
            <StyledTableHeaderCell>Field</StyledTableHeaderCell>
            <StyledTableHeaderCell>Before</StyledTableHeaderCell>
            <StyledTableHeaderCell>After</StyledTableHeaderCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {fields.map((field) => (
            <TableRow key={field}>
              <StyledTableCell sx={{ fontFamily: "monospace", whiteSpace: "nowrap" }}>
                {field}
              </StyledTableCell>
              <StyledTableCell>
                <Box component="pre" sx={preStyle}>
                  {formatValue(diff[field]?.old)}
                </Box>
              </StyledTableCell>
              <StyledTableCell>
                <Box component="pre" sx={preStyle}>
                  {formatValue(diff[field]?.new)}
                </Box>
              </StyledTableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {Array.isArray(diff._truncated_fields) && diff._truncated_fields.length > 0 && (
        <Typography variant="caption" color="text.secondary">
          Not shown (too large): {diff._truncated_fields.join(", ")}
        </Typography>
      )}
    </Box>
  );
};

const preStyle = {
  m: 0,
  fontSize: "0.75rem",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  maxHeight: 200,
  overflow: "auto",
};

const Detail = ({ label, value, mono }) => (
  <Grid item xs={12} sm={6} md={4}>
    <Typography variant="caption" color="text.secondary" display="block">
      {label}
    </Typography>
    <Typography
      variant="body2"
      sx={{ fontFamily: mono ? "monospace" : "inherit", wordBreak: "break-all" }}
    >
      {value || "—"}
    </Typography>
  </Grid>
);

const RecordDetails = ({ record, full }) => {
  const rec = full || record;
  return (
    <Box sx={{ p: 2, backgroundColor: "action.hover" }} data-testid="audit-record-details">
      <Grid container spacing={2} sx={{ mb: 2 }}>
        <Detail label="Request ID" value={rec.req_id} mono />
        <Detail label="Route" value={rec.route} mono />
        <Detail label="URL" value={rec.url} mono />
        <Detail label="Resource" value={rec.resource_type ? `${rec.resource_type} ${rec.resource_id || ""}`.trim() : ""} />
        <Detail label="Resource name" value={rec.resource_name} />
        <Detail label="User ID" value={rec.user_id ? String(rec.user_id) : ""} />
        <Detail label="User agent" value={rec.user_agent} />
        <Detail label="Duration" value={rec.duration_ms !== undefined ? `${rec.duration_ms} ms` : ""} />
        {rec.error && (
          <Grid item xs={12}>
            <Alert severity="warning" sx={{ py: 0 }}>
              {rec.error}
            </Alert>
          </Grid>
        )}
      </Grid>
      <DiffTable diff={rec.diff} />
      {full === undefined && (
        <Box sx={{ display: "flex", justifyContent: "center", p: 1 }}>
          <CircularProgress size={18} />
        </Box>
      )}
      {full && (full.request_dump || full.response_dump) && (
        <Grid container spacing={2} sx={{ mt: 1 }}>
          {full.request_dump && (
            <Grid item xs={12} md={6}>
              <Typography variant="subtitle2">Request</Typography>
              <Box component="pre" sx={{ ...preStyle, maxHeight: 320, p: 1, bgcolor: "background.paper" }}>
                {JSON.stringify(full.request_dump, null, 2)}
              </Box>
            </Grid>
          )}
          {full.response_dump && (
            <Grid item xs={12} md={6}>
              <Typography variant="subtitle2">Response</Typography>
              <Box component="pre" sx={{ ...preStyle, maxHeight: 320, p: 1, bgcolor: "background.paper" }}>
                {JSON.stringify(full.response_dump, null, 2)}
              </Box>
            </Grid>
          )}
        </Grid>
      )}
    </Box>
  );
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

const AuditTrail = () => {
  const [status, setStatus] = useState(null);
  const [statusError, setStatusError] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const [startDate, setStartDate] = useState(isoDate(new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)));
  const [endDate, setEndDate] = useState(isoDate(new Date()));
  const [draft, setDraft] = useState({
    user: "",
    action: "",
    resourceType: "",
    method: "",
    statusClass: "",
    search: "",
  });
  const [filters, setFilters] = useState(draft);

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [data, setData] = useState({ records: [], total: 0 });
  const [summary, setSummary] = useState(null);
  const [expanded, setExpanded] = useState(null);
  const [details, setDetails] = useState({});
  const [exporting, setExporting] = useState(false);

  const params = useMemo(() => {
    const p = { start_date: startDate, end_date: endDate };
    if (filters.user) p.user = filters.user;
    if (filters.action) p.action = filters.action;
    if (filters.resourceType) p.resource_type = filters.resourceType;
    if (filters.method) p.method = filters.method;
    if (filters.statusClass) p.status_class = filters.statusClass;
    if (filters.search) p.search = filters.search;
    return p;
  }, [startDate, endDate, filters]);

  useEffect(() => {
    let cancelled = false;
    apiClient
      .get("/audit/status")
      .then((res) => {
        if (!cancelled) setStatus(res.data);
      })
      .catch(() => {
        if (!cancelled) setStatusError("Failed to check audit trail availability");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const queryable = status?.available && status?.enabled && status?.store_type !== "file";

  const fetchAll = useCallback(async () => {
    if (!queryable) return;
    setLoading(true);
    setError(null);
    try {
      const [recordsRes, summaryRes] = await Promise.all([
        apiClient.get("/audit/records", {
          params: { ...params, page: page + 1, page_size: rowsPerPage },
        }),
        apiClient.get("/audit/summary", { params }),
      ]);
      setData({
        records: recordsRes.data.records || [],
        total: recordsRes.data.total || 0,
      });
      setSummary(summaryRes.data);
    } catch (err) {
      if (isPermissionDenied(err)) {
        setError("Your role does not include access to the audit trail");
      } else if (isEnterpriseFeature(err)) {
        setStatus((s) => ({ ...(s || {}), available: false }));
      } else {
        setError(err.response?.data?.errors?.[0]?.detail || "Failed to load audit records");
      }
    } finally {
      setLoading(false);
    }
  }, [queryable, params, page, rowsPerPage]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  const applyFilters = () => {
    setPage(0);
    setExpanded(null);
    setFilters(draft);
  };

  const clearFilters = () => {
    const empty = { user: "", action: "", resourceType: "", method: "", statusClass: "", search: "" };
    setDraft(empty);
    setFilters(empty);
    setPage(0);
  };

  const toggleExpand = async (rec) => {
    if (expanded === rec.id) {
      setExpanded(null);
      return;
    }
    setExpanded(rec.id);
    if (details[rec.id] === undefined) {
      try {
        const res = await apiClient.get(`/audit/records/${rec.id}`);
        setDetails((d) => ({ ...d, [rec.id]: res.data }));
      } catch (err) {
        setDetails((d) => ({ ...d, [rec.id]: null }));
      }
    }
  };

  const handleExport = async (format) => {
    setExporting(true);
    try {
      const res = await apiClient.get("/audit/export", {
        params: { ...params, format },
        responseType: "blob",
      });
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement("a");
      link.href = url;
      link.setAttribute("download", `audit_trail_${startDate}_${endDate}.${format}`);
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

  const header = (
    <TitleBox top="64px">
      <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
        <HistoryIcon sx={{ fontSize: 32 }} />
        <Typography variant="headingXLarge">Audit Trail</Typography>
        <Chip label="Enterprise" color="primary" size="small" />
      </Box>
      {queryable && (
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
          <Button
            variant="outlined"
            startIcon={<DownloadIcon />}
            onClick={() => handleExport("csv")}
            disabled={exporting || loading}
          >
            CSV
          </Button>
          <Button
            variant="outlined"
            startIcon={<DownloadIcon />}
            onClick={() => handleExport("json")}
            disabled={exporting || loading}
          >
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
        <Box sx={{ p: 3 }}>
          <Alert severity="info">
            <Typography variant="h6" gutterBottom>
              Enterprise Feature
            </Typography>
            <Typography>
              The Audit Trail is an Enterprise Edition feature that records every action taken
              through the management API so your security team can reconstruct incidents and
              trace the history of any object:
            </Typography>
            <ul>
              <li>Who did what, when, from which IP, with the HTTP status</li>
              <li>Field-level diffs for updates and the final state of deleted objects</li>
              <li>Login, logout, SSO and failed authentication events</li>
              <li>Filter by user, action, resource, status and date; export to CSV or JSON</li>
              <li>Configurable retention, file or database storage, optional full request/response capture</li>
            </ul>
            <Typography sx={{ mt: 2 }}>
              Visit{" "}
              <a href="https://tyk.io/ai-studio/pricing" target="_blank" rel="noopener noreferrer">
                tyk.io/ai-studio/pricing
              </a>{" "}
              for more information.
            </Typography>
          </Alert>
        </Box>
      </Box>
    );
  }

  if (!status.enabled) {
    return (
      <Box>
        {header}
        <Box sx={{ p: 3 }}>
          <Alert severity="warning">
            The audit trail is switched off. Set <code>AUDIT_ENABLED=true</code> on the AI Studio
            server to start recording.
          </Alert>
        </Box>
      </Box>
    );
  }

  if (status.store_type === "file") {
    return (
      <Box>
        {header}
        <Box sx={{ p: 3 }}>
          <Alert severity="info">
            The audit trail is configured to write to a file only (<code>AUDIT_STORE_TYPE=file</code>),
            so it cannot be queried here. Set <code>AUDIT_STORE_TYPE=db</code> or <code>both</code>{" "}
            to browse records in the dashboard.
          </Alert>
        </Box>
      </Box>
    );
  }

  const actionOptions = (summary?.by_action || []).map((a) => a.name);
  if (filters.action && !actionOptions.includes(filters.action)) actionOptions.unshift(filters.action);
  const resourceOptions = (summary?.by_resource_type || []).map((a) => a.name);
  if (filters.resourceType && !resourceOptions.includes(filters.resourceType)) {
    resourceOptions.unshift(filters.resourceType);
  }

  return (
    <Box>
      {header}
      <Box sx={{ p: 3 }}>
        {status.dropped > 0 && (
          <Alert severity="warning" sx={{ mb: 2 }}>
            {status.dropped} audit record(s) were dropped because the write queue was full. Consider
            raising <code>AUDIT_QUEUE_SIZE</code>.
          </Alert>
        )}

        <Grid container spacing={2} sx={{ mb: 3 }}>
          <SummaryTile label="Recorded actions" value={summary?.total} />
          <SummaryTile label="Failed (4xx/5xx)" value={summary?.failed} color="error.main" />
          <SummaryTile label="Distinct users" value={summary?.distinct_users} />
          <SummaryTile
            label="Retention"
            value={status.retention_days > 0 ? `${status.retention_days}d` : "∞"}
            color="text.secondary"
          />
        </Grid>

        <StyledPaper sx={{ p: 2, mb: 3 }}>
          <Grid container spacing={2} alignItems="center">
            <Grid item xs={12} sm={6} md={3}>
              <TextField
                fullWidth
                size="small"
                label="User"
                placeholder="email contains"
                value={draft.user}
                onChange={(e) => setDraft({ ...draft, user: e.target.value })}
                onKeyDown={(e) => e.key === "Enter" && applyFilters()}
              />
            </Grid>
            <Grid item xs={12} sm={6} md={3}>
              <TextField
                select
                fullWidth
                size="small"
                label="Action"
                value={draft.action}
                onChange={(e) => setDraft({ ...draft, action: e.target.value })}
              >
                <MenuItem value="">All actions</MenuItem>
                {actionOptions.map((a) => (
                  <MenuItem key={a} value={a}>
                    {a}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={12} sm={6} md={2}>
              <TextField
                select
                fullWidth
                size="small"
                label="Resource"
                value={draft.resourceType}
                onChange={(e) => setDraft({ ...draft, resourceType: e.target.value })}
              >
                <MenuItem value="">All resources</MenuItem>
                {resourceOptions.map((r) => (
                  <MenuItem key={r} value={r}>
                    {r}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={6} sm={3} md={2}>
              <TextField
                select
                fullWidth
                size="small"
                label="Method"
                value={draft.method}
                onChange={(e) => setDraft({ ...draft, method: e.target.value })}
              >
                <MenuItem value="">Any</MenuItem>
                {METHODS.map((m) => (
                  <MenuItem key={m} value={m}>
                    {m}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={6} sm={3} md={2}>
              <TextField
                select
                fullWidth
                size="small"
                label="Status"
                value={draft.statusClass}
                onChange={(e) => setDraft({ ...draft, statusClass: e.target.value })}
              >
                <MenuItem value="">Any</MenuItem>
                {STATUS_CLASSES.map((s) => (
                  <MenuItem key={s.value} value={s.value}>
                    {s.label}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={12} sm={8} md={8}>
              <TextField
                fullWidth
                size="small"
                label="Search"
                placeholder="action, URL, user, resource name, IP or request ID"
                value={draft.search}
                onChange={(e) => setDraft({ ...draft, search: e.target.value })}
                onKeyDown={(e) => e.key === "Enter" && applyFilters()}
              />
            </Grid>
            <Grid item xs={12} sm={4} md={4} sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
              <Button onClick={clearFilters} disabled={loading}>
                Clear
              </Button>
              <Button variant="contained" onClick={applyFilters} disabled={loading}>
                Apply filters
              </Button>
            </Grid>
          </Grid>
        </StyledPaper>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        <StyledPaper>
          <TableContainer>
            <Table size="small" aria-label="audit records">
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell sx={{ width: 40 }} />
                  <StyledTableHeaderCell>Time</StyledTableHeaderCell>
                  <StyledTableHeaderCell>User</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Action</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Resource</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Method</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell>IP</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <StyledTableCell colSpan={8} align="center">
                      <CircularProgress size={24} />
                    </StyledTableCell>
                  </TableRow>
                ) : data.records.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={8} align="center">
                      <Typography color="text.secondary" sx={{ py: 3 }}>
                        No audit records match these filters.
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  data.records.map((rec) => (
                    <React.Fragment key={rec.id}>
                      <StyledTableRow
                        hover
                        sx={{ cursor: "pointer" }}
                        onClick={() => toggleExpand(rec)}
                        data-testid={`audit-row-${rec.id}`}
                      >
                        <StyledTableCell>
                          <IconButton size="small" aria-label="expand">
                            {expanded === rec.id ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                          </IconButton>
                        </StyledTableCell>
                        <StyledTableCell sx={{ whiteSpace: "nowrap" }}>{formatTime(rec.timestamp)}</StyledTableCell>
                        <StyledTableCell>
                          {rec.user || <Typography variant="caption" color="text.secondary">anonymous</Typography>}
                        </StyledTableCell>
                        <StyledTableCell>{rec.action}</StyledTableCell>
                        <StyledTableCell>
                          {rec.resource_type && (
                            <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                              <Chip label={rec.resource_type} size="small" variant="outlined" />
                              <Typography variant="body2" noWrap sx={{ maxWidth: 240 }}>
                                {rec.resource_name || rec.resource_id}
                              </Typography>
                            </Box>
                          )}
                        </StyledTableCell>
                        <StyledTableCell>
                          <Chip label={rec.method} size="small" color={methodColor(rec.method)} />
                        </StyledTableCell>
                        <StyledTableCell>
                          <Chip label={rec.status} size="small" color={statusColor(rec.status)} />
                        </StyledTableCell>
                        <StyledTableCell sx={{ fontFamily: "monospace" }}>{rec.ip}</StyledTableCell>
                      </StyledTableRow>
                      <TableRow>
                        <StyledTableCell colSpan={8} sx={{ p: 0, border: 0 }}>
                          <Collapse in={expanded === rec.id} timeout="auto" unmountOnExit>
                            <RecordDetails record={rec} full={details[rec.id]} />
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

export default AuditTrail;
