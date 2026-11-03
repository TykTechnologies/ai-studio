import apiClient from "../utils/apiClient";

// Guardrail exceptions of an app (features/GuardrailExceptions.md): soften a
// guardrail filter's action on a verdict for one app, filter, detector and
// channel. Listing needs apps:read; proposing, editing and retiring
// apps:write; deleting apps:delete; approving a full administrator. A full
// administrator's own exception is approved as it is made.

const base = (appId) => `/apps/${appId}/guardrail-exceptions`;

// listGuardrailExceptions returns the exceptions and the rules the API
// applies (meta: max_approved, actions per channel, floors per capability).
export const listGuardrailExceptions = async (appId) => {
  const response = await apiClient.get(base(appId));
  return { exceptions: response.data?.data || [], meta: response.data?.meta || null };
};

export const createGuardrailException = async (appId, exception) => {
  const response = await apiClient.post(base(appId), exception);
  return response.data?.data;
};

export const updateGuardrailException = async (appId, id, changes) => {
  const response = await apiClient.patch(`${base(appId)}/${id}`, changes);
  return response.data?.data;
};

export const deleteGuardrailException = async (appId, id) => {
  await apiClient.delete(`${base(appId)}/${id}`);
};

// approveGuardrailException approves the exception as the administrator
// reviewed it: contentHash is the content_hash the API returned. The API
// refuses an approval of an exception that changed since (409,
// isGuardrailExceptionChanged).
export const approveGuardrailException = async (appId, id, contentHash) => {
  if (!contentHash) throw new Error("The reviewed exception's content hash is missing; reload it");
  const response = await apiClient.post(`${base(appId)}/${id}/approve`, { content_hash: contentHash });
  return response.data?.data;
};

export const retireGuardrailException = async (appId, id) => {
  const response = await apiClient.post(`${base(appId)}/${id}/retire`);
  return response.data?.data;
};

// listFilterGuardrailExceptions lists every app's exceptions of one filter.
export const listFilterGuardrailExceptions = async (filterId) => {
  const response = await apiClient.get(`/filters/${filterId}/exceptions`);
  return response.data?.data || [];
};

/** isGuardrailExceptionChanged: an approval refused because the exception changed since it was reviewed. */
export const isGuardrailExceptionChanged = (error) =>
  error?.response?.status === 409 && error?.response?.data?.errors?.[0]?.title === "Guardrail Exception Changed";

export const CHANNEL_LABELS = {
  text_user: "The app's own messages",
  text_system: "The system prompt",
  tool_result: "Tool results",
  tool_call: "Tool calls",
  tool_definition: "Tool definitions",
};

export const channelLabel = (channel) => CHANNEL_LABELS[channel] || channel;

export const isToolChannel = (channel) => ["tool_result", "tool_call", "tool_definition"].includes(channel);

export const ACTION_LABELS = {
  log: "Log only",
  quarantine: "Remove the flagged part",
  remove: "Remove the tool definition",
  alert: "Log and raise event",
};

export const exceptionActionLabel = (action) => ACTION_LABELS[action] || action;

const RANK = { log: 1, alert: 1, redact: 2, quarantine: 3, remove: 3, block: 4 };

// allowedActions are the actions an exception on channel may soften to for
// an app of the (effective) capability, softest first: what the channel
// supports, no softer than the floor (guardrails.CheckException). Empty when
// the channel cannot be softened for that capability.
export const allowedActions = (meta, capability, channel) => {
  const actions = meta?.actions?.[channel] || [];
  const floor = meta?.floors?.[capability || "acting"]?.[channel] || "";
  if (!floor) return [];
  return actions.filter((a) => (RANK[a] || 0) >= (RANK[floor] || 0));
};

// expiresAt is when an approved exception lapses back to proposed: its
// expiry window after it last applied (or after its approval when it has
// not applied yet). null for an exception that is not approved.
export const exceptionExpiresAt = (e) => {
  if (!e || e.status !== "approved" || !e.expires_after_days) return null;
  // From the later of its approval and its last hit (models.Lapsed).
  let from = e.last_hit_at;
  if (!from || (e.approved_at && new Date(e.approved_at) > new Date(from))) from = e.approved_at;
  if (!from) return null;
  return new Date(new Date(from).getTime() + e.expires_after_days * 24 * 3600 * 1000);
};

export { apiErrorMessage } from "./promptTemplateService";
