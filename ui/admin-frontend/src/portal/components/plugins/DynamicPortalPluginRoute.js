import React, { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import portalPluginLoaderService from '../../services/portalPluginLoaderService';

/**
 * PortalPluginRouteHandler - Component that loads and renders a portal plugin
 * component based on the current route path.
 */
export const PortalPluginRouteHandler = () => {
  const location = useLocation();
  const [Component, setComponent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    const loadPluginComponent = async () => {
      setLoading(true);
      setError(null);

      try {
        if (!portalPluginLoaderService.isInitialized) {
          await portalPluginLoaderService.initialize();
        }

        const routes = portalPluginLoaderService.getPluginRoutes();

        // Try exact match first
        let matchingRoute = routes.find(route => route.path === location.pathname);

        // Try with /portal prefix
        if (!matchingRoute) {
          const pathWithPortal = `/portal${location.pathname}`;
          matchingRoute = routes.find(route => route.path === pathWithPortal);
        }

        // Try removing /portal from route paths
        if (!matchingRoute) {
          matchingRoute = routes.find(route => {
            const routeWithoutPortal = route.path.startsWith('/portal/')
              ? route.path.substring('/portal/'.length)
              : route.path;
            return routeWithoutPortal === location.pathname.substring(1);
          });
        }

        if (!matchingRoute) {
          throw new Error(`No portal plugin route found for ${location.pathname}. Available: ${routes.map(r => r.path).join(', ')}`);
        }

        const LoadedComponent = await portalPluginLoaderService.loadPlugin(
          matchingRoute.pluginId, matchingRoute.componentTag
        );
        setComponent(() => LoadedComponent);
        setLoading(false);
      } catch (err) {
        console.error('Failed to load portal plugin component:', err);
        setError(err.message);
        setLoading(false);
      }
    };

    loadPluginComponent();
  }, [location.pathname]);

  if (loading) {
    return <div style={{ padding: '20px' }}>Loading plugin...</div>;
  }

  if (error) {
    return (
      <div style={{ padding: '20px' }}>
        <h3>Plugin Load Error</h3>
        <p>{error}</p>
      </div>
    );
  }

  if (!Component) {
    return <div style={{ padding: '20px' }}>Plugin component not found</div>;
  }

  return <Component />;
};

/**
 * usePortalPluginRoutes - Hook to get portal plugin routes for React Router integration
 */
export const usePortalPluginRoutes = () => {
  const [routes, setRoutes] = useState([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchRoutes = async () => {
      try {
        if (!portalPluginLoaderService.isInitialized) {
          await portalPluginLoaderService.initialize();
        }
        const pluginRoutes = portalPluginLoaderService.getPluginRoutes();
        setRoutes(pluginRoutes);
      } catch (error) {
        console.error('Failed to fetch portal plugin routes:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchRoutes();

    const handlePluginRefresh = () => {
      fetchRoutes();
    };

    window.addEventListener('portal-plugin-loader-refreshed', handlePluginRefresh);
    return () => {
      window.removeEventListener('portal-plugin-loader-refreshed', handlePluginRefresh);
    };
  }, []);

  return { routes, isLoading };
};

export default PortalPluginRouteHandler;
