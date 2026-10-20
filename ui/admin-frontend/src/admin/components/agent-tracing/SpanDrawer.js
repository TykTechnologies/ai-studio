import React, { useEffect, useRef, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Drawer,
  IconButton,
  Link,
  List,
  ListItem,
  Stack,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableRow,
  Tabs,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@mui/material";
import CloseIcon from "@mui/icons-material/Close";
import { useSessionDetail } from "./SessionDetailContext";
import {
  INPUT_PARTS,
  KIND_LABELS,
  OUTPUT_PARTS,
  PART_LABELS,
  attrsOf,
  durationMs,
  formatBytes,
  partPlaceholder,
  redactionSegments,
  resourceName,
} from "./spanModel";
import { formatCost, formatDuration } from "./sessionFilters";
import agentTracingService from "../../services/agentTracingService";

// The span detail drawer (spec §6.2): right-anchored and resizable, with
// Overview, Input, Output, Attributes and Links tabs. The span's links and
// events load when the drawer opens; captured content loads only when a
// content tab is shown, and never for a viewer without content access.
// The drawer is modal: focus stays inside while it is open and returns to
// where it was when it closes (Esc closes it).

export const PREVIEW_BYTES = 2048;
export const FULL_BYTES = 2 * 1024 * 1024;
const WIDTH_KEY = "agentTracing.drawerWidth";
const VIEW_KEY = "agentTracing.view.";

const readPref = (key, fallback) => {
  try {
    return window.localStorage.getItem(key) || fallback;
  } catch (e) {
    return fallback;
  }
};
const writePref = (key, value) => {
  try {
    window.localStorage.setItem(key, value);
  } catch (e) {
    // private mode: the preference just is not remembered
  }
};

/** Text with [REDACTED:TYPE] markers shown as chips. */
export const RedactedText = ({ text }) => (
  <Box component="span" sx={{ whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
    {redactionSegments(text).map((seg, i) =>
      seg.redacted ? (
        <Chip key={i} size="small" color="warning" variant="outlined" label={`REDACTED:${seg.redacted}`} sx={{ mx: 0.25, height: 20, color: "text.primary" }} />
      ) : (
        <React.Fragment key={i}>{seg.text}</React.Fragment>
      ),
    )}
  </Box>
);

const parseJSON = (s) => {
  try {
    return { ok: true, value: JSON.parse(s) };
  } catch (e) {
    return { ok: false };
  }
};

/** Content in a Pretty or JSON (raw) view; the choice is remembered per part. */
export const ContentView = ({ part, payload }) => {
  const [mode, setMode] = useState(() => readPref(VIEW_KEY + part, "pretty"));
  const text = payload.content ?? "";
  const json = parseJSON(text);
  const object = json.ok && json.value && typeof json.value === "object" && !Array.isArray(json.value) ? json.value : null;
  let pretty;
  if (object && Object.keys(object).length <= 20) {
    pretty = (
      <Table size="small" data-testid="kv-table">
        <TableBody>
          {Object.entries(object).map(([k, v]) => (
            <TableRow key={k}>
              <TableCell sx={{ fontFamily: "monospace", verticalAlign: "top", whiteSpace: "nowrap" }}>{k}</TableCell>
              <TableCell>
                <RedactedText text={typeof v === "string" ? v : JSON.stringify(v, null, 2)} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    );
  } else if (json.ok) {
    pretty = (
      <Box component="pre" sx={{ m: 0, fontSize: 12 }}>
        <RedactedText text={JSON.stringify(json.value, null, 2)} />
      </Box>
    );
  } else {
    pretty = <RedactedText text={text} />;
  }
  return (
    <Box>
      <ToggleButtonGroup
        size="small"
        exclusive
        value={mode}
        onChange={(_, v) => {
          if (!v) return;
          setMode(v);
          writePref(VIEW_KEY + part, v);
        }}
        sx={{ mb: 1 }}
      >
        <ToggleButton value="pretty">Pretty</ToggleButton>
        <ToggleButton value="raw">JSON</ToggleButton>
      </ToggleButtonGroup>
      {mode === "raw" ? (
        <Box component="pre" data-testid="raw-view" sx={{ m: 0, fontSize: 12, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
          {text}
        </Box>
      ) : (
        pretty
      )}
      {payload.truncated && (
        <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 1 }}>
          Showing {formatBytes(payload.returned_bytes ?? text.length)} of {formatBytes(payload.original_bytes)}
        </Typography>
      )}
    </Box>
  );
};

/** One captured part: its placeholder, then (for content readers) its content. */
export const PartView = ({ spanId, part }) => {
  const { appId, viewer } = useSessionDetail();
  const ph = partPlaceholder(part);
  const canLoad = ph.loadable && viewer.can_read_content;
  const [state, setState] = useState({ loading: canLoad, payload: null, error: null, full: false });

  const load = async (maxBytes, full) => {
    setState((s) => ({ ...s, loading: true, error: null }));
    try {
      const payload = await agentTracingService.getPayload(spanId, appId, part.part, maxBytes);
      setState({ loading: false, payload, error: null, full });
    } catch (err) {
      const status = err?.response?.status;
      const error =
        status === 403
          ? "Your role cannot view captured content"
          : status === 404 || status === 405
            ? "Content is not available on this server yet"
            : "Could not load the content";
      setState({ loading: false, payload: null, error, full });
    }
  };

  useEffect(() => {
    if (canLoad) load(PREVIEW_BYTES, false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [spanId, part.part, canLoad]);

  const more = state.payload && state.payload.truncated && !state.full && (part.original_bytes || 0) > PREVIEW_BYTES;
  return (
    <Box sx={{ mb: 2 }} data-testid={`part-${part.part}`}>
      <Typography variant="subtitle2">{PART_LABELS[part.part] || part.part}</Typography>
      <Typography variant="caption" color="text.secondary" display="block" data-testid="part-placeholder">
        {ph.text}
      </Typography>
      {ph.loadable && !viewer.can_read_content && (
        <Typography variant="caption" color="text.secondary" display="block" data-testid="no-content-permission">
          Your role does not include reading captured content.
        </Typography>
      )}
      {state.loading && <CircularProgress size={16} sx={{ mt: 1 }} />}
      {state.error && (
        <Alert severity="info" sx={{ mt: 1 }}>
          {state.error}
        </Alert>
      )}
      {state.payload && (
        <Box sx={{ mt: 1 }}>
          <ContentView part={part.part} payload={state.payload} />
          {more && (
            <Button size="small" onClick={() => load(FULL_BYTES, true)} sx={{ mt: 1 }}>
              Load full
            </Button>
          )}
        </Box>
      )}
    </Box>
  );
};

const PartsPanel = ({ span, names }) => {
  const parts = (span.parts || []).filter((p) => names.includes(p.part));
  if (!parts.length) {
    return (
      <Typography variant="body2" color="text.secondary" data-testid="no-parts">
        Nothing was captured for this span ({span.content_state || "not captured"}).
      </Typography>
    );
  }
  return parts.map((p) => <PartView key={p.part} spanId={span.span_id} part={p} />);
};

const Row = ({ label, children }) =>
  children === undefined || children === null || children === "" ? null : (
    <TableRow>
      <TableCell sx={{ color: "text.secondary", whiteSpace: "nowrap", verticalAlign: "top" }}>{label}</TableCell>
      <TableCell sx={{ wordBreak: "break-all" }}>{children}</TableCell>
    </TableRow>
  );

const Overview = ({ span, events, resources }) => {
  const u = span.usage || {};
  const candidates = (span.links || []).filter((l) => l.type === "candidate");
  return (
    <Box>
      <Table size="small">
        <TableBody>
          <Row label="Kind">{KIND_LABELS[span.kind] || span.kind}</Row>
          <Row label="Status">{span.status}</Row>
          <Row label="Started">{new Date(span.start_time).toLocaleString()}</Row>
          <Row label="Duration">{span.end_time ? formatDuration(durationMs(span)) : "in progress"}</Row>
          <Row label="Resource">{resourceName(resources, span)}</Row>
          <Row label="Model">{span.response_model || span.request_model}</Row>
          <Row label="Tool">{span.tool_name}</Row>
          <Row label="Tokens">{u.input_tokens || u.output_tokens ? `${u.input_tokens || 0} in · ${u.output_tokens || 0} out` : undefined}</Row>
          <Row label="Cost">{u.cost_micros ? formatCost(u.cost_micros) : undefined}</Row>
          <Row label="Time to first token">{span.ttft_ms != null ? `${span.ttft_ms} ms` : undefined}</Row>
          <Row label="Finish">{(span.finish_reasons || []).join(", ")}</Row>
          <Row label="Error">{span.error_type}</Row>
          <Row label="HTTP status">{span.http_status || undefined}</Row>
          <Row label="Span id">{span.span_id}</Row>
          <Row label="Trace id">{span.trace_id}</Row>
          <Row label="Parent">
            {span.parent_span_id
              ? `${span.parent_span_id} (${span.parent_confidence || "declared"}${span.parent_rule ? `, ${span.parent_rule}` : ""})`
              : undefined}
          </Row>
          <Row label="Tool call ids">{(span.tool_call_ids || []).join(", ")}</Row>
          <Row label="Request log">{span.analytics_ref}</Row>
        </TableBody>
      </Table>
      {candidates.length > 0 && (
        <Box sx={{ mt: 2 }}>
          <Typography variant="subtitle2">Candidate links (not used for nesting)</Typography>
          {candidates.map((l) => (
            <Typography key={l.span_id} variant="body2">
              {l.span_id} · {l.confidence} · {l.rule}
            </Typography>
          ))}
        </Box>
      )}
      {events?.length > 0 && (
        <Box sx={{ mt: 2 }}>
          <Typography variant="subtitle2">Gateway events</Typography>
          <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
            {events.map((e) => (
              <Chip key={e.span_id} size="small" label={e.name} color={e.status === "error" ? "error" : "default"} />
            ))}
          </Stack>
        </Box>
      )}
    </Box>
  );
};

const LinksPanel = ({ span }) => {
  const { dispatch, index } = useSessionDetail();
  const links = span.links || [];
  if (!links.length) return <Typography variant="body2">No links.</Typography>;
  return (
    <List dense>
      {links.map((l, i) => (
        <ListItem key={i} disableGutters>
          <Stack direction="row" spacing={1} alignItems="center">
            <Chip size="small" label={l.type} />
            <Tooltip title={`Rule: ${l.rule}`}>
              <Chip size="small" variant="outlined" label={l.confidence} />
            </Tooltip>
            {index.byId.has(l.span_id) ? (
              <Link component="button" onClick={() => dispatch({ type: "open", id: l.span_id })}>
                {index.byId.get(l.span_id).span.name}
              </Link>
            ) : (
              <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                {l.span_id}
              </Typography>
            )}
          </Stack>
        </ListItem>
      ))}
    </List>
  );
};

const TABS = ["overview", "input", "output", "attributes", "links"];

const SpanDrawer = () => {
  const { drawerSpanId, dispatch, appId, index, resources } = useSessionDetail();
  const [tab, setTab] = useState("overview");
  const [detail, setDetail] = useState(null);
  const [error, setError] = useState(null);
  const [width, setWidth] = useState(() => Number(readPref(WIDTH_KEY, "560")) || 560);
  const resizing = useRef(false);
  const open = Boolean(drawerSpanId);
  const skeleton = drawerSpanId ? index.byId.get(drawerSpanId)?.span : null;

  useEffect(() => {
    if (!drawerSpanId) return undefined;
    let cancelled = false;
    setDetail(null);
    setError(null);
    agentTracingService
      .getSpan(drawerSpanId, appId)
      .then((d) => !cancelled && setDetail(d))
      .catch(() => !cancelled && setError("Could not load this span's links and events"));
    return () => {
      cancelled = true;
    };
  }, [drawerSpanId, appId]);

  const span = detail?.span || skeleton;

  const onResizeStart = (e) => {
    e.preventDefault();
    resizing.current = true;
    const move = (ev) => {
      if (!resizing.current) return;
      const w = Math.min(Math.max(window.innerWidth - ev.clientX, 360), window.innerWidth - 120);
      setWidth(w);
    };
    const up = () => {
      resizing.current = false;
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
      setWidth((w) => {
        writePref(WIDTH_KEY, String(Math.round(w)));
        return w;
      });
    };
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={() => dispatch({ type: "close" })}
      PaperProps={{ sx: { width, maxWidth: "100vw" }, "aria-labelledby": "span-drawer-title", role: "dialog" }}
      data-testid="span-drawer"
    >
      <Box
        aria-hidden
        onMouseDown={onResizeStart}
        sx={{ position: "absolute", left: 0, top: 0, bottom: 0, width: 6, cursor: "col-resize", zIndex: 1 }}
      />
      {span && (
        <Box sx={{ p: 2, pl: 3 }}>
          <Stack direction="row" alignItems="flex-start" spacing={1}>
            <Box sx={{ flexGrow: 1, minWidth: 0 }}>
              <Typography variant="headingSmall" component="h2" id="span-drawer-title" sx={{ wordBreak: "break-word" }}>
                {span.name}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {KIND_LABELS[span.kind] || span.kind}
                {span.inferred ? " · inferred" : ""}
              </Typography>
            </Box>
            <IconButton aria-label="Close span details" onClick={() => dispatch({ type: "close" })}>
              <CloseIcon />
            </IconButton>
          </Stack>
          <Tabs value={tab} onChange={(_, v) => setTab(v)} variant="scrollable" sx={{ mt: 1, mb: 2 }} aria-label="Span details">
            {TABS.map((t) => (
              <Tab key={t} value={t} label={t[0].toUpperCase() + t.slice(1)} />
            ))}
          </Tabs>
          {error && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}
          {tab === "overview" && <Overview span={span} events={detail?.events} resources={detail?.resources || resources} />}
          {tab === "input" && <PartsPanel span={span} names={[...INPUT_PARTS, "user_message"]} />}
          {tab === "output" && <PartsPanel span={span} names={OUTPUT_PARTS} />}
          {tab === "attributes" && (
            <Box component="pre" sx={{ m: 0, fontSize: 12, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
              {JSON.stringify(attrsOf(span), null, 2)}
            </Box>
          )}
          {tab === "links" && <LinksPanel span={span} />}
        </Box>
      )}
    </Drawer>
  );
};

export default SpanDrawer;
