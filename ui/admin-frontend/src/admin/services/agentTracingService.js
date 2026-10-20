import apiClient from "../utils/apiClient";

// Agent Call Tracing read API (Enterprise). The sessions list always goes
// through the POST query endpoint so client-supplied identifiers (end user,
// external session id) and name filters never reach a URL or an access log;
// the in-session find term is likewise sent in a body.

const BASE = "/agent-tracing";

const agentTracingService = {
  listSessions: (query) => apiClient.post(`${BASE}/sessions/query`, query).then((r) => r.data),
  getSession: (sessionId) => apiClient.get(`${BASE}/sessions/${sessionId}`).then((r) => r.data),
  listSpans: (sessionId, params = {}) =>
    apiClient.get(`${BASE}/sessions/${sessionId}/spans`, { params }).then((r) => r.data),
  // Trace and span ids are unique only within an App, so both need it.
  getTrace: (traceId, appId, params = {}) =>
    apiClient.get(`${BASE}/traces/${traceId}`, { params: { ...params, app_id: appId } }).then((r) => r.data),
  getSpan: (spanId, appId) =>
    apiClient.get(`${BASE}/spans/${spanId}`, { params: { app_id: appId } }).then((r) => r.data),
  // Captured content of one part (audited server-side, jyx.26).
  getPayload: (spanId, appId, part, maxBytes) =>
    apiClient
      .get(`${BASE}/spans/${spanId}/payloads/${part}`, { params: { app_id: appId, ...(maxBytes ? { max_bytes: maxBytes } : {}) } })
      .then((r) => r.data),
  // Synchronous export (jyx.27): the file as a Blob.
  exportSession: (sessionId, format, content) =>
    apiClient
      .get(`${BASE}/sessions/${sessionId}/export`, { params: { format, content: Boolean(content) }, responseType: "blob" })
      .then((r) => r.data),
  search: (sessionId, q, limit) =>
    apiClient.post(`${BASE}/sessions/${sessionId}/search`, { q, ...(limit ? { limit } : {}) }).then((r) => r.data),
};

export default agentTracingService;
