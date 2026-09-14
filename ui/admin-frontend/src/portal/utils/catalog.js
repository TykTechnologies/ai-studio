import { getVendorName, getVendorLogo } from "../../admin/utils/vendorLogos";
import {
  getVectorStoreName,
  getVectorStoreLogo,
  getEmbedderName,
} from "../../admin/utils/vendorUtils";
import { privacyLevelForScore, PRIVACY_LEVELS } from "../../admin/components/common/privacy/privacyLevels";
import { generateSlug } from "../../admin/components/wizards/quick-start/utils";
import { getConfig } from "../../config";

/**
 * The portal's unified catalog (UX review D4): one item shape for LLM
 * providers, data sources, tools and plugin resources, served by
 * GET /common/catalog. Everything here derives display facts from that
 * shape so the browse page, the overview and the detail pages agree on
 * labels, routes and ordering.
 */

export const CATALOG_TYPES = {
  LLM: "llm",
  DATASOURCE: "datasource",
  TOOL: "tool",
  PLUGIN_RESOURCE: "plugin_resource",
};

// Terminology from the September 2026 audit (M9): "LLM provider", "Data
// source", "Tool". Plugin resources take their type's own name.
const TYPE_LABELS = {
  [CATALOG_TYPES.LLM]: { singular: "LLM provider", plural: "LLM providers", slug: "llms", icon: "microchip-ai" },
  [CATALOG_TYPES.DATASOURCE]: { singular: "Data source", plural: "Data sources", slug: "datasources", icon: "layer-group" },
  [CATALOG_TYPES.TOOL]: { singular: "Tool", plural: "Tools", slug: "tools", icon: "screwdriver-wrench" },
  [CATALOG_TYPES.PLUGIN_RESOURCE]: { singular: "Resource", plural: "Resources", slug: "resources", icon: "puzzle-piece" },
};

export const typeLabel = (type, { plural = false } = {}) => {
  const entry = TYPE_LABELS[type];
  if (!entry) return plural ? "Assets" : "Asset";
  return plural ? entry.plural : entry.singular;
};

/** The singular label in running text ("LLM provider" keeps its capitals). */
export const typeLabelLower = (type) => {
  const label = typeLabel(type);
  return label.startsWith("LLM") ? label : label.toLowerCase();
};

export const typeIcon = (type) => TYPE_LABELS[type]?.icon || "puzzle-piece";

/** Route slug for a type: /portal/catalog/<slug>. */
export const typeSlug = (type) => TYPE_LABELS[type]?.slug || "resources";

/** Type for a route slug, or null. */
export const typeForSlug = (slug) =>
  Object.keys(TYPE_LABELS).find((type) => TYPE_LABELS[type].slug === slug) || null;

const attrs = (item) => item?.attributes || {};

/** The item's own type label: a plugin resource reads as its type name. */
export const itemTypeLabel = (item, options) => {
  const a = attrs(item);
  if (item?.type === CATALOG_TYPES.PLUGIN_RESOURCE && a.resource_type?.name) {
    return a.resource_type.name;
  }
  return typeLabel(item?.type, options);
};

/** Human name for the item's kind (vendor, store type, protocol, resource type). */
export const kindLabel = (item) => {
  const a = attrs(item);
  if (a.kind_label) return a.kind_label;
  switch (item?.type) {
    case CATALOG_TYPES.LLM:
      return getVendorName(a.kind) || a.kind || "";
    case CATALOG_TYPES.DATASOURCE:
      return getVectorStoreName(a.kind) || a.kind || "";
    case CATALOG_TYPES.TOOL:
      return a.kind ? a.kind.toUpperCase() : "";
    default:
      return a.kind || "";
  }
};

/** A small vendor/store logo for the kind, when one is bundled. */
export const kindLogo = (item) => {
  const a = attrs(item);
  switch (item?.type) {
    case CATALOG_TYPES.LLM:
      return getVendorLogo(a.kind);
    case CATALOG_TYPES.DATASOURCE:
      return getVectorStoreLogo(a.kind) || null;
    default:
      return null;
  }
};

export const embedderLabel = (code) => getEmbedderName(code) || code || "";

/** The browse route for a type, e.g. /portal/catalog/llms. */
export const browsePath = (type, item) => {
  if (!type) return "/portal/catalog";
  if (type === CATALOG_TYPES.PLUGIN_RESOURCE) {
    const rt = attrs(item).resource_type;
    if (rt) return `/portal/catalog/resources/${rt.plugin_id}/${rt.slug}`;
    return "/portal/catalog";
  }
  return `/portal/catalog/${typeSlug(type)}`;
};

