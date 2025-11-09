import React, { useState, useEffect, useMemo } from 'react';
import { Route } from 'react-router-dom';
import pluginLoaderService from '../../services/pluginLoaderService';

/**
 * getDynamicPluginRoutes - Returns React Router routes dynamically from plugin manifests
 * This function should be called to get routes that can be added to the main admin routes
 */
const getDynamicPluginRoutes = async () => {
  try {
    // Initialize plugin loader if not already initialized
    if (!pluginLoaderService.isInitialized) {
      await pluginLoaderService.initialize();
    }

    // Get plugin routes
    const routes = pluginLoaderService.getPluginRoutes();
    console.log(`Loaded ${routes.length} dynamic plugin routes`);

    return routes.map((route) => {
      // Create lazy-loaded component for each plugin route
      const LazyPluginComponent = React.lazy(async () => {
        const Component = await pluginLoaderService.loadPlugin(route.pluginId);
        return { default: Component };
      });

      return (
        <Route
          key={`plugin-route-${route.pluginId}-${route.path}`}
          path={route.path}
          element={
            <PluginErrorBoundary route={route}>
              <React.Suspense
                fallback={
                  <div style={{ padding: '20px', textAlign: 'center' }}>
                    Loading {route.title}...
                  </div>
                }
              >
                <LazyPluginComponent />
              </React.Suspense>
            </PluginErrorBoundary>
          }
        />
      );
    });
  } catch (error) {
    console.error('Failed to get dynamic plugin routes:', error);
    return [];
  }
};

/**
 * DynamicPluginRoute - Component that renders plugin routes
 * This should be used within a Routes component
 */
const DynamicPluginRoute = () => {
  const [routeElements, setRouteElements] = useState([]);
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    const loadRoutes = async () => {
      try {
        const routes = await getDynamicPluginRoutes();
        setRouteElements(routes);
        setInitialized(true);
      } catch (error) {
        console.error('Failed to load plugin routes:', error);
        setInitialized(true);
      }
    };

    loadRoutes();

    // Listen for plugin loader refresh events
    const handlePluginRefresh = () => {
      console.log('DynamicPluginRoute received plugin refresh event, reloading routes');
      loadRoutes();
    };

    window.addEventListener('plugin-loader-refreshed', handlePluginRefresh);

    return () => {
      window.removeEventListener('plugin-loader-refreshed', handlePluginRefresh);
    };
  }, []);

  if (!initialized) {
    return null;
  }

  // Return individual Route components that can be used in Routes
  return routeElements;
};

// Hook to get plugin routes for use in other components
export const usePluginRoutes = () => {
  const [pluginRoutes, setPluginRoutes] = useState([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchRoutes = async () => {
      try {
        if (!pluginLoaderService.isInitialized) {
          await pluginLoaderService.initialize();
        }
        const routes = pluginLoaderService.getPluginRoutes();
        setPluginRoutes(routes);
      } catch (error) {
        console.error('Failed to fetch plugin routes:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchRoutes();

    // Listen for plugin loader refresh events
    const handlePluginRefresh = () => {
      console.log('usePluginRoutes received plugin refresh event, reloading routes');
      fetchRoutes();
    };

    window.addEventListener('plugin-loader-refreshed', handlePluginRefresh);

    return () => {
      window.removeEventListener('plugin-loader-refreshed', handlePluginRefresh);
    };
  }, []);

  return { routes: pluginRoutes, isLoading };
};

// Error boundary for plugin components
class PluginErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error };
  }

  componentDidCatch(error, errorInfo) {
    console.error('Plugin component error:', error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div style={{ padding: '20px', border: '1px solid #f44336', borderRadius: '4px', margin: '16px' }}>
          <h3 style={{ color: '#f44336' }}>Plugin Load Error</h3>
          <p>Failed to load plugin component for route: {this.props.route?.path}</p>
          <p><strong>Plugin ID:</strong> {this.props.route?.pluginId}</p>
          <p><strong>Error:</strong> {this.state.error?.message}</p>
          <button
            onClick={() => this.setState({ hasError: false, error: null })}
            style={{
              background: '#1976d2',
              color: 'white',
              border: 'none',
              padding: '8px 16px',
              borderRadius: '4px',
              cursor: 'pointer'
            }}
          >
            Try Again
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}

export default DynamicPluginRoute;