// Form model for a Semantic Router: the editable draft the form holds, the
// conversion to and from the API's attributes, and the client-side checks
// that mirror the server's obvious validation rules (pkg/semanticrouting
// validate.go). The server stays the authority; these only catch the
// mistakes that would otherwise cost a round trip.

/** LLM vendors that serve embeddings; the embedding picker offers only these. */
export const EMBEDDING_VENDORS = ["openai", "ollama", "google_ai", "vertex", "huggingface"];

export const supportsEmbeddings = (vendor) => EMBEDDING_VENDORS.includes(vendor);

export const ROUTE_NAME_PATTERN = /^[a-z0-9]+([-_][a-z0-9]+)*$/;
export const SLUG_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)*$/;

/** "auto" is the model a client sends to let the router classify. */
export const RESERVED_ROUTE_NAME = "auto";

export const DEFAULT_THRESHOLD = 0.75;
export const DEFAULT_AFFINITY_HEADER = "X-Tyk-Session-Id";

export const TARGET_LLM = "llm";
export const TARGET_MODEL_ROUTER = "model_router";

export const MODE_LABELS = { enforce: "Enforce", shadow: "Shadow" };
export const INPUT_SCOPE_LABELS = {
  last_user: "Last user message",
  all_user: "All user messages",
};
export const JUDGE_WHEN_LABELS = {
  low_confidence: "Only when the other stages are unsure",
  always: "On every request",
};
export const REASON_LABELS = {
  keyword: "Keyword match",
  embedding: "Embedding similarity",
  judge: "LLM judge",
  default: "Default route",
  explicit: "Route named in the model",
  affinity: "Session affinity",
  classifier_error: "Classifier error (default route)",
};

const str = (value) => (value === null || value === undefined ? "" : String(value));

export const emptyRoute = () => ({
  name: "",
  description: "",
  priority: 0,
  keywords: [],
  utterancesText: "",
  threshold: "",
  target: { type: TARGET_LLM, llm_id: "", model_router_id: "", model: "" },
});

export const emptyDraft = () => ({
  name: "",
  slug: "",
  description: "",
  short_description: "",
  long_description: "",
  logo_url: "",
  active: false,
  namespace: "",
  settings: {
    mode: "enforce",
    allow_explicit_route: false,
    input_scope: "last_user",
    max_input_chars: "",
    embedding: { llm_id: "", model: "", timeout_ms: "" },
    judge: { enabled: false, llm_id: "", model: "", timeout_ms: "", when: "low_confidence" },
    affinity: { enabled: false, header: DEFAULT_AFFINITY_HEADER, ttl_seconds: "" },
    default_route: "",
  },
  routes: [],
});

/** The form draft for a router's API attributes. */
export const draftFromAttributes = (attributes = {}) => {
  const base = emptyDraft();
  const s = attributes.settings || {};
  const judgeRef = s.judge?.model_ref || {};
  return {
    ...base,
    name: attributes.name || "",
    slug: attributes.slug || "",
    description: attributes.description || "",
    short_description: attributes.short_description || "",
    long_description: attributes.long_description || "",
    logo_url: attributes.logo_url || "",
    active: Boolean(attributes.active),
    namespace: attributes.namespace || "",
    settings: {
      mode: s.mode || "enforce",
      allow_explicit_route: Boolean(s.allow_explicit_route),
      input_scope: s.input_scope || "last_user",
      max_input_chars: s.max_input_chars ? String(s.max_input_chars) : "",
      embedding: {
        llm_id: s.embedding?.llm_id ? String(s.embedding.llm_id) : "",
        model: s.embedding?.model || "",
        timeout_ms: s.embedding?.timeout_ms ? String(s.embedding.timeout_ms) : "",
      },
      judge: {
        enabled: Boolean(s.judge?.enabled),
        llm_id: judgeRef.llm_id ? String(judgeRef.llm_id) : "",
        model: judgeRef.model || "",
        timeout_ms: judgeRef.timeout_ms ? String(judgeRef.timeout_ms) : "",
        when: s.judge?.when || "low_confidence",
      },
      affinity: {
        enabled: Boolean(s.affinity?.enabled),
        header: s.affinity?.header || DEFAULT_AFFINITY_HEADER,
        ttl_seconds: s.affinity?.ttl_seconds ? String(s.affinity.ttl_seconds) : "",
      },
      default_route: s.default_route || "",
    },
    routes: (attributes.routes || []).map((route) => ({
      name: route.name || "",
      description: route.description || "",
      priority: route.priority || 0,
      keywords: (route.keywords || []).map((k) => ({ pattern: k.pattern || "", regex: Boolean(k.regex) })),
      utterancesText: (route.utterances || []).join("\n"),
      threshold: route.threshold ? String(route.threshold) : "",
      target: {
        type: route.target?.type || TARGET_LLM,
        llm_id: str(route.target?.llm_id || ""),
        model_router_id: str(route.target?.model_router_id || ""),
        model: route.target?.model || "",
      },
    })),
  };
};

