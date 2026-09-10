import { format, formatDistance, isValid, parseISO } from "date-fns";

// Helpers shared by the "Models in use" table on the LLM details page and the
// per-model detail page. Kept free of React so they can be unit tested directly.

const ZERO_TIME_PREFIX = "0001-01-01";

/** Parse a timestamp from the analytics API. Returns null for missing or zero values. */
export const parseUsageTime = (value) => {
  if (!value) return null;
  if (value instanceof Date) return isValid(value) ? value : null;
  if (typeof value === "string" && value.startsWith(ZERO_TIME_PREFIX)) return null;
  const parsed = typeof value === "string" ? parseISO(value) : new Date(value);
  return isValid(parsed) ? parsed : null;
};

/**
 * Relative and absolute renderings of a "last used" timestamp.
 * Relative is what you scan ("4 days ago"); absolute is what you quote in an email.
 */
export const formatUsageTime = (value, now = new Date()) => {
  const date = parseUsageTime(value);
  if (!date) return { relative: "Never", absolute: "" };
  const diffMs = now.getTime() - date.getTime();
  const relative =
    diffMs >= 0 && diffMs < 60 * 1000
      ? "Just now"
      : formatDistance(date, now, { addSuffix: true });
  return { relative, absolute: format(date, "d MMM yyyy HH:mm") };
};

/** Route to the per-model detail view. The model goes in the query string because
 *  vendor model IDs can contain colons and slashes (Bedrock, OpenRouter). */
export const modelDetailPath = (llmId, model) =>
  `/admin/llms/${llmId}/models?model=${encodeURIComponent(model)}`;

const compare = (a, b, type) => {
  if (type === "date") {
    const ta = parseUsageTime(a)?.getTime() ?? -Infinity;
    const tb = parseUsageTime(b)?.getTime() ?? -Infinity;
    return ta - tb;
  }
  if (type === "number") {
    return (Number(a) || 0) - (Number(b) || 0);
  }
  return String(a ?? "").localeCompare(String(b ?? ""), undefined, { sensitivity: "base" });
};

/**
 * Sort rows by a column. `columns` maps field name to a type ("string", "number", "date").
 * Returns a new array; the input is not mutated.
 */
export const sortUsageRows = (rows, sortConfig, columns) => {
  if (!Array.isArray(rows)) return [];
  if (!sortConfig?.field) return [...rows];
  const type = columns?.[sortConfig.field] || "string";
  const dir = sortConfig.direction === "asc" ? 1 : -1;
  return [...rows].sort((a, b) => dir * compare(a[sortConfig.field], b[sortConfig.field], type));
};

/** Flip direction when the same column is clicked again; new columns start descending
 *  for numeric and date columns (biggest / newest first) and ascending for text. */
export const nextSortConfig = (current, field, columns) => {
  if (current?.field === field) {
    return { field, direction: current.direction === "asc" ? "desc" : "asc" };
  }
  const type = columns?.[field] || "string";
  return { field, direction: type === "string" ? "asc" : "desc" };
};

export const formatTokens = (n) => (Number(n) || 0).toLocaleString();

export const formatCost = (n) => `$${(Number(n) || 0).toFixed(2)}`;
