// Sessions list filters (spec §6.1) and the formatting the list shares with
// the session detail.
//
// Only filters that carry no personal data or free text live in the page URL
// (security review, jyx.28): the time range, App and Studio user ids,
// status, source, the yes/no filters, numeric ranges, the sort and an exact
// session UUID. Agent, model and tool names, the end user and an external
// session id stay in component state and travel in the POST body.

export const RANGES = [
  { value: "1h", label: "Last hour", ms: 3600e3 },
  { value: "24h", label: "Last 24 hours", ms: 24 * 3600e3 },
  { value: "7d", label: "Last 7 days", ms: 7 * 24 * 3600e3 },
  { value: "30d", label: "Last 30 days", ms: 30 * 24 * 3600e3 },
  { value: "custom", label: "Custom range" },
];

export const DEFAULT_SORT = "-started_at";
export const PAGE_SIZE = 50;

export const SORT_FIELDS = ["started_at", "cost", "duration", "tokens", "turns", "llm_calls", "tool_calls", "errors"];

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const DATE = /^\d{4}-\d{2}-\d{2}$/;
const ID = /^[1-9]\d*$/;
const DECIMAL = /^\d+(\.\d+)?$/;

export const isSessionUUID = (s) => UUID.test(String(s || "").trim().toLowerCase());

/** The URL filters with every field present ("" when unset). */
export const emptyUrlFilters = () => ({
  range: "24h",
  from: "",
  to: "",
  app: "",
  user: "",
  status: "",
  source: "",
  weak: false,
  errors: "",
  gw: "",
  costMin: "",
  costMax: "",
  durMin: "",
  durMax: "",
  sort: DEFAULT_SORT,
  session: "",
});

/** Filters held in component state only (never in a URL). */
export const emptyStateFilters = () => ({ agent: "", model: "", tool: "", endUser: "", externalId: "" });

/**
 * Reads the URL filters, dropping anything malformed so a hand-edited link
 * cannot put an unexpected value into the query.
 */
export const filtersFromSearchParams = (sp) => {
  const f = emptyUrlFilters();
  const get = (k) => (sp.get(k) || "").trim();
  const range = get("range");
  if (RANGES.some((r) => r.value === range)) f.range = range;
  if (f.range === "custom") {
    if (DATE.test(get("from"))) f.from = get("from");
    if (DATE.test(get("to"))) f.to = get("to");
    if (!f.from || !f.to) f.range = "24h";
  }
  if (ID.test(get("app"))) f.app = get("app");
  if (ID.test(get("user"))) f.user = get("user");
  if (["active", "closed", "error"].includes(get("status"))) f.status = get("status");
  if (["declared", "inferred"].includes(get("source"))) f.source = get("source");
  f.weak = get("weak") === "1";
  if (["yes", "no"].includes(get("errors"))) f.errors = get("errors");
  if (["yes", "no"].includes(get("gw"))) f.gw = get("gw");
  ["costMin", "costMax", "durMin", "durMax"].forEach((k) => {
    const key = { costMin: "cost_min", costMax: "cost_max", durMin: "dur_min", durMax: "dur_max" }[k];
    if (DECIMAL.test(get(key))) f[k] = get(key);
  });
  const sort = get("sort");
  if (SORT_FIELDS.includes(sort.replace(/^-/, ""))) f.sort = sort;
  if (isSessionUUID(get("session"))) f.session = get("session").toLowerCase();
  return f;
};

/** Writes the URL filters, leaving defaults out so links stay short. */
export const filtersToSearchParams = (f) => {
  const sp = new URLSearchParams();
  if (f.range !== "24h") sp.set("range", f.range);
  if (f.range === "custom") {
    if (f.from) sp.set("from", f.from);
    if (f.to) sp.set("to", f.to);
  }
  if (f.app) sp.set("app", f.app);
  if (f.user) sp.set("user", f.user);
  if (f.status) sp.set("status", f.status);
  if (f.source) sp.set("source", f.source);
  if (f.weak) sp.set("weak", "1");
  if (f.errors) sp.set("errors", f.errors);
  if (f.gw) sp.set("gw", f.gw);
  if (f.costMin) sp.set("cost_min", f.costMin);
  if (f.costMax) sp.set("cost_max", f.costMax);
  if (f.durMin) sp.set("dur_min", f.durMin);
  if (f.durMax) sp.set("dur_max", f.durMax);
  if (f.sort && f.sort !== DEFAULT_SORT) sp.set("sort", f.sort);
  if (f.session) sp.set("session", f.session);
  return sp;
};

