import React, { useEffect, useRef } from 'react';
import { Box } from '@mui/material';
import ToolCallCard from './ToolCallCard';

/**
 * Wraps a plugin-provided web component (manifest slot `chat.tool_renderer`)
 * as a tool-call renderer.
 *
 * Contract with the custom element:
 *  - it receives the call as attributes `data-tool-name`, `data-args`
 *    (JSON), `data-result` (JSON, absent while running), `data-is-error`
 *    ("true"/"false") and `data-status` (running | complete | requires-action
 *    | incomplete), and as the property `toolCall` ({ toolName, args, result,
 *    isError, status });
 *  - to answer a human-in-the-loop call it dispatches a `tool-result`
 *    CustomEvent whose `detail` is the result (or `{ result, isError: true }`).
 *
 * The element is mounted inside a plain div so the wrapper can locate it and
 * listen for the event.
 */
export const makePluginToolRenderer = (tagName, Wrapper) => {
  const Renderer = ({ toolName, args, argsText, result, isError, status, addResult }) => {
    const hostRef = useRef(null);
    const addResultRef = useRef(addResult);
    addResultRef.current = addResult;

    useEffect(() => {
      const host = hostRef.current;
      const el = host?.querySelector(tagName);
      if (!el) return undefined;
      const statusType = status?.type || (result !== undefined ? 'complete' : 'running');
      el.setAttribute('data-tool-name', toolName || '');
      el.setAttribute('data-args', JSON.stringify(args ?? {}));
      if (result !== undefined) {
        el.setAttribute('data-result', typeof result === 'string' ? result : JSON.stringify(result));
      } else {
        el.removeAttribute('data-result');
      }
      el.setAttribute('data-is-error', isError ? 'true' : 'false');
      el.setAttribute('data-status', statusType);
      el.toolCall = { toolName, args, argsText, result, isError, status: statusType };
      if (typeof el.render === 'function') {
        try {
          el.render();
        } catch (e) {
          console.error(`Plugin tool renderer ${tagName} failed to render`, e);
        }
      }

      const onResult = (event) => {
        const detail = event?.detail;
        if (detail && typeof detail === 'object' && detail.isError === true) {
          addResultRef.current?.({ result: detail.result, isError: true });
        } else {
          addResultRef.current?.(detail);
        }
      };
      el.addEventListener('tool-result', onResult);
      return () => el.removeEventListener('tool-result', onResult);
    }, [toolName, args, argsText, result, isError, status]);

    return (
      <Box ref={hostRef} sx={{ my: 1 }} data-testid="plugin-tool-renderer">
        <Wrapper />
      </Box>
    );
  };
  Renderer.displayName = `PluginTool(${tagName})`;
  return Renderer;
};

/** Fallback used when a plugin renderer fails to load. */
export const FallbackToolRenderer = ToolCallCard;

export default makePluginToolRenderer;
