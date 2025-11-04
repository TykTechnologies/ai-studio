import apiClient from '../utils/apiClient';

/**
 * Plugin Loader Service - Handles dynamic loading of plugin UI components
 * Implements the architecture described in Hot-load-ui-plugins-plan.md
 */
class PluginLoaderService {
  constructor() {
    this.loadedComponents = new Map();
    this.pluginRegistry = new Map();
    this.isInitialized = false;
  }

  /**
   * Initialize the plugin loader by fetching the UI registry
   */
  async initialize() {
    try {
      const response = await apiClient.get('/plugins/ui-registry');
      const registry = response.data.data || [];

      // Build plugin registry
      registry.forEach(entry => {
        if (entry.is_active) {
          // Use composite key to allow multiple components per plugin
          const compositeKey = `${entry.plugin_id}_${entry.component_tag}`;
          this.pluginRegistry.set(compositeKey, entry);
          console.log(`Registry entry ${entry.ID}: plugin_id=${entry.plugin_id}, route=${entry.route_pattern}, tag=${entry.component_tag}, key=${compositeKey}`);
        }
      });

      this.isInitialized = true;
      console.log(`Plugin loader initialized with ${this.pluginRegistry.size} components`);

      return registry;
    } catch (error) {
      console.error('Failed to initialize plugin loader:', error);
      throw error;
    }
  }

  /**
   * Load a Web Component plugin dynamically
   * @param {Object} pluginEntry - Plugin registry entry
   */
  async loadWebComponent(pluginEntry) {
    // Handle both camelCase and snake_case field names from API
    const component_tag = pluginEntry.component_tag;
    const entry_point = pluginEntry.entry_point;
    const plugin_id = pluginEntry.plugin_id;
    const mount_config = pluginEntry.mount_config;

    console.log(`DEBUG loadWebComponent: entry=`, pluginEntry);
    console.log(`DEBUG loadWebComponent: plugin_id=${plugin_id}, component_tag=${component_tag}, entry_point=${entry_point}`);

    if (this.loadedComponents.has(component_tag)) {
      return this.loadedComponents.get(component_tag);
    }

    try {
      // Fetch the JavaScript content using apiClient (includes auth headers)
      const assetPath = `/plugins/assets/${plugin_id}${entry_point}`;
      console.log(`Loading Web Component from: ${assetPath}`);

      const response = await apiClient.get(assetPath, {
        headers: {
          'Accept': 'application/javascript'
        }
      });

      if (!response.data) {
        throw new Error(`No content received from ${assetPath}`);
      }

      // Check if custom element is already registered (avoid redeclaration)
      if (customElements.get(component_tag)) {
        console.log(`Web Component ${component_tag} already registered, skipping execution`);
      } else {
        // Execute the JavaScript to register the Web Component
        console.log(`Executing Web Component JavaScript (${response.data.length} chars)`);

        // Create a script element and execute the code
        const script = document.createElement('script');
        script.type = 'text/javascript';
        script.text = response.data;
        document.head.appendChild(script);

        // Give the script a moment to execute
        await new Promise(resolve => setTimeout(resolve, 100));

        // Verify the custom element was registered
        if (!customElements.get(component_tag)) {
          throw new Error(`Web Component ${component_tag} was not registered after executing JavaScript`);
        }
      }

      // Create a React wrapper for the Web Component with plugin context
      const WebComponentWrapper = this.createWebComponentWrapper(component_tag, mount_config, plugin_id);

      this.loadedComponents.set(component_tag, WebComponentWrapper);

      // Mark plugin as loaded
      await this.markPluginLoaded(plugin_id);

      console.log(`Successfully loaded Web Component: ${component_tag}`);
      return WebComponentWrapper;

    } catch (error) {
      console.error(`Failed to load Web Component ${component_tag}:`, error);
      throw error;
    }
  }

