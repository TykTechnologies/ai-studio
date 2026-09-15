import { useMemo, useRef } from 'react';
import { useLocalRuntime, ExportedMessageRepository, generateId } from '@assistant-ui/react';
import { streamRun, fetchHistory, uploadFile, cancelRun } from '../api/chatV2Client';
import { toThreadMessageLike, buildRunBody } from './messageMapping';

/**
 * Builds an assistant-ui LocalRuntime on top of one v2 chat session.
 *
 * The runtime owns thread state; we supply three adapters:
 *  - chatModel: POSTs a turn and streams the reply (edit and regenerate are
 *    derived from the thread shape, see buildRunBody)
 *  - history: loads the persisted transcript; appends are a no-op because the
 *    backend already persists every turn
 *  - attachments: uploads files to the session so they become file_refs on
 *    the next message
 */
export const useStudioRuntime = ({ sessionId, onRunError }) => {
  // runtime message id -> backend row id, plus the backend's last row id.
  const idMap = useRef(new Map());
  const backendHead = useRef(null);

  const adapter = useMemo(
    () => ({
      async *run({ messages, abortSignal, unstable_assistantMessageId }) {
        const last = messages[messages.length - 1];
        const body = buildRunBody(messages, idMap.current, backendHead.current);

        let ids = null;
        let stream;
        try {
          stream = await streamRun(sessionId, body, {
            signal: abortSignal,
            onData: (d) => {
              if (d.name === 'message-ids') ids = d.data;
            },
          });
        } catch (err) {
          if (err?.name === 'AbortError') {
            cancelRun(sessionId);
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
            cancelRun(sessionId);
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
    [sessionId, onRunError],
  );

  const history = useMemo(
    () => ({
      async load() {
        const messages = await fetchHistory(sessionId);
        idMap.current = new Map();
        messages.forEach((m) => idMap.current.set(String(m.id), String(m.id)));
        backendHead.current = messages.length ? String(messages[messages.length - 1].id) : null;
        return ExportedMessageRepository.fromArray(messages.map(toThreadMessageLike));
      },
      async append() {
        // The backend persists every turn itself.
      },
    }),
    [sessionId],
  );

  const attachments = useMemo(
    () => ({
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
        await uploadFile(sessionId, attachment.file);
        return {
          ...attachment,
          status: { type: 'complete' },
          content: [{ type: 'data', name: 'file_ref', data: { filename: attachment.name } }],
        };
      },
      async remove() {},
    }),
    [sessionId],
  );

  return useLocalRuntime(adapter, { adapters: { history, attachments } });
};

export default useStudioRuntime;