/** The detail page for an item. */
export const detailPath = (item) => {
  const a = attrs(item);
  switch (item?.type) {
    case CATALOG_TYPES.LLM:
      return `/portal/catalog/llms/${item.id}`;
    case CATALOG_TYPES.DATASOURCE:
      return `/portal/catalog/datasources/${item.id}`;
    case CATALOG_TYPES.TOOL:
      return `/portal/catalog/tools/${item.id}`;
    case CATALOG_TYPES.PLUGIN_RESOURCE:
      return a.resource_type
        ? `/portal/catalog/resources/${a.resource_type.plugin_id}/${a.resource_type.slug}/${encodeURIComponent(item.id)}`
        : "/portal/catalog";
    default:
      return "/portal/catalog";
  }
};

/** The app builder with the item preselected. */
export const buildAppPath = (item) => {
  const a = attrs(item);
  switch (item?.type) {
    case CATALOG_TYPES.LLM:
      return `/portal/app/new?llm=${item.id}`;
    case CATALOG_TYPES.DATASOURCE:
      return `/portal/app/new?datasource=${item.id}`;
    case CATALOG_TYPES.TOOL:
      return `/portal/app/new?tool=${item.id}`;
    case CATALOG_TYPES.PLUGIN_RESOURCE:
      return a.resource_type
        ? `/portal/app/new?plugin_resource=${encodeURIComponent(`${a.resource_type.plugin_id}:${a.resource_type.slug}:${item.id}`)}`
        : "/portal/app/new";
    default:
      return "/portal/app/new";
  }
};

/** Primary action wording: data sources are "accessed", the rest "built on". */
export const buildActionLabel = (item) =>
  item?.type === CATALOG_TYPES.DATASOURCE ? "Get access" : "Build app";

/** Stable React key across types. */
export const itemKey = (item) => `${item?.type}:${item?.id}`;

/** Key of the catalog filter option an item's catalog maps to. */
export const catalogFilterKey = (type, catalogId) => `${type}:${catalogId}`;

// --- letter avatars ---------------------------------------------------------

// Hues spaced around the wheel; saturation and lightness are fixed so every
// avatar sits at the same contrast against white text.
const AVATAR_HUES = [212, 262, 292, 335, 12, 32, 152, 176, 196, 232, 100, 52];

