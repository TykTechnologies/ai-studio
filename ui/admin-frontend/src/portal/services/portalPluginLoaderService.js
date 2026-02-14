import pubClient from '../../admin/utils/pubClient';

/**
 * Portal Plugin Loader Service - Handles dynamic loading of plugin UI components for the AI Portal.
 * This mirrors the admin pluginLoaderService but uses portal-scoped endpoints (/common/plugins/...)
 * which are accessible to all authenticated users (not just admins).
 */
class PortalPluginLoaderService {
  constructor() {
    this.loadedComponents = new Map();
    this.pluginRegistry = new Map();
    this.isInitialized = false;
  }

  /**
   * Initialize the portal plugin loader by fetching the portal UI registry
   */
  async initialize() {
    try {
      const response = await pubClient.get('/common/plugins/portal-ui-registry');
      const registry = response.data.data || [];

      // Build plugin registry
      registry.forEach(entry => {
        if (entry.is_active) {
          const compositeKey = `${entry.plugin_id}_${entry.component_tag}`;
          this.pluginRegistry.set(compositeKey, entry);
        }
      });

      this.isInitialized = true;
      return registry;
    } catch (error) {
      console.error('Failed to initialize portal plugin loader:', error);
      // Don't throw - portal should work even without plugins
      this.isInitialized = true;
      return [];
    }
  }

  /**
   * Load a Web Component plugin dynamically
   */
  async loadWebComponent(pluginEntry) {
    const component_tag = pluginEntry.component_tag;
    const entry_point = pluginEntry.entry_point;
    const plugin_id = pluginEntry.plugin_id;
    const mount_config = pluginEntry.mount_config;

    if (this.loadedComponents.has(component_tag)) {
      return this.loadedComponents.get(component_tag);
    }

    try {
      // Fetch JS content via portal-accessible asset endpoint
      const assetPath = `/common/plugins/assets/${plugin_id}${entry_point}`;

      const response = await pubClient.get(assetPath, {
        headers: { 'Accept': 'application/javascript' }
      });

      if (!response.data) {
        throw new Error(`No content received from ${assetPath}`);
      }

      // Register the custom element if not already done
      if (!customElements.get(component_tag)) {
        const script = document.createElement('script');
        script.type = 'text/javascript';
        script.text = response.data;
        document.head.appendChild(script);

        await new Promise(resolve => setTimeout(resolve, 100));

        if (!customElements.get(component_tag)) {
          throw new Error(`Web Component ${component_tag} was not registered after executing JavaScript`);
        }
      }

      // Create React wrapper with portal-scoped plugin API
      const WebComponentWrapper = this.createWebComponentWrapper(component_tag, mount_config, plugin_id);
      this.loadedComponents.set(component_tag, WebComponentWrapper);

      return WebComponentWrapper;
    } catch (error) {
      console.error(`Failed to load portal Web Component ${component_tag}:`, error);
      throw error;
    }
  }

  /**
   * Create a React wrapper for a Web Component with portal plugin API injection
   */
  createWebComponentWrapper(tagName, mountConfig = {}, pluginId = null) {
    const React = window.React || require('react');
    const { useEffect, useRef } = React;

    return React.forwardRef((props, ref) => {
      const elementRef = useRef(null);

      useEffect(() => {
        const element = elementRef.current;
        if (!element) return;

        // Inject portal-scoped plugin API (routes through /common/plugins/:id/portal-rpc/:method)
        if (pluginId) {
          element.portalPluginAPI = {
            call: async (method, payload = {}) => {
              try {
                const response = await pubClient.post(
                  `/common/plugins/${pluginId}/portal-rpc/${method}`,
                  payload
                );
                return response.data.data;
              } catch (error) {
                console.error(`Portal plugin RPC call failed: ${method}`, error);
                throw error;
              }
            }
          };
        }

        // Set props from mount config
        const configProps = mountConfig.props || {};
        Object.entries(configProps).forEach(([key, value]) => {
          const attrName = key.replace(/([A-Z])/g, '-$1').toLowerCase();
          element.setAttribute(`data-${attrName}`, typeof value === 'string' ? value : JSON.stringify(value));
        });

        // Set component props
        Object.entries(props).forEach(([key, value]) => {
          if (key !== 'children') {
            element.setAttribute(`data-${key}`, typeof value === 'string' ? value : JSON.stringify(value));
          }
        });

        if (ref) {
          if (typeof ref === 'function') {
            ref(element);
          } else {
            ref.current = element;
          }
        }
      }, [props, ref]);

      return React.createElement(tagName, {
        ref: elementRef,
        ...props
      });
    });
  }

