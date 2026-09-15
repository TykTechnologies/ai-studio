import { useMemo } from 'react';
import { makeHumanToolRenderer } from './parts/HumanToolCard';
import { useChatUi } from './ChatUiContext';

/**
 * Registry of dedicated renderers for tool calls, keyed by tool operation
 * name. Anything not listed falls back to ToolCallCard.
 *
 * Three sources are merged, later ones winning:
 *  1. built-ins declared here,
 *  2. plugin-provided renderers (manifest slot `chat.tool_renderer`),
 *  3. the session's client (human-in-the-loop) tools, rendered as
 *     approval / form cards.
 */
const builtins = {};

let pluginRenderers = {};
const listeners = new Set();

export const registerPluginToolRenderers = (renderers) => {
  pluginRenderers = { ...pluginRenderers, ...renderers };
  listeners.forEach((fn) => fn());
};

export const subscribeToolRenderers = (fn) => {
  listeners.add(fn);
  return () => listeners.delete(fn);
};

export const getToolUiRegistry = (clientTools = []) => {
  const human = {};
  clientTools.forEach((info) => {
    if (info?.name) human[info.name] = makeHumanToolRenderer(info);
  });
  return { ...builtins, ...pluginRenderers, ...human };
};

/** React hook returning the merged registry for the current session. */
export const useToolUiRegistry = () => {
  const { session } = useChatUi();
  const clientTools = session?.client_tools;
  return useMemo(() => getToolUiRegistry(clientTools || []), [clientTools]);
};

export default builtins;