/** Deterministic 32-bit hash (FNV-1a) of a string. */
export const hashString = (input) => {
  let hash = 0x811c9dc5;
  const text = String(input || "");
  for (let i = 0; i < text.length; i += 1) {
    hash ^= text.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash >>> 0;
};

/** The avatar colour for a name: the same name always gets the same hue. */
export const avatarColor = (seed) => {
  const hue = AVATAR_HUES[hashString(seed) % AVATAR_HUES.length];
  return {
    background: `linear-gradient(135deg, hsl(${hue}, 62%, 46%) 0%, hsl(${(hue + 28) % 360}, 66%, 38%) 100%)`,
    solid: `hsl(${hue}, 62%, 46%)`,
    foreground: "#FFFFFF",
  };
};

/**
 * Up to two initials for a name: first letters of the first two words
 * ("Acme OpenAI" → "AO"), or the first two characters of a single word
 * ("gpt4" → "GP"), digits allowed.
 */
export const initialsFor = (name) => {
  const words = String(name || "")
    .replace(/[^\p{L}\p{N}\s-]/gu, " ")
    .split(/[\s-]+/)
    .filter(Boolean);
  if (words.length === 0) return "?";
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase();
  return (words[0][0] + words[1][0]).toUpperCase();
};

// --- search, filter, sort ---------------------------------------------------

export const SORT_OPTIONS = [
  { value: "newest", label: "Newest first" },
  { value: "name", label: "Name (A–Z)" },
  { value: "privacy_asc", label: "Privacy level (low to high)" },
  { value: "privacy_desc", label: "Privacy level (high to low)" },
];

export const DEFAULT_SORT = "newest";

const searchable = (item) => {
  const a = attrs(item);
  return [
    a.name,
    a.short_description,
    a.long_description,
    kindLabel(item),
    itemTypeLabel(item),
    a.default_model,
    ...(a.allowed_models || []),
    ...(a.operations || []),
    ...(a.tags || []),
    ...(a.catalogs || []).map((c) => c.name),
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
};

/**
 * Applies the browse filters. `type` narrows to one type, `kind` to one
 * vendor/store/protocol/resource type, `privacy` to one named level,
 * `catalog` to one "<type>:<id>" option and `community` to community
 * submissions; `q` is a case-insensitive substring over everything shown.
 */
export const filterCatalogItems = (items, { q = "", type = "", kind = "", privacy = "", catalog = "", community = false } = {}) => {
  const term = q.trim().toLowerCase();
  const terms = term ? term.split(/\s+/) : [];
  return (items || []).filter((item) => {
    const a = attrs(item);
    if (type && item.type !== type) return false;
    if (kind && a.kind !== kind) return false;
    if (privacy) {
      const level = privacyLevelForScore(a.privacy_score);
      if (!level || level.key !== privacy) return false;
    }
    if (catalog) {
      const [catalogType, catalogId] = catalog.split(":");
      if (item.type !== catalogType) return false;
      if (!(a.catalogs || []).some((c) => String(c.id) === catalogId)) return false;
    }
    if (community && !a.community_submitted) return false;
    if (terms.length > 0) {
      const haystack = searchable(item);
      if (!terms.every((t) => haystack.includes(t))) return false;
    }
    return true;
  });
};

const createdAtMs = (item) => {
  const value = attrs(item).created_at;
  const ms = value ? new Date(value).getTime() : NaN;
  return Number.isNaN(ms) ? 0 : ms;
};

const nameOf = (item) => String(attrs(item).name || "").toLowerCase();

const privacyOf = (item) => {
  const score = attrs(item).privacy_score;
  return typeof score === "number" ? score : null;
};

/** Sorts a copy of the items. Newest first is the default (D4). */
export const sortCatalogItems = (items, sort = DEFAULT_SORT) => {
  const list = [...(items || [])];
  const byName = (x, y) => nameOf(x).localeCompare(nameOf(y));
  switch (sort) {
    case "name":
      return list.sort(byName);
    case "privacy_asc":
      return list.sort((x, y) => (privacyOf(x) ?? Infinity) - (privacyOf(y) ?? Infinity) || byName(x, y));
    case "privacy_desc":
      return list.sort((x, y) => (privacyOf(y) ?? -Infinity) - (privacyOf(x) ?? -Infinity) || byName(x, y));
    case "newest":
    default:
      return list.sort((x, y) => createdAtMs(y) - createdAtMs(x) || byName(x, y));
  }
};

/** Distinct kinds among the items, with labels, for the Kind filter. */
export const kindOptions = (items) => {
  const seen = new Map();
  (items || []).forEach((item) => {
    const kind = attrs(item).kind;
    if (!kind || seen.has(kind)) return;
    seen.set(kind, { value: kind, label: kindLabel(item) || kind, type: item.type });
  });
  return [...seen.values()].sort((x, y) => x.label.localeCompare(y.label));
};

export const privacyOptions = () => PRIVACY_LEVELS.map((level) => ({ value: level.key, label: level.label }));

/**
 * Formats a per-million-token price. Prices are dollar figures throughout the
 * product, so the plain "$2.50" form is used; another currency is spelled
 * out after the number. Sub-dollar prices keep up to four decimals so
 * "$0.15" and "$0.0006" both read correctly.
 */
export const formatPerMillion = (value, currency = "USD") => {
  if (typeof value !== "number" || Number.isNaN(value)) return "—";
  let text = value.toFixed(value < 1 ? 4 : 2);
  if (value < 1) {
    // "0.1500" -> "0.15", but never fewer than two decimals.
    text = text.replace(/0+$/, "");
    if (text.split(".")[1].length < 2) text = Number(text).toFixed(2);
  }
  if (!currency || currency === "USD") return `$${text}`;
  return `${text} ${currency}`;
};

// --- endpoints ---------------------------------------------------------------

/**
 * The OpenAI-compatible base URL for an LLM provider: `<proxyURL>/ai/<slug>/v1`.
 * proxyURL comes from /auth/config; the fallback mirrors AppDetailView so the
 * two pages never disagree about the host.
 */
export const openAICompatibleBaseUrl = (llmName) => {
  const config = getConfig();
  const proxyUrl =
    config.proxyURL || `${window.location.protocol}//${window.location.hostname}:9090`;
  return `${proxyUrl.replace(/\/+$/, "")}/ai/${generateSlug(llmName)}/v1`;
};