const startOfDay = (d) => new Date(`${d}T00:00:00`);
const nextDay = (d) => {
  const t = startOfDay(d);
  t.setDate(t.getDate() + 1);
  return t;
};

/**
 * The POST /agent-tracing/sessions/query body for the filters, at now.
 * Costs are entered in currency units and sent as micros; durations are
 * entered in seconds and sent as milliseconds.
 */
export const toQueryBody = (url, state, { now = new Date(), cursor, limit = PAGE_SIZE } = {}) => {
  const body = { sort: url.sort || DEFAULT_SORT, limit };
  if (url.range === "custom" && url.from && url.to) {
    body.from = startOfDay(url.from).toISOString();
    body.to = nextDay(url.to).toISOString();
  } else {
    const range = RANGES.find((r) => r.value === url.range) || RANGES[1];
    body.to = now.toISOString();
    body.from = new Date(now.getTime() - range.ms).toISOString();
  }
  if (url.app) body.app_id = Number(url.app);
  if (url.user) body.user_id = Number(url.user);
  if (url.status) body.status = url.status;
  if (url.source) body.source = url.source;
  if (url.weak) body.weak_links = true;
  if (url.errors) body.has_errors = url.errors === "yes";
  if (url.gw) body.has_gateway_events = url.gw === "yes";
  if (url.costMin) body.cost_min_micros = Math.round(Number(url.costMin) * 1e6);
  if (url.costMax) body.cost_max_micros = Math.round(Number(url.costMax) * 1e6);
  if (url.durMin) body.duration_min_ms = Math.round(Number(url.durMin) * 1000);
  if (url.durMax) body.duration_max_ms = Math.round(Number(url.durMax) * 1000);
  if (url.session) body.session_id = url.session;
  const trimmed = (v) => String(v || "").trim();
  if (trimmed(state.agent)) body.agent = trimmed(state.agent);
  if (trimmed(state.model)) body.model = trimmed(state.model);
  if (trimmed(state.tool)) body.tool = trimmed(state.tool);
  if (trimmed(state.endUser)) body.end_user = trimmed(state.endUser);
  if (trimmed(state.externalId)) body.external_id = trimmed(state.externalId);
  if (cursor) body.cursor = cursor;
  return body;
};

/** The DataTable sort config for a sort param ("-cost") and back. */
export const sortConfigOf = (sort) => {
  const s = sort || DEFAULT_SORT;
  return { field: s.replace(/^-/, ""), direction: s.startsWith("-") ? "desc" : "asc" };
};
export const sortParamOf = ({ field, direction }) => `${direction === "desc" ? "-" : ""}${field}`;

export const shortId = (id) => String(id || "").slice(0, 8);

export const formatCost = (micros, currency) => {
  const amount = (Number(micros) || 0) / 1e6;
  const code = currency || "USD";
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: code,
      minimumFractionDigits: amount > 0 && amount < 0.01 ? 4 : 2,
      maximumFractionDigits: amount > 0 && amount < 0.01 ? 4 : 2,
    }).format(amount);
  } catch (e) {
    return `${amount.toFixed(4)} ${code}`;
  }
};

export const formatDuration = (ms) => {
  const n = Number(ms) || 0;
  if (n < 1000) return `${n} ms`;
  const s = n / 1000;
  if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${Math.round(s % 60)} s`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
};

export const formatTokens = (n) => (Number(n) || 0).toLocaleString();

/** Shares of declared, exact, strong and weak parent edges (sum 1, or all 0). */
export const confidenceShares = (counts = {}) => {
  const keys = ["declared", "exact", "strong", "weak"];
  const total = keys.reduce((sum, k) => sum + (Number(counts[k]) || 0), 0);
  return keys.map((k) => ({ key: k, count: Number(counts[k]) || 0, share: total ? (Number(counts[k]) || 0) / total : 0 }));
};
