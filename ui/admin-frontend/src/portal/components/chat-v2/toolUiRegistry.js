import { useMemo } from 'react';

/**
 * Registry of dedicated renderers for tool calls, keyed by tool operation
 * name. Anything not listed falls back to ToolCallCard.
 *
 * Built-ins are registered here; plugin-provided renderers (manifest slot
 * `chat.tool_renderer`) are merged in by the page once loaded.
 */
const builtins = {};

let pluginRenderers = {};
const listeners = new Set();

export const registerPluginToolRenderers = (renderers) => {
  pluginRenderers = { ...pluginRenderers, ...renderers };
  listeners.forEach((fn) => fn());
};

export const getToolUiRegistry = () => ({ ...builtins, ...pluginRenderers });

/** React hook returning the merged registry (stable while unchanged). */
export const useToolUiRegistry = () => useMemo(() => getToolUiRegistry(), []);

export default builtins;
