import portalPluginLoaderService from '../../services/portalPluginLoaderService';
import { makePluginToolRenderer } from './parts/PluginToolRenderer';
import { registerPluginToolRenderers } from './toolUiRegistry';

export const CHAT_TOOL_RENDERER_SLOT = 'chat.tool_renderer';

let loadPromise = null;

/**
 * Loads every plugin-provided chat tool renderer visible to the user
 * (portal UI registry entries in the `chat.tool_renderer` slot) and
 * registers them by tool name. Idempotent; failures are logged and the
 * default ToolCallCard keeps rendering those tools.
 */
export const loadPluginToolRenderers = () => {
  if (loadPromise) return loadPromise;
  loadPromise = (async () => {
    let registry;
    try {
      registry = await portalPluginLoaderService.initialize();
    } catch (err) {
      console.error('Failed to load the portal plugin registry', err);
      return {};
    }
    const entries = (registry || []).filter(
      (e) => e.is_active && e.slot_type === CHAT_TOOL_RENDERER_SLOT && e.route_pattern && e.component_tag,
    );
    const renderers = {};
    await Promise.all(
      entries.map(async (entry) => {
        try {
          const Wrapper = await portalPluginLoaderService.loadWebComponent(entry);
          renderers[entry.route_pattern] = makePluginToolRenderer(entry.component_tag, Wrapper);
        } catch (err) {
          console.error(`Failed to load chat tool renderer for ${entry.route_pattern}`, err);
        }
      }),
    );
    if (Object.keys(renderers).length > 0) registerPluginToolRenderers(renderers);
    return renderers;
  })();
  return loadPromise;
};

/** Test hook: forget the cached load so the next call fetches again. */
export const resetPluginToolRenderers = () => {
  loadPromise = null;
};

export default loadPluginToolRenderers;