const toInt = (value) => {
  const n = parseInt(value, 10);
  return Number.isNaN(n) ? 0 : n;
};

/** One example per non-blank line. */
export const utterancesOf = (text) =>
  String(text || "")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);

const targetPayload = (target) =>
  target.type === TARGET_MODEL_ROUTER
    ? { type: TARGET_MODEL_ROUTER, model_router_id: toInt(target.model_router_id), model: target.model.trim() }
    : { type: TARGET_LLM, llm_id: toInt(target.llm_id), model: target.model.trim() };

/** The router's configuration in the shape the API takes (settings + routes). */
export const configFromDraft = (draft) => {
  const s = draft.settings;
  const settings = {
    mode: s.mode,
    allow_explicit_route: Boolean(s.allow_explicit_route),
    input_scope: s.input_scope,
    judge: {
      enabled: Boolean(s.judge.enabled),
      model_ref: {
        llm_id: toInt(s.judge.llm_id),
        model: s.judge.model.trim(),
        ...(toInt(s.judge.timeout_ms) > 0 && { timeout_ms: toInt(s.judge.timeout_ms) }),
      },
      when: s.judge.when,
    },
    affinity: {
      enabled: Boolean(s.affinity.enabled),
      header: s.affinity.header.trim(),
      ...(toInt(s.affinity.ttl_seconds) > 0 && { ttl_seconds: toInt(s.affinity.ttl_seconds) }),
    },
    default_route: s.default_route,
  };
  if (toInt(s.max_input_chars) > 0) settings.max_input_chars = toInt(s.max_input_chars);
  // No embedding LLM means no embedding stage; the setting is left out.
  if (s.embedding.llm_id) {
    settings.embedding = {
      llm_id: toInt(s.embedding.llm_id),
      model: s.embedding.model.trim(),
      ...(toInt(s.embedding.timeout_ms) > 0 && { timeout_ms: toInt(s.embedding.timeout_ms) }),
    };
  }
  const routes = draft.routes.map((route) => {
    const threshold = parseFloat(route.threshold);
    return {
      name: route.name.trim(),
      description: route.description.trim(),
      priority: toInt(route.priority),
      keywords: route.keywords
        .filter((k) => k.pattern.trim())
        .map((k) => ({ pattern: k.pattern.trim(), regex: Boolean(k.regex) })),
      utterances: utterancesOf(route.utterancesText),
      ...(threshold > 0 && { threshold }),
      target: targetPayload(route.target),
    };
  });
  return { settings, routes };
};

/** The full attributes object for POST/PATCH (and the draft test). */
export const attributesFromDraft = (draft) => ({
  name: draft.name.trim(),
  slug: draft.slug.trim(),
  description: draft.description,
  short_description: draft.short_description,
  long_description: draft.long_description,
  logo_url: draft.logo_url,
  active: Boolean(draft.active),
  namespace: draft.namespace,
  ...configFromDraft(draft),
});

