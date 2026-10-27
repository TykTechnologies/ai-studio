import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  FormControlLabel,
  Grid,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { useDebounce } from "use-debounce";
import { TitleBox, ContentBox } from "../styles/sharedStyles";
import DataTable from "../components/common/DataTable";
import DateRangePicker from "../components/common/DateRangePicker";
import EnterpriseFeatureBadge from "../components/common/EnterpriseFeatureBadge";
import PermissionDeniedPanel from "../components/rbac/PermissionDeniedPanel";
import CorrelationQualityBar from "../components/agent-tracing/CorrelationQualityBar";
import CaptureSettingsDialog from "../components/agent-tracing/CaptureSettingsDialog";
import SessionUser from "../components/agent-tracing/SessionUser";
import {
  PAGE_SIZE,
  RANGES,
  emptyStateFilters,
  filtersFromSearchParams,
  filtersToSearchParams,
  formatCost,
  formatDuration,
  formatTokens,
  isSessionUUID,
  shortId,
  sortConfigOf,
  sortParamOf,
  toQueryBody,
} from "../components/agent-tracing/sessionFilters";
import agentTracingService from "../services/agentTracingService";
import apiClient from "../utils/apiClient";
import { listAll } from "../utils/listAll";
import { useEdition } from "../context/EditionContext";
import useSystemFeatures from "../hooks/useSystemFeatures";
import { isEnterpriseFeature, isPermissionDenied } from "../utils/apiErrors";
import { P } from "../rbac/permissions";
import { usePermissions } from "../context/PermissionsContext";

// Agent sessions list (spec §6.1; Enterprise). See sessionFilters.js for
// which filters may live in the URL.

// Without trace-content:read the end-user and external-id filters still
// match the raw id, but results show pseudonyms (jyx.174, option B).
const PSEUDONYM_FILTER_HINT = "Matches the exact id sent by the client; results show pseudonyms.";

const LIVE_REFRESH_MS = 5000;

const STATUS_COLOURS = { active: "info", closed: "default", error: "error" };

const LiveDot = () => (
  <Box
    component="span"
    aria-label="Active"
    data-testid="live-dot"
    sx={{
      display: "inline-block",
      width: 8,
      height: 8,
      borderRadius: "50%",
      bgcolor: "success.main",
      mr: 1,
      animation: "agentSessionPulse 1.5s ease-in-out infinite",
      "@keyframes agentSessionPulse": { "0%, 100%": { opacity: 1 }, "50%": { opacity: 0.3 } },
      "@media (prefers-reduced-motion: reduce)": { animation: "none" },
    }}
  />
);

const names = (list, max = 2) => {
  const items = Array.isArray(list) ? list : [];
  if (items.length <= max) return items.join(", ");
  return `${items.slice(0, max).join(", ")} +${items.length - max}`;
};

const triState = (label, value, onChange, name) => (
  <TextField select fullWidth size="small" label={label} name={name} value={value} onChange={(e) => onChange(e.target.value)}>
    <MenuItem value="">Any</MenuItem>
    <MenuItem value="yes">Yes</MenuItem>
    <MenuItem value="no">No</MenuItem>
  </TextField>
);

