import apiClient from '../utils/apiClient';
import pluginLoaderService from './pluginLoaderService';

jest.mock('../utils/apiClient', () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));

// Opening a plugin page must not write anything: POST /plugins/:id/ui/load
// needs plugins:write, so a read-only user got a "permission denied" toast on
// every plugin page, and an administrator's page view wrote the plugin row,
// an audit entry and a config-sync refresh.
describe('pluginLoaderService page views', () => {
  beforeEach(() => {
    jest.spyOn(console, 'log').mockImplementation(() => {});
    apiClient.get.mockResolvedValue({ data: 'customElements.define("x", class extends HTMLElement {})' });
    apiClient.post.mockResolvedValue({ data: {} });
    pluginLoaderService.loadedComponents.clear();
    pluginLoaderService.pluginRegistry.clear();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('loads a web component page without posting to the server', async () => {
    // Already registered, so the loader skips executing the script.
    customElements.define('viewer-plugin-page', class extends HTMLElement {});
    pluginLoaderService.pluginRegistry.set('7_viewer-plugin-page', {
      plugin_id: 7,
      component_tag: 'viewer-plugin-page',
      entry_point: '/ui/main.js',
      mount_config: { kind: 'webc' },
      is_active: true,
    });

    const Component = await pluginLoaderService.loadPlugin(7, 'viewer-plugin-page');

    expect(Component).toBeTruthy();
    expect(apiClient.get).toHaveBeenCalledWith('/plugins/assets/7/ui/main.js', expect.anything());
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  it('loads a module federation page without posting to the server', async () => {
    const Remote = () => null;
    jest.spyOn(pluginLoaderService, 'loadRemoteContainer').mockResolvedValue({
      get: async () => () => Remote,
    });
    pluginLoaderService.pluginRegistry.set('8_remote-page', {
      plugin_id: 8,
      component_tag: 'remote-page',
      mount_config: { kind: 'module-federation', remote: '/remote.js', exposed: './Page' },
      is_active: true,
    });

    const Component = await pluginLoaderService.loadPlugin(8, 'remote-page');

    expect(Component).toBe(Remote);
    expect(apiClient.post).not.toHaveBeenCalled();
  });
});