/** Names of the routes that have one, in order. */
export const routeNames = (routes = []) =>
  routes.map((route) => (route.name || "").trim()).filter(Boolean);

/**
 * Client-side checks. Returns an errors object keyed by field (`name`,
 * `slug`, `routes`, `route_<i>_name`, `route_<i>_target`, `route_<i>_model`,
 * `route_<i>_threshold`, `default_route`, `embedding`, `judge`); empty when
 * the draft looks valid. `llms` (JSON:API rows) lets the embedding vendor be
 * checked.
 */
export const validateDraft = (draft, { llms = [] } = {}) => {
  const errors = {};
  if (!draft.name.trim()) errors.name = "Name is required";
  const slug = draft.slug.trim();
  if (!slug) errors.slug = "Slug is required";
  else if (!SLUG_PATTERN.test(slug)) errors.slug = "Use lowercase letters, digits and single hyphens";

  if (draft.routes.length === 0) errors.routes = "At least one route is required";

  const seen = new Set();
  let anyUtterances = false;
  draft.routes.forEach((route, i) => {
    const name = route.name.trim();
    if (!name) errors[`route_${i}_name`] = "Route name is required";
    else if (name === RESERVED_ROUTE_NAME) errors[`route_${i}_name`] = `"${RESERVED_ROUTE_NAME}" is reserved`;
    else if (!ROUTE_NAME_PATTERN.test(name)) errors[`route_${i}_name`] = "Use lowercase letters and digits, joined by - or _";
    else if (seen.has(name)) errors[`route_${i}_name`] = "Route names must be unique";
    seen.add(name);

    const { target } = route;
    if (target.type === TARGET_MODEL_ROUTER ? !target.model_router_id : !target.llm_id) {
      errors[`route_${i}_target`] =
        target.type === TARGET_MODEL_ROUTER ? "Pick a model router" : "Pick an LLM provider";
    }
    if (!target.model.trim()) {
      errors[`route_${i}_model`] =
        target.type === TARGET_MODEL_ROUTER ? "The model alias sent to the router is required" : "Model is required";
    }
    if (route.threshold !== "" && route.threshold !== null && route.threshold !== undefined) {
      const t = Number(route.threshold);
      if (Number.isNaN(t) || t < 0 || t > 1) errors[`route_${i}_threshold`] = "Between 0 and 1";
    }
    if (utterancesOf(route.utterancesText).length > 0) anyUtterances = true;
  });

  const def = draft.settings.default_route;
  if (!def) errors.default_route = "Pick the default route";
  else if (!routeNames(draft.routes).includes(def)) errors.default_route = "The default route must be one of the routes";

  const emb = draft.settings.embedding;
  if (anyUtterances && (!emb.llm_id || !emb.model.trim())) {
    errors.embedding = "Routes with examples need an embedding LLM and model";
  } else if (emb.llm_id) {
    const llm = llms.find((l) => String(l.id) === String(emb.llm_id));
    const vendor = llm?.attributes?.vendor;
    if (llm && vendor && !supportsEmbeddings(vendor)) {
      errors.embedding = `${llm.attributes.name} (${vendor}) does not provide embeddings`;
    } else if (!emb.model.trim()) {
      errors.embedding = "Embedding model is required";
    }
  }

  const judge = draft.settings.judge;
  if (judge.enabled && (!judge.llm_id || !judge.model.trim())) {
    errors.judge = "The judge needs an LLM and a model";
  }
  return errors;
};

/** The first error detail from an API error response, if any. */
export const apiErrorDetail = (error) =>
  error?.response?.data?.errors?.[0]?.detail ||
  error?.response?.data?.message ||
  error?.response?.data?.error ||
  "";
