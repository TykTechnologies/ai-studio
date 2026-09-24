// Draft <-> API mapping and validation for Embedders, shared by the embedder
// page and the inline create dialog.

export const MODE_LINKED = "linked";
export const MODE_STANDALONE = "standalone";

// What a client sends back to keep a stored key it was shown redacted.
export const REDACTED_KEY = "[redacted]";

export const emptyDraft = () => ({
  name: "",
  description: "",
  mode: MODE_STANDALONE,
  llm_id: "",
  vendor: "",
  endpoint: "",
  api_key: "",
  model: "",
  privacy_score: 0,
});

export const draftFromAttributes = (a = {}) => ({
  name: a.name || "",
  description: a.description || "",
  mode: a.linked ? MODE_LINKED : MODE_STANDALONE,
  llm_id: a.llm_id ? String(a.llm_id) : "",
  vendor: a.linked ? "" : a.vendor || "",
  endpoint: a.linked ? "" : a.endpoint || "",
  // The API never returns a stored key; "[redacted]" keeps it on save.
  api_key: a.linked ? "" : a.api_key || "",
  model: a.model || "",
  privacy_score: a.privacy_score ?? 0,
});

/** The JSON:API attributes for a create or update. */
export const attributesFromDraft = (d) => {
  const base = {
    name: d.name.trim(),
    description: d.description,
    model: d.model.trim(),
  };
  if (d.mode === MODE_LINKED) {
    return { ...base, llm_id: Number(d.llm_id) };
  }
  return {
    ...base,
    llm_id: null,
    vendor: d.vendor,
    endpoint: d.endpoint.trim(),
    api_key: d.api_key,
    privacy_score: Number(d.privacy_score) || 0,
  };
};

/** Field errors for a draft; empty when it looks valid. */
export const validateDraft = (d) => {
  const errors = {};
  if (!d.name.trim()) errors.name = "Name is required";
  if (!d.model.trim()) errors.model = "Model is required";
  if (d.mode === MODE_LINKED) {
    if (!d.llm_id) errors.llm_id = "Pick the LLM provider to link to";
  } else {
    if (!d.vendor) errors.vendor = "Pick the API compatibility";
    const score = Number(d.privacy_score);
    if (!Number.isInteger(score) || score < 0 || score > 100) {
      errors.privacy_score = "Privacy level must be between 0 and 100";
    }
  }
  return errors;
};

/** Label and hint for the endpoint field, which Vertex uses differently. */
export const endpointField = (vendor) =>
  vendor === "vertex"
    ? { label: "Project and location", placeholder: "my-project:us-central1", helper: "Vertex takes the Google Cloud project and region as project:location." }
    : { label: "Endpoint URL", placeholder: "https://api.openai.com/v1", helper: "Leave empty to use the vendor's default endpoint." };

/** One line describing where an embedder sends text. */
export const describeEmbedder = (attrs = {}) => {
  const via = attrs.linked ? attrs.llm_name || `LLM #${attrs.llm_id}` : attrs.vendor;
  return [attrs.model, via].filter(Boolean).join(" · ");
};

/** Error detail from an API failure, or the fallback. */
export const apiErrorDetail = (error, fallback) =>
  error?.response?.data?.errors?.[0]?.detail || fallback;