  /**
   * Load a plugin component based on its mount configuration
   */
  async loadPlugin(pluginId, componentTag = null) {
    let pluginEntry = null;

    if (componentTag) {
      const compositeKey = `${pluginId}_${componentTag}`;
      pluginEntry = this.pluginRegistry.get(compositeKey);
    } else {
      for (const [, entry] of this.pluginRegistry) {
        if (entry.plugin_id === parseInt(pluginId)) {
          pluginEntry = entry;
          break;
        }
      }
    }

    if (!pluginEntry) {
      throw new Error(`Portal plugin ${pluginId} not found in registry`);
    }

    const mount_config = pluginEntry.mount_config;
    if (!mount_config || !mount_config.kind) {
      throw new Error(`Portal plugin ${pluginId} has invalid mount configuration`);
    }

    switch (mount_config.kind) {
      case 'webc':
        return await this.loadWebComponent(pluginEntry);
      case 'iframe':
        return this.loadIframe(pluginEntry);
      default:
        throw new Error(`Unsupported portal plugin mount kind: ${mount_config.kind}`);
    }
  }

  /**
   * Load an iframe plugin
   */
  loadIframe(pluginEntry) {
    const { mount_config, plugin_id } = pluginEntry;
    const { app } = mount_config;

    const React = window.React || require('react');
    const { useEffect, useRef } = React;

    return React.forwardRef((props, ref) => {
      const iframeRef = useRef(null);

      useEffect(() => {
        const iframe = iframeRef.current;
        if (!iframe) return;

        const assetUrl = `/common/plugins/assets/${plugin_id}${app}`;
        iframe.src = assetUrl;

        if (ref) {
          if (typeof ref === 'function') {
            ref(iframe);
          } else {
            ref.current = iframe;
          }
        }
      }, [plugin_id, props, ref]);

      return React.createElement('iframe', {
        ref: iframeRef,
        style: { width: '100%', height: '100%', border: 'none', ...props.style },
        sandbox: 'allow-scripts allow-same-origin allow-forms',
        title: `Portal Plugin ${plugin_id}`,
        ...props
      });
    });
  }

  /**
   * Get all portal plugin routes for React Router
   */
  getPluginRoutes() {
    const routes = [];
    for (const [, entry] of this.pluginRegistry) {
      if (entry.route_pattern && entry.is_active) {
        routes.push({
          path: entry.route_pattern,
          pluginId: entry.plugin_id,
          component: entry.component_tag,
          componentTag: entry.component_tag,
          title: entry.mount_config?.title || 'Plugin',
          exact: true
        });
      }
    }
    return routes;
  }

  /**
   * Get sidebar menu items from portal plugins
   */
  async getSidebarMenuItems() {
    try {
      const response = await pubClient.get('/common/plugins/portal-sidebar-menu');
      return response.data.data || [];
    } catch (error) {
      console.error('Failed to get portal plugin sidebar items:', error);
      return [];
    }
  }

  /**
   * Refresh the portal plugin registry (hot reload support)
   */
  async refresh() {
    this.loadedComponents.clear();
    this.pluginRegistry.clear();
    const result = await this.initialize();
    window.dispatchEvent(new CustomEvent('portal-plugin-loader-refreshed'));
    return result;
  }
}

const portalPluginLoaderService = new PortalPluginLoaderService();
export default portalPluginLoaderService;