  /**
   * Create a React wrapper for a Web Component
   * @param {string} tagName - Custom element tag name
   * @param {Object} mountConfig - Mount configuration from manifest
   * @param {number} pluginId - Plugin ID for RPC calls
   */
  createWebComponentWrapper(tagName, mountConfig = {}, pluginId = null) {
    const React = window.React || require('react'); // Support both import methods
    const { useEffect, useRef } = React;

    return React.forwardRef((props, ref) => {
      const elementRef = useRef(null);

      useEffect(() => {
        const element = elementRef.current;
        if (!element) return;

        // Inject context-aware plugin API (hides all implementation details)
        if (pluginId) {
          element.pluginAPI = {
            call: async (method, payload = {}) => {
              try {
                const { pluginRPCCall } = await import('../utils/apiClient');
                const result = await pluginRPCCall(pluginId, method, payload);
                return result.data;
              } catch (error) {
                console.error(`Plugin RPC call failed: ${method}`, error);
                throw error;
              }
            }
          };
          console.log(`Injected plugin API for plugin ${pluginId} into ${element.tagName}`);
        }

        // Set props from mount config
        const configProps = mountConfig.props || {};
        Object.entries(configProps).forEach(([key, value]) => {
          // Convert camelCase to kebab-case for HTML attributes
          const attrName = key.replace(/([A-Z])/g, '-$1').toLowerCase();
          element.setAttribute(`data-${attrName}`, typeof value === 'string' ? value : JSON.stringify(value));
        });

        // Set component props
        Object.entries(props).forEach(([key, value]) => {
          if (key !== 'children') {
            element.setAttribute(`data-${key}`, typeof value === 'string' ? value : JSON.stringify(value));
          }
        });

        // Forward ref
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
   * Load a Module Federation plugin
   * @param {Object} pluginEntry - Plugin registry entry
   */
  async loadModuleFederation(pluginEntry) {
    const { mount_config, plugin_id } = pluginEntry;
    const { remote, exposed } = mount_config;

    if (this.loadedComponents.has(exposed)) {
      return this.loadedComponents.get(exposed);
    }

    try {
      console.log(`Loading Module Federation component: ${exposed} from ${remote}`);

      // Load the remote container
      const container = await this.loadRemoteContainer(remote, `plugin_${plugin_id}`);

      // Get the exposed component
      const factory = await container.get(exposed);
      const Component = factory();

      this.loadedComponents.set(exposed, Component);

      // Mark plugin as loaded
      await this.markPluginLoaded(plugin_id);

      console.log(`Successfully loaded Module Federation component: ${exposed}`);
      return Component;

    } catch (error) {
      console.error(`Failed to load Module Federation component ${exposed}:`, error);
      throw error;
    }
  }

  /**
   * Load a Module Federation remote container
   * @param {string} remoteUrl - URL to the remote entry
   * @param {string} scope - Scope name for the remote
   */
  async loadRemoteContainer(remoteUrl, scope) {
    return new Promise((resolve, reject) => {
      const script = document.createElement('script');
      script.type = 'text/javascript';
      script.async = true;
      script.src = remoteUrl;

      script.onload = async () => {
        try {
          // Initialize webpack sharing (check if webpack globals exist)
          if (typeof window.__webpack_init_sharing__ === 'function') {
            await window.__webpack_init_sharing__('default');
          }

          const container = window[scope];
          if (!container) {
            throw new Error(`Remote container ${scope} not found`);
          }

          // Initialize the container
          if (typeof window.__webpack_share_scopes__ !== 'undefined') {
            await container.init(window.__webpack_share_scopes__.default);
          } else {
            await container.init({});
          }
          resolve(container);
        } catch (error) {
          reject(error);
        }
      };

      script.onerror = () => {
        reject(new Error(`Failed to load remote script: ${remoteUrl}`));
      };

      document.head.appendChild(script);
    });
  }

  /**
   * Load an iframe plugin
   * @param {Object} pluginEntry - Plugin registry entry
   */
  loadIframe(pluginEntry) {
    const { mount_config, plugin_id } = pluginEntry;
    const { app } = mount_config;

    const React = window.React || require('react'); // Support both import methods
    const { useEffect, useRef } = React;

    return React.forwardRef((props, ref) => {
      const iframeRef = useRef(null);

      useEffect(() => {
        const iframe = iframeRef.current;
        if (!iframe) return;

        // Set up iframe source
        const assetUrl = `/plugins/assets/${plugin_id}${app}`;
        iframe.src = assetUrl;

        // Set up message handling for postMessage communication
        const handleMessage = (event) => {
          // Validate origin for security
          if (event.origin !== window.location.origin) return;

          // Handle plugin messages
          if (event.data.source === 'plugin' && event.data.pluginId === plugin_id) {
            console.log('Plugin message:', event.data);
            // Forward messages to parent component if needed
            if (props.onMessage) {
              props.onMessage(event.data);
            }
          }
        };

        window.addEventListener('message', handleMessage);

        // Forward ref
        if (ref) {
          if (typeof ref === 'function') {
            ref(iframe);
          } else {
            ref.current = iframe;
          }
        }

        return () => {
          window.removeEventListener('message', handleMessage);
        };
      }, [plugin_id, props, ref]);

      return React.createElement('iframe', {
        ref: iframeRef,
        style: {
          width: '100%',
          height: '100%',
          border: 'none',
          ...props.style
        },
        sandbox: 'allow-scripts allow-same-origin allow-forms',
        title: `Plugin ${plugin_id}`,
        ...props
      });
    });
  }

  /**
   * Load a plugin component based on its mount configuration
   * @param {string} pluginId - Plugin ID
   * @param {string} componentTag - Component tag (optional, for specific component lookup)
   */
  async loadPlugin(pluginId, componentTag = null) {
    console.log(`DEBUG LOAD PLUGIN: Loading plugin ID ${pluginId}, component: ${componentTag}`);

    let pluginEntry = null;

    if (componentTag) {
      // Direct lookup using composite key
      const compositeKey = `${pluginId}_${componentTag}`;
      pluginEntry = this.pluginRegistry.get(compositeKey);
      console.log(`DEBUG LOAD PLUGIN: Direct lookup with key ${compositeKey}: ${pluginEntry ? 'found' : 'not found'}`);
    } else {
      // Find any component for this plugin ID (backward compatibility)
      for (const [compositeKey, entry] of this.pluginRegistry) {
        if (entry.plugin_id === parseInt(pluginId)) {
          pluginEntry = entry;
          console.log(`DEBUG LOAD PLUGIN: Found plugin via composite key ${compositeKey}`);
          break;
        }
      }
    }

    if (!pluginEntry) {
      console.error(`DEBUG LOAD PLUGIN: Plugin ${pluginId} not found in registry. Available entries:`, Array.from(this.pluginRegistry.keys()));
      console.error(`DEBUG LOAD PLUGIN: Registry contents:`, Array.from(this.pluginRegistry.values()).map(e => ({id: e.ID, plugin_id: e.plugin_id, route: e.route_pattern})));
      throw new Error(`Plugin ${pluginId} not found in registry`);
    }

    console.log(`DEBUG LOAD PLUGIN: Found plugin entry:`, pluginEntry);

    const mount_config = pluginEntry.mount_config;
    if (!mount_config || !mount_config.kind) {
      console.error(`DEBUG LOAD PLUGIN: Invalid mount_config:`, mount_config);
      throw new Error(`Plugin ${pluginId} has invalid mount configuration`);
    }

    const { kind } = mount_config;
    console.log(`DEBUG LOAD PLUGIN: Mount kind: ${kind}`);

    switch (kind) {
      case 'webc':
        console.log(`DEBUG LOAD PLUGIN: Loading Web Component`);
        return await this.loadWebComponent(pluginEntry);

      case 'module-federation':
        console.log(`DEBUG LOAD PLUGIN: Loading Module Federation`);
        return await this.loadModuleFederation(pluginEntry);

      case 'iframe':
        console.log(`DEBUG LOAD PLUGIN: Loading iFrame`);
        return this.loadIframe(pluginEntry);

      default:
        console.error(`DEBUG LOAD PLUGIN: Unsupported mount kind: ${kind}`);
        throw new Error(`Unsupported plugin mount kind: ${kind}`);
    }
  }

  /**
   * Get all plugin routes for React Router
   */
  getPluginRoutes() {
    const routes = [];

    console.log(`DEBUG: getPluginRoutes called, registry size: ${this.pluginRegistry.size}`);

    for (const [id, entry] of this.pluginRegistry) {
      console.log(`DEBUG: Checking entry ID ${id}:`, {
        route_pattern: entry.route_pattern,
        is_active: entry.is_active,
        plugin_id: entry.plugin_id,
        component_tag: entry.component_tag
      });

      if (entry.route_pattern && entry.is_active) {
        const route = {
          path: entry.route_pattern,
          pluginId: entry.plugin_id, // Use the actual plugin ID, not registry entry ID
          component: entry.component_tag,
          componentTag: entry.component_tag, // Add componentTag for specific lookup
          title: entry.mount_config?.title || 'Plugin',
          exact: true
        };

        console.log(`DEBUG: Adding route:`, route);
        routes.push(route);
      } else {
        console.log(`DEBUG: Skipping entry - route_pattern: ${entry.route_pattern}, is_active: ${entry.is_active}`);
      }
    }

    console.log(`DEBUG: Returning ${routes.length} routes:`, routes);
    return routes;
  }

  /**
   * Get sidebar menu items from plugins
   */
  async getSidebarMenuItems() {
    try {
      const response = await apiClient.get('/plugins/sidebar-menu');
      return response.data.data || [];
    } catch (error) {
      console.error('Failed to get sidebar menu items:', error);
      return [];
    }
  }

  /**
   * Mark a plugin as loaded
   * @param {number} pluginId - Plugin ID
   */
  async markPluginLoaded(pluginId) {
    try {
      await apiClient.post(`/api/v1/plugins/${pluginId}/ui/load`);
    } catch (error) {
      console.warn(`Failed to mark plugin ${pluginId} as loaded:`, error);
    }
  }

  /**
   * Refresh the plugin registry (hot reload support)
   */
  async refresh() {
    this.loadedComponents.clear();
    this.pluginRegistry.clear();
    const result = await this.initialize();

    // Dispatch event to notify UI components to refresh
    window.dispatchEvent(new CustomEvent('plugin-loader-refreshed'));
    console.log('Plugin loader refreshed and notification dispatched');

    return result;
  }

  /**
   * Check if a plugin component is loaded
   * @param {string} componentTag - Component tag or identifier
   */
  isLoaded(componentTag) {
    return this.loadedComponents.has(componentTag);
  }

  /**
   * Get loaded component
   * @param {string} componentTag - Component tag or identifier
   */
  getComponent(componentTag) {
    return this.loadedComponents.get(componentTag);
  }
}

// Create singleton instance
const pluginLoaderService = new PluginLoaderService();

export default pluginLoaderService;