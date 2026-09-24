/**
 * Presentation helpers for the "what will be pushed" preview: how each change
 * type is named, where its admin page lives, and how times are shown.
 */

// Display order of the groups in the preview; the product's names for each
// change type and the admin page that shows one of them (when there is one).
export const CHANGE_TYPES = [
  { type: 'llm', label: 'LLM providers', path: (id) => `/admin/llms/${id}` },
  { type: 'app', label: 'Apps', path: (id) => `/admin/apps/${id}` },
  { type: 'tool', label: 'Tools', path: (id) => `/admin/tools/${id}` },
  { type: 'datasource', label: 'Data sources', path: (id) => `/admin/datasources/${id}` },
  { type: 'filter', label: 'Filters', path: (id) => `/admin/filters/${id}` },
  { type: 'model_price', label: 'Model prices', path: (id) => `/admin/model-prices/${id}` },
  { type: 'model_router', label: 'Model routers', path: (id) => `/admin/model-routers/${id}` },
  { type: 'semantic_router', label: 'Semantic routers', path: (id) => `/admin/semantic-routers/${id}` },
  { type: 'embedder', label: 'Embedders', path: (id) => `/admin/embedders/${id}` },
  { type: 'plugin', label: 'Plugins', path: (id) => `/admin/plugins/${id}` },
  { type: 'oauth_client', label: 'OAuth clients', path: null },
  { type: 'access_token', label: 'Access tokens', path: null },
];

const TYPE_INDEX = new Map(CHANGE_TYPES.map((t, i) => [t.type, i]));

/** Product name for a change type; unknown types are shown as they come. */
export const changeTypeLabel = (type) =>
  CHANGE_TYPES.find((t) => t.type === type)?.label || type;

/**
 * Admin page for a change, or null when there is none to link to (deleted
 * objects have no page any more; tokens and OAuth clients have no page of
 * their own).
 */
export const changeLink = (change) => {
  if (!change || change.change === 'deleted' || change.id == null) return null;
  const entry = CHANGE_TYPES.find((t) => t.type === change.type);
  return entry?.path ? entry.path(change.id) : null;
};

/**
 * Groups changes by type in CHANGE_TYPES order (unknown types last, in the
 * order they arrived). Returns [{ type, label, changes }].
 */
export const groupChanges = (changes = []) => {
  const groups = new Map();
  for (const change of changes) {
    if (!change) continue;
    const type = change.type || 'other';
    if (!groups.has(type)) {
      groups.set(type, { type, label: changeTypeLabel(type), changes: [] });
    }
    groups.get(type).changes.push(change);
  }
  return [...groups.values()].sort((a, b) => {
    const ai = TYPE_INDEX.has(a.type) ? TYPE_INDEX.get(a.type) : CHANGE_TYPES.length;
    const bi = TYPE_INDEX.has(b.type) ? TYPE_INDEX.get(b.type) : CHANGE_TYPES.length;
    return ai - bi;
  });
};

/** "13:12" for today, "13 Sep, 13:12" otherwise; null when there is no time. */
export const formatPushTime = (iso, now = new Date()) => {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  if (sameDay) return time;
  const day = d.toLocaleDateString([], { day: 'numeric', month: 'short' });
  return `${day}, ${time}`;
};

/** "just now", "5 minutes ago", "3 hours ago", "yesterday", "4 days ago". */
export const relativeTime = (iso, now = new Date()) => {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const seconds = Math.max(0, Math.round((now - d) / 1000));
  if (seconds < 45) return 'just now';
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? '' : 's'} ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'} ago`;
  const days = Math.round(hours / 24);
  if (days === 1) return 'yesterday';
  if (days < 30) return `${days} days ago`;
  return d.toLocaleDateString();
};

/**
 * The global/default namespace has three spellings across the APIs: "" (the
 * sync status and object tables), "global" (the push endpoint) and "default"
 * (edge registration). Compare namespaces through these so a lookup never
 * misses its own row.
 */
export const canonicalNamespace = (ns) => {
  const trimmed = (ns == null ? '' : String(ns)).trim();
  const lower = trimmed.toLowerCase();
  return lower === '' || lower === 'global' || lower === 'default' ? 'default' : trimmed;
};

export const sameNamespace = (a, b) => canonicalNamespace(a) === canonicalNamespace(b);

/**
 * What the preview measured from (mirrors the API's `baseline`): a recorded
 * push, an in-sync edge's ack when no push was ever recorded (a Studio
 * upgraded from before pushes were stamped), or nothing.
 */
const pendingBaseline = ({ baseline, lastPushAt } = {}) => {
  if (baseline === 'push' || baseline === 'edge_ack' || baseline === 'none') return baseline;
  return lastPushAt ? 'push' : 'none';
};

/**
 * The one-line summary above the list:
 *   "12 changes since the last push at 13:12"
 *   "Nothing has changed since the last push (13:12)"
 * when the reference point is an edge's ack rather than a recorded push,
 *   "12 changes since the last sync at 13:12"
 * and, when nothing was ever pushed, "12 changes (never pushed)".
 */
export const summarizePending = (data = {}, now = new Date()) => {
  const { total = 0, lastPushAt = null, since = null } = data;
  const baseline = pendingBaseline(data);
  const count = `${total} change${total === 1 ? '' : 's'}`;

  if (baseline === 'edge_ack') {
    const synced = formatPushTime(since, now);
    if (total === 0) {
      return synced ? `Nothing has changed since the last sync (${synced})` : 'Nothing has changed since the last sync';
    }
    return synced ? `${count} since the last sync at ${synced}` : `${count} since the last sync`;
  }

  const pushed = baseline === 'push' ? formatPushTime(lastPushAt || since, now) : null;
  if (total === 0) {
    return pushed
      ? `Nothing has changed since the last push (${pushed})`
      : 'Nothing has changed (no push recorded yet)';
  }
  return pushed ? `${count} since the last push at ${pushed}` : `${count} (never pushed)`;
};
