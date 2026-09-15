import { useEffect, useMemo, useState } from 'react';
import { makeHumanToolRenderer } from './parts/HumanToolCard';
import { makePresentRenderer } from './parts/GenerativeUiCard';
import { useChatUi } from './ChatUiContext';

/**
 * Registry of dedicated renderers for tool calls, keyed by tool operation
 * name. Anything not listed falls back to ToolCallCard.
 *
 * Three sources are merged, later ones winning:
 *  1. built-ins declared here,
 *  2. plugin-provided renderers (manifest slot `chat.tool_renderer`),
 *  3. the session's client tools, rendered as approval / form cards
 *     (human-in-the-loop) or as generative UI (kind "present").
 */
const builtins = {};

let pluginRenderers = {};
let pluginVersion = 0;
const listeners = new Set();

export const registerPluginToolRenderers = (renderers) => {
  pluginRenderers = { ...pluginRenderers, ...renderers };
  pluginVersion += 1;
  listeners.forEach((fn) => fn(pluginVersion));
};

export const subscribeToolRenderers = (fn) => {
  listeners.add(fn);
  return () => listeners.delete(fn);
};

export const getToolUiRegistry = (clientTools = []) => {
  const human = {};
  clientTools.forEach((info) => {
    if (!info?.name) return;
    human[info.name] = info.ui?.kind === 'present' ? makePresentRenderer(info) : makeHumanToolRenderer(info);
  });
  return { ...builtins, ...pluginRenderers, ...human };
};

/** React hook returning the merged registry for the current session. */
export const useToolUiRegistry = () => {
  const { session } = useChatUi();
  const clientTools = session?.client_tools;
  const [version, setVersion] = useState(pluginVersion);
  useEffect(() => subscribeToolRenderers(setVersion), []);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(() => getToolUiRegistry(clientTools || []), [clientTools, version]);
};

export default builtins;
