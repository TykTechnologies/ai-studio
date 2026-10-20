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
  getTrace: (traceId, params = {}) => apiClient.get(`${BASE}/traces/${traceId}`, { params }).then((r) => r.data),
  getSpan: (spanId) => apiClient.get(`${BASE}/spans/${spanId}`).then((r) => r.data),
  search: (sessionId, q, limit) =>
    apiClient.post(`${BASE}/sessions/${sessionId}/search`, { q, ...(limit ? { limit } : {}) }).then((r) => r.data),
};

export default agentTracingService;
