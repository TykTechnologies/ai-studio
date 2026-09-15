import portalPluginLoaderService from '../../services/portalPluginLoaderService';
import { loadPluginToolRenderers, resetPluginToolRenderers } from './pluginToolRenderers';
import { getToolUiRegistry } from './toolUiRegistry';

jest.mock('../../services/portalPluginLoaderService', () => ({
  __esModule: true,
  default: { initialize: jest.fn(), loadWebComponent: jest.fn() },
}));

describe('loadPluginToolRenderers', () => {
  beforeEach(() => {
    resetPluginToolRenderers();
    jest.clearAllMocks();
  });

  it('registers renderers for chat.tool_renderer entries and skips failures', async () => {
    portalPluginLoaderService.initialize.mockResolvedValue([
      { is_active: true, slot_type: 'chat.tool_renderer', route_pattern: 'getWeather', component_tag: 'x-weather', plugin_id: 3, entry_point: '/w.js' },
      { is_active: true, slot_type: 'chat.tool_renderer', route_pattern: 'broken', component_tag: 'x-broken', plugin_id: 3, entry_point: '/b.js' },
      { is_active: true, slot_type: 'portal_sidebar.section', route_pattern: '/page', component_tag: 'x-page', plugin_id: 3 },
      { is_active: false, slot_type: 'chat.tool_renderer', route_pattern: 'inactive', component_tag: 'x-off', plugin_id: 3 },
    ]);
    const Wrapper = () => null;
    portalPluginLoaderService.loadWebComponent.mockImplementation((entry) =>
      entry.component_tag === 'x-broken' ? Promise.reject(new Error('boom')) : Promise.resolve(Wrapper),
    );

    const renderers = await loadPluginToolRenderers();
    expect(Object.keys(renderers)).toEqual(['getWeather']);
    expect(portalPluginLoaderService.loadWebComponent).toHaveBeenCalledTimes(2);
    expect(getToolUiRegistry().getWeather).toBeDefined();
    expect(getToolUiRegistry().broken).toBeUndefined();

    // Cached: a second call does not fetch again.
    await loadPluginToolRenderers();
    expect(portalPluginLoaderService.initialize).toHaveBeenCalledTimes(1);
  });

  it('tolerates a registry failure', async () => {
    portalPluginLoaderService.initialize.mockRejectedValue(new Error('offline'));
    await expect(loadPluginToolRenderers()).resolves.toEqual({});
  });
});
