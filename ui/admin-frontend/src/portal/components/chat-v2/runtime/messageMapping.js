/**
 * Pure helpers that translate between the v2 chat API and assistant-ui's
 * message model. Kept free of React so they can be unit tested directly.
 */

export const ROOT_MESSAGE_ID = 'root';

/** Converts one message from GET .../messages/v2 into a ThreadMessageLike. */
export const toThreadMessageLike = (msg) => {
  const content = (msg.parts || []).map((part) => {
    switch (part.type) {
      case 'text':
        return { type: 'text', text: part.text || '' };
      case 'tool-call':
        return {
          type: 'tool-call',
          toolCallId: part.toolCallId,
          toolName: part.toolName || 'tool',
          args: part.args && typeof part.args === 'object' ? part.args : {},
          argsText: part.argsText || JSON.stringify(part.args || {}),
          ...(part.result !== undefined ? { result: part.result } : {}),
          ...(part.isError ? { isError: true } : {}),
        };
      case 'data':
        return { type: 'data', name: part.name, data: part.data };
      default:
        return null;
    }
  }).filter(Boolean);

  const like = {
    id: String(msg.id),
    role: msg.role === 'assistant' ? 'assistant' : 'user',
    content,
  };
  if (msg.created_at) like.createdAt = new Date(msg.created_at);
  if (like.role === 'assistant') like.status = { type: 'complete', reason: 'stop' };
  return like;
};

/** The text a user typed, ignoring context and attachment parts. */
export const extractText = (message) =>
  (message?.content || [])
    .filter((p) => p.type === 'text')
    .map((p) => p.text)
    .join('\n')
    .trim();

/** File names uploaded for this message (see the attachment adapter). */
export const extractFileRefs = (message) => {
  const refs = [];
  const collect = (parts) => {
    (parts || []).forEach((p) => {
      if (p?.type === 'data' && p.name === 'file_ref' && p.data?.filename) {
        refs.push(p.data.filename);
      }
    });
  };
  (message?.attachments || []).forEach((a) => collect(a.content));
  collect(message?.content);
  return Array.from(new Set(refs));
};

/**
 * True when the assistant message the runtime is about to continue holds
 * answered client (human-in-the-loop) tool calls.
 */
export const isResume = (currentAssistant, humanToolNames = new Set()) =>
  Boolean(
    currentAssistant &&
      humanToolNames.size > 0 &&
      (currentAssistant.content || []).some(
        (p) => p.type === 'tool-call' && p.result !== undefined && humanToolNames.has(p.toolName),
      ),
  );

/**
 * Decides what to ask the backend for, given the thread assistant-ui wants
 * to run and what we know about the persisted history.
 *
 * @param {readonly object[]} messages  thread up to and including the turn to run
 * @param {Map<string,string>} idMap    runtime message id -> backend row id
 * @param {string|null} backendHead     backend row id of the last persisted message
 */
export const buildRunBody = (messages, idMap, backendHead, humanToolNames = new Set(), currentAssistant = null) => {
  const last = messages[messages.length - 1];
  if (!last) throw new Error('nothing to run');

  if (isResume(currentAssistant, humanToolNames)) {
    // Resuming a turn parked on client (human-in-the-loop) tools: send the
    // answers the user gave in the tool cards. The runtime hands us the
    // parked assistant message separately (unstable_getMessage), not as the
    // last thread message.
    const tool_results = currentAssistant.content
      .filter((p) => p.type === 'tool-call' && p.result !== undefined && humanToolNames.has(p.toolName))
      .map((p) => ({
        tool_call_id: p.toolCallId,
        result: typeof p.result === 'string' ? p.result : JSON.stringify(p.result),
        is_error: Boolean(p.isError),
      }));
    return { tool_results };
  }

  if (last.role === 'user' && idMap.has(last.id)) {
    // Reload of an assistant reply: the user turn already exists server-side.
    return { regenerate: true, after_message_id: idMap.get(last.id) };
  }

  const body = { message: extractText(last), file_refs: extractFileRefs(last) };

  const prev = messages[messages.length - 2];
  if (!prev) {
    // First message of the thread; if the backend already has content this
    // is an edit of the opening message.
    if (backendHead) body.after_message_id = ROOT_MESSAGE_ID;
    return body;
  }

  const prevBackend = idMap.get(prev.id);
  if (prevBackend !== undefined && backendHead !== null && prevBackend !== backendHead) {
    // The message we are appending after is not the backend's tail: an edit.
    body.after_message_id = prevBackend;
  }
  return body;
};
