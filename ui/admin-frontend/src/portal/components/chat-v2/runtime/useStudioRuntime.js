import { useMemo, useRef } from 'react';
import { useLocalRuntime, ExportedMessageRepository, generateId } from '@assistant-ui/react';
import { streamRun, fetchHistory, uploadFile, cancelRun } from '../api/chatV2Client';
import { toThreadMessageLike, buildRunBody } from './messageMapping';

/**
 * Builds an assistant-ui LocalRuntime on top of one v2 session (chat room or
 * agent, selected by `endpoints`).
 *
 * The runtime owns thread state; we supply three adapters:
 *  - chatModel: POSTs a turn and streams the reply (edit and regenerate are
 *    derived from the thread shape, see buildRunBody)
 *  - history: loads the persisted transcript; appends are a no-op because the
 *    backend already persists every turn
 *  - attachments: uploads files to the session so they become file_refs on
 *    the next message (chat rooms only)
 */
export const useStudioRuntime = ({ sessionId, endpoints, clientToolNames = [], onRunError }) => {
  // runtime message id -> backend row id, plus the backend's last row id.
  const idMap = useRef(new Map());
  const backendHead = useRef(null);
  const humanToolNames = useMemo(() => new Set(clientToolNames), [clientToolNames]);

  const adapter = useMemo(
    () => ({
      async *run({ messages, abortSignal, unstable_assistantMessageId, unstable_getMessage }) {
        const last = messages[messages.length - 1];
        const current = typeof unstable_getMessage === 'function' ? unstable_getMessage() : null;
        const body = buildRunBody(messages, idMap.current, backendHead.current, humanToolNames, current);
        // When resuming after client tool answers the runtime keeps the
        // parked message's parts itself and appends what we yield.

        let ids = null;
        let stream;
        try {
          stream = await streamRun(endpoints, sessionId, body, {
            signal: abortSignal,
            onData: (d) => {
              if (d.name === 'message-ids') ids = d.data;
            },
          });
        } catch (err) {
          if (err?.name === 'AbortError') {
            cancelRun(endpoints, sessionId);
            throw err;
          }
          onRunError?.(err);
          throw err;
        }

        const reader = stream.getReader();
        try {
          for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            yield {
              content: value.content,
              status: value.status,
              metadata: value.metadata,
            };
          }
        } catch (err) {
          if (err?.name === 'AbortError' || abortSignal.aborted) {
            cancelRun(endpoints, sessionId);
          }
          throw err;
        } finally {
          reader.releaseLock();
        }

        if (ids) {
          if (ids.user_message_id && last?.role === 'user') {
            idMap.current.set(last.id, ids.user_message_id);
          }
          if (ids.assistant_message_id && unstable_assistantMessageId) {
            idMap.current.set(unstable_assistantMessageId, ids.assistant_message_id);
          }
          backendHead.current = ids.assistant_message_id || ids.user_message_id || backendHead.current;
        }
      },
    }),
    [sessionId, endpoints, onRunError, humanToolNames],
  );

  const history = useMemo(
    () => ({
      async load() {
        const messages = await fetchHistory(endpoints, sessionId);
        idMap.current = new Map();
        messages.forEach((m) => idMap.current.set(String(m.id), String(m.id)));
        backendHead.current = messages.length ? String(messages[messages.length - 1].id) : null;
        return ExportedMessageRepository.fromArray(messages.map(toThreadMessageLike));
      },
      async append() {
        // The backend persists every turn itself.
      },
    }),
    [sessionId, endpoints],
  );

  const attachments = useMemo(() => {
    if (!endpoints.upload) return undefined;
    return {
      accept: '*/*',
      async add({ file }) {
        return {
          id: generateId(),
          type: 'file',
          name: file.name,
          contentType: file.type,
          file,
          status: { type: 'requires-action', reason: 'composer-send' },
        };
      },
      async send(attachment) {
        await uploadFile(endpoints, sessionId, attachment.file);
        return {
          ...attachment,
          status: { type: 'complete' },
          content: [{ type: 'data', name: 'file_ref', data: { filename: attachment.name } }],
        };
      },
      async remove() {},
    };
  }, [sessionId, endpoints]);

  return useLocalRuntime(adapter, {
    adapters: { history, ...(attachments ? { attachments } : {}) },
    unstable_humanToolNames: clientToolNames.length ? clientToolNames : undefined,
  });
};

export default useStudioRuntime;
