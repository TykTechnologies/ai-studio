import { UIMessageStreamDecoder, AssistantMessageAccumulator } from 'assistant-stream';
import pubClient from '../../../../admin/utils/pubClient';
import { fetchCSRFToken } from '../../../../admin/utils/urlUtils';

/**
 * Thin client for the v2 chat API.
 *
 * Chat rooms and plugin agents expose the same session/run/history shape
 * under different paths, described by an `endpoints` object so the runtime
 * is shared. Plain JSON calls go through the shared axios client (cookies +
 * CSRF); the streamed turn uses fetch so the body can be piped through the
 * assistant-stream decoder.
 */

const baseUrl = () => pubClient.defaults.baseURL || '';

/** Endpoints for a chat room. */
export const chatEndpoints = (chatId) => ({
  kind: 'chat',
  sessions: `/common/chat/${chatId}/sessions`,
  history: (sid) => `/common/chat-sessions/${sid}/messages/v2`,
  runs: (sid) => `/common/chat-sessions/${sid}/runs`,
  cancel: (sid) => `/common/chat-sessions/${sid}/cancel`,
  upload: (sid) => `/common/chat-sessions/${sid}/upload`,
});

/** Endpoints for a plugin agent. */
export const agentEndpoints = (agentId) => ({
  kind: 'agent',
  sessions: `/common/agents/${agentId}/sessions`,
  history: (sid) => `/common/agent-sessions/${sid}/messages/v2`,
  runs: (sid) => `/common/agent-sessions/${sid}/runs`,
  cancel: (sid) => `/common/agent-sessions/${sid}/cancel`,
  upload: null,
});

export const createSession = async (endpoints, sessionId) => {
  const body = sessionId ? { session_id: sessionId } : {};
  const res = await pubClient.post(endpoints.sessions, body);
  return res.data;
};

export const fetchHistory = async (endpoints, sessionId) => {
  const res = await pubClient.get(endpoints.history(sessionId));
  return res.data?.messages || [];
};

export const uploadFile = async (endpoints, sessionId, file) => {
  if (!endpoints.upload) throw new Error('This chat does not accept file uploads');
  const formData = new FormData();
  formData.append('file', file);
  await pubClient.post(endpoints.upload(sessionId), formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
  return { name: file.name, size: file.size };
};

export const cancelRun = (endpoints, sessionId) =>
  pubClient.post(endpoints.cancel(sessionId)).catch(() => null);

// Chat-room only: mid-session tool / datasource management.
export const addTool = (sessionId, toolId) =>
  pubClient.post(`/common/chat-sessions/${sessionId}/tools`, { tool_id: String(toolId) });

export const removeTool = (sessionId, toolId) =>
  pubClient.delete(`/common/chat-sessions/${sessionId}/tools/${toolId}`);

export const addDatasource = (sessionId, datasourceId) =>
  pubClient.post(`/common/chat-sessions/${sessionId}/datasources`, {
    datasource_id: parseInt(datasourceId, 10),
  });

export const removeDatasource = (sessionId, datasourceId) =>
  pubClient.delete(`/common/chat-sessions/${sessionId}/datasources/${datasourceId}`);

/** Extracts a readable message from a failed fetch response. */
const readError = async (res) => {
  try {
    const data = await res.json();
    const first = data?.errors?.[0];
    if (first) return first.detail ? `${first.title}: ${first.detail}` : first.title;
    if (data?.error) return data.error;
  } catch (e) {
    // fall through
  }
  return `Request failed with status ${res.status}`;
};

/**
 * Streams one turn. Resolves to a ReadableStream of accumulated assistant
 * messages (assistant-stream's AssistantMessage), one per update.
 *
 * @param {object} endpoints
 * @param {string} sessionId
 * @param {object} body  V2RunRequest
 * @param {{signal?: AbortSignal, onData?: (d: {name: string, data: any}) => void}} options
 */
export const streamRun = async (endpoints, sessionId, body, { signal, onData } = {}) => {
  const csrf = await fetchCSRFToken();
  const headers = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  };
  if (csrf) headers['X-CSRF-Token'] = csrf;

  const res = await fetch(`${baseUrl()}${endpoints.runs(sessionId)}`, {
    method: 'POST',
    credentials: 'include',
    headers,
    body: JSON.stringify(body),
    signal,
  });

  if (res.status === 401) {
    localStorage.clear();
    if (!window.location.pathname.includes('/login')) {
      window.location.href = '/login';
    }
    throw new Error('Authentication failed. Please sign in again.');
  }
  if (!res.ok || !res.body) {
    throw new Error(await readError(res));
  }

  return res.body
    .pipeThrough(new UIMessageStreamDecoder({ onData }))
    .pipeThrough(new AssistantMessageAccumulator());
};