const AgentSessions = () => {
  const { isEnterprise } = useEdition();
  const { can } = usePermissions();
  const canReadContent = can(P.TRACE_CONTENT_READ);
  const { features, loading: featuresLoading } = useSystemFeatures();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const url = useMemo(() => filtersFromSearchParams(searchParams), [searchParams]);
  const [state, setState] = useState(emptyStateFilters);
  const [debouncedState] = useDebounce(state, 400);
  const [idSearch, setIdSearch] = useState("");
  // The cursor of each visited page, for the filters they were read with.
  const [pager, setPager] = useState({ key: "", stack: [null] });
  const [pageData, setPageData] = useState({ data: [], next_cursor: null });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [apps, setApps] = useState([]);
  const [users, setUsers] = useState([]);
  const requestSeq = useRef(0);

  const tracingOn = Boolean(features?.feature_agent_tracing);
  const enabled = isEnterprise && tracingOn;

  const updateUrl = useCallback(
    (changes) => {
      setSearchParams(filtersToSearchParams({ ...url, ...changes }), { replace: true });
    },
    [url, setSearchParams],
  );

  // Any filter or sort change starts again from the first page, in the
  // same render, so no request pairs new filters with an old cursor.
  const filterKey = `${searchParams.toString()}|${JSON.stringify(debouncedState)}`;
  const cursors = pager.key === filterKey ? pager.stack : [null];
  const cursor = cursors[cursors.length - 1];

  const fetchPage = useCallback(async () => {
    const seq = ++requestSeq.current;
    setLoading(true);
    try {
      const res = await agentTracingService.listSessions(toQueryBody(url, debouncedState, { cursor, limit: PAGE_SIZE }));
      if (seq !== requestSeq.current) return;
      setPageData({ data: res?.data || [], next_cursor: res?.next_cursor || null });
      setError(null);
    } catch (err) {
      if (seq !== requestSeq.current) return;
      if (isPermissionDenied(err)) setError({ kind: "denied" });
      else if (isEnterpriseFeature(err)) setError({ kind: "enterprise" });
      else setError({ kind: "failed", message: err?.response?.data?.errors?.[0]?.detail || "Failed to load agent sessions" });
    } finally {
      if (seq === requestSeq.current) setLoading(false);
    }
  }, [url, debouncedState, cursor]);

  useEffect(() => {
    if (enabled) fetchPage();
  }, [enabled, fetchPage]);

  // Active sessions refresh every 5 s while they are on screen.
  const hasActive = pageData.data.some((s) => s.status === "active");
  useEffect(() => {
    if (!enabled || !hasActive) return undefined;
    const t = setInterval(fetchPage, LIVE_REFRESH_MS);
    return () => clearInterval(t);
  }, [enabled, hasActive, fetchPage]);

  useEffect(() => {
    if (!enabled) return;
    listAll(apiClient, "/apps")
      .then((r) => setApps(r.data?.data || []))
      .catch(() => setApps([]));
    listAll(apiClient, "/users")
      .then((r) => setUsers(r.data?.data || []))
      .catch(() => setUsers([]));
  }, [enabled]);

  // The id box: a session UUID goes in the URL; anything else is an
  // external session id, which may be personal data and stays in state.
  const applyIdSearch = () => {
    const v = idSearch.trim();
    if (isSessionUUID(v)) {
      setState((s) => ({ ...s, externalId: "" }));
      updateUrl({ session: v.toLowerCase() });
    } else {
      updateUrl({ session: "" });
      setState((s) => ({ ...s, externalId: v }));
    }
  };

  const columns = useMemo(
    () => [
      {
        field: "started_at",
        headerName: "Started",
        sortable: true,
        sx: { whiteSpace: "nowrap" },
        renderCell: (s) => new Date(s.started_at).toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" }),
      },
      {
        field: "session",
        headerName: "Session",
        renderCell: (s) => (
          <Stack direction="row" alignItems="center" spacing={0.5}>
            {s.status === "active" && <LiveDot />}
            <Typography variant="body2" sx={{ fontFamily: s.name ? undefined : "monospace" }}>
              {s.name || shortId(s.id)}
            </Typography>
            {s.source === "inferred" && (
              <Tooltip title="Grouped by the gateway from traffic patterns; the client did not name this session">
                <Chip size="small" variant="outlined" label="inferred" data-testid="inferred-chip" />
              </Tooltip>
            )}
            {s.totals?.dropped_count > 0 && (
              <Tooltip title={`${s.totals.dropped_count} captures were dropped under load`}>
                <Chip size="small" color="warning" variant="outlined" label="incomplete" sx={{ color: "text.primary" }} />
              </Tooltip>
            )}
          </Stack>
        ),
      },
      { field: "app", headerName: "App", renderCell: (s) => s.app_name || `App ${s.app_id}` },
      {
        field: "user",
        headerName: "User",
        renderCell: (s) => <SessionUser session={s} masked={!canReadContent} />,
      },
      {
        field: "agents",
        headerName: "Agents · models",
        renderCell: (s) => (
          <Box sx={{ minWidth: 0, maxWidth: 220 }}>
            <Typography variant="body2" noWrap>
              {s.agent_names?.length ? `${s.agent_names.length} · ${names(s.agent_names)}` : "—"}
            </Typography>
            <Typography variant="caption" color="text.secondary" display="block" noWrap>
              {names(s.models) || "—"}
            </Typography>
          </Box>
        ),
      },
      // Turns, LLM and tool calls share one column (sorted by LLM calls) so
      // the table fits the page width.
      {
        field: "llm_calls",
        headerName: "Activity",
        sortable: true,
        sx: { whiteSpace: "nowrap" },
        renderCell: (s) => (
          <Tooltip title={`${s.totals?.turn_count ?? 0} turns · ${s.totals?.llm_call_count ?? 0} LLM calls · ${s.totals?.tool_call_count ?? 0} tool calls`}>
            <Typography variant="body2">
              {s.totals?.turn_count ?? 0} turns · {s.totals?.llm_call_count ?? 0} LLM · {s.totals?.tool_call_count ?? 0} tools
            </Typography>
          </Tooltip>
        ),
      },
      {
        field: "errors",
        headerName: "Errors",
        sortable: true,
        align: "right",
        renderCell: (s) => (
          <Typography variant="body2" color={s.totals?.error_count ? "error" : "text.primary"}>
            {s.totals?.error_count ?? 0}
          </Typography>
        ),
      },
      { field: "duration", headerName: "Duration", sortable: true, align: "right", sx: { whiteSpace: "nowrap" }, renderCell: (s) => formatDuration(s.duration_ms) },
      {
        field: "cost",
        headerName: "Usage",
        sortable: true,
        align: "right",
        sx: { whiteSpace: "nowrap" },
        renderCell: (s) => (
          <>
            <Typography variant="body2">{formatCost(s.totals?.cost_micros, s.totals?.currency)}</Typography>
            <Typography variant="caption" color="text.secondary">
              {formatTokens((s.totals?.input_tokens || 0) + (s.totals?.output_tokens || 0))} tokens
            </Typography>
          </>
        ),
      },
      {
        field: "status",
        headerName: "Status",
        renderCell: (s) => <Chip size="small" label={s.status} color={STATUS_COLOURS[s.status] || "default"} />,
      },
      {
        field: "correlation",
        headerName: "Correlation",
        renderCell: (s) => <CorrelationQualityBar counts={s.confidence_counts} />,
      },
    ],
    [canReadContent],
  );

  const [captureOpen, setCaptureOpen] = useState(false);
  const title = (
    <TitleBox top="var(--studio-header-height)">
      <Typography variant="headingXLarge">Agent sessions</Typography>
      {isEnterprise && tracingOn && (
        <Button variant="outlined" onClick={() => setCaptureOpen(true)}>
          Capture settings
        </Button>
      )}
      <CaptureSettingsDialog open={captureOpen} onClose={() => setCaptureOpen(false)} />
    </TitleBox>
  );

  // The Community Edition shows the upgrade panel without asking the API.
  if (!isEnterprise) {
    return (
      <>
        {title}
        <ContentBox>
          <EnterpriseFeatureBadge
            feature="Agent Call Tracing"
            description="See every agent session that crosses the gateway: turns, LLM and tool calls, sub-agents and gateway events, with transcripts and timelines."
          />
        </ContentBox>
      </>
    );
  }
  if (!featuresLoading && !tracingOn) {
    return (
      <>
        {title}
        <ContentBox>
          <Alert severity="info" data-testid="tracing-disabled">
            Agent Call Tracing is not running. Set AGENT_TRACING_ENABLED=true on the Studio server to start capturing
            agent sessions.
          </Alert>
        </ContentBox>
      </>
    );
  }
  if (error?.kind === "denied") {
    return <PermissionDeniedPanel required={P.TRACES_READ} />;
  }
  if (error?.kind === "enterprise") {
    return (
      <>
        {title}
        <ContentBox>
          <EnterpriseFeatureBadge feature="Agent Call Tracing" />
        </ContentBox>
      </>
    );
  }

  const droppedSessions = pageData.data.filter((s) => s.totals?.dropped_count > 0).length;
  const appOptions = apps.map((a) => ({ id: String(a.id), name: a.attributes?.name || `App ${a.id}` }));
  const userOptions = users.map((u) => ({ id: String(u.id), name: u.attributes?.name || u.attributes?.email || `User ${u.id}` }));
  const setStateField = (key) => (e) => setState((s) => ({ ...s, [key]: e.target.value }));

  return (
    <>
      {title}
      <ContentBox>
        <Grid container spacing={2} sx={{ mb: 2 }} data-testid="session-filters">
          <Grid item xs={12} md={3}>
            <TextField
              select
              fullWidth
              size="small"
              label="Time range"
              name="range"
              value={url.range}
              onChange={(e) => {
                const range = e.target.value;
                if (range === "custom") {
                  const today = new Date().toISOString().slice(0, 10);
                  updateUrl({ range, from: url.from || today, to: url.to || today });
                } else {
                  updateUrl({ range, from: "", to: "" });
                }
              }}
            >
              {RANGES.map((r) => (
                <MenuItem key={r.value} value={r.value}>
                  {r.label}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          {url.range === "custom" && (
            <Grid item xs={12} md={9}>
              <DateRangePicker
                startDate={url.from}
                endDate={url.to}
                onStartDateChange={(v) => updateUrl({ from: v })}
                onEndDateChange={(v) => updateUrl({ to: v })}
              />
            </Grid>
          )}
          <Grid item xs={12} md={3}>
            <TextField select fullWidth size="small" label="App" name="app" value={url.app} onChange={(e) => updateUrl({ app: e.target.value })}>
              <MenuItem value="">All Apps</MenuItem>
              {appOptions.map((a) => (
                <MenuItem key={a.id} value={a.id}>
                  {a.name}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid item xs={12} md={3}>
            <TextField select fullWidth size="small" label="User" name="user" value={url.user} onChange={(e) => updateUrl({ user: e.target.value })}>
              <MenuItem value="">All users</MenuItem>
              {userOptions.map((u) => (
                <MenuItem key={u.id} value={u.id}>
                  {u.name}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid item xs={6} md={3}>
            <TextField select fullWidth size="small" label="Status" name="status" value={url.status} onChange={(e) => updateUrl({ status: e.target.value })}>
              <MenuItem value="">Any</MenuItem>
              <MenuItem value="active">Active</MenuItem>
              <MenuItem value="closed">Closed</MenuItem>
              <MenuItem value="error">Error</MenuItem>
            </TextField>
          </Grid>
          <Grid item xs={6} md={3}>
            {triState("Has errors", url.errors, (v) => updateUrl({ errors: v }), "errors")}
          </Grid>
          <Grid item xs={6} md={3}>
            {triState("Has gateway events", url.gw, (v) => updateUrl({ gw: v }), "gw")}
          </Grid>
          <Grid item xs={6} md={3}>
            <TextField select fullWidth size="small" label="Session source" name="source" value={url.source} onChange={(e) => updateUrl({ source: e.target.value })}>
              <MenuItem value="">Any</MenuItem>
              <MenuItem value="declared">Declared by the client</MenuItem>
              <MenuItem value="inferred">Inferred by the gateway</MenuItem>
            </TextField>
          </Grid>
          <Grid item xs={12} md={3}>
            <FormControlLabel
              control={<Checkbox checked={url.weak} onChange={(e) => updateUrl({ weak: e.target.checked })} name="weak" />}
              label="Contains weak links"
            />
          </Grid>
          <Grid item xs={12} md={6}>
            <Stack direction="row" spacing={1}>
              <TextField
                fullWidth
                size="small"
                label="Session or external id"
                name="id-search"
                value={idSearch}
                onChange={(e) => setIdSearch(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") applyIdSearch();
                }}
                helperText={canReadContent ? "Exact match" : PSEUDONYM_FILTER_HINT}
              />
              <Button variant="outlined" onClick={applyIdSearch} sx={{ alignSelf: "flex-start" }}>
                Find
              </Button>
            </Stack>
          </Grid>
        </Grid>

        <Accordion disableGutters sx={{ mb: 2 }}>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Typography>More filters</Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Grid container spacing={2}>
              <Grid item xs={12} md={4}>
                <TextField fullWidth size="small" label="Agent name" name="agent" value={state.agent} onChange={setStateField("agent")} />
              </Grid>
              <Grid item xs={12} md={4}>
                <TextField fullWidth size="small" label="Model" name="model" value={state.model} onChange={setStateField("model")} />
              </Grid>
              <Grid item xs={12} md={4}>
                <TextField fullWidth size="small" label="Tool called" name="tool" value={state.tool} onChange={setStateField("tool")} />
              </Grid>
              <Grid item xs={12} md={4}>
                <TextField fullWidth size="small" label="End user" name="endUser" value={state.endUser} onChange={setStateField("endUser")}
                  helperText={canReadContent ? "The user id the client declared" : PSEUDONYM_FILTER_HINT} />
              </Grid>
              {[
                ["costMin", "Minimum cost"],
                ["costMax", "Maximum cost"],
                ["durMin", "Minimum duration (s)"],
                ["durMax", "Maximum duration (s)"],
              ].map(([key, label]) => (
                <Grid item xs={6} md={2} key={key}>
                  <TextField
                    fullWidth
                    size="small"
                    type="number"
                    label={label}
                    name={key}
                    value={url[key]}
                    inputProps={{ min: 0, step: "any" }}
                    onChange={(e) => updateUrl({ [key]: e.target.value })}
                  />
                </Grid>
              ))}
            </Grid>
          </AccordionDetails>
        </Accordion>

        {droppedSessions > 0 && (
          <Alert severity="warning" sx={{ mb: 2 }} data-testid="dropped-banner">
            {droppedSessions === 1 ? "One session on this page is" : `${droppedSessions} sessions on this page are`} incomplete:
            the gateway dropped some captures under load.
          </Alert>
        )}
        {error?.kind === "failed" && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error.message}
          </Alert>
        )}

        <DataTable
          aria-label="Agent sessions"
          columns={columns}
          data={pageData.data}
          loading={loading}
          sortConfig={sortConfigOf(url.sort)}
          onSortChange={(next) => updateUrl({ sort: sortParamOf(next) })}
          onRowClick={(s) => navigate(`/admin/agent-sessions/${s.id}`)}
          getRowLabel={(s) => s.name || shortId(s.id)}
          emptyMessage="No agent sessions match these filters"
        />

        <Stack direction="row" spacing={2} alignItems="center" justifyContent="flex-end" sx={{ mt: 2 }}>
          <Typography variant="body2" color="text.secondary">
            Page {cursors.length}
          </Typography>
          <Button
            variant="outlined"
            disabled={cursors.length <= 1 || loading}
            onClick={() => setPager({ key: filterKey, stack: cursors.slice(0, -1) })}
          >
            Previous
          </Button>
          <Button
            variant="outlined"
            disabled={!pageData.next_cursor || loading}
            onClick={() => setPager({ key: filterKey, stack: [...cursors, pageData.next_cursor] })}
          >
            Next
          </Button>
        </Stack>
      </ContentBox>
    </>
  );
};

export default AgentSessions;
