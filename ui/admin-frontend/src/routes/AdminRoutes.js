import React, { useState, useEffect } from "react";
import { Routes, Route, useLocation } from "react-router-dom";
import { mainAdminRoutes, ssoRoutes, groupRoutes, catalogRoutes } from "../admin/routes";
import { usePluginRoutes } from "../admin/components/plugins/DynamicPluginRoute";
import useSystemFeatures from "../admin/hooks/useSystemFeatures";

// Plugin route handler component
const PluginRouteHandler = () => {
  const location = useLocation();
  const [Component, setComponent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    const loadPluginComponent = async () => {
      try {
        console.log(`DEBUG PLUGIN ROUTE: Loading component for ${location.pathname}`);

        const { default: pluginLoaderService } = await import('../admin/services/pluginLoaderService');

        if (!pluginLoaderService.isInitialized) {
          await pluginLoaderService.initialize();
        }

        // Find route that matches current path
        const routes = pluginLoaderService.getPluginRoutes();
        console.log(`DEBUG PLUGIN ROUTE: Looking for route matching ${location.pathname}`);
        console.log(`DEBUG PLUGIN ROUTE: Available routes:`, routes.map(r => r.path));

        // Try exact match first
        let matchingRoute = routes.find(route => route.path === location.pathname);

        // If no exact match, try with /admin prefix added to current path
        if (!matchingRoute) {
          const pathWithAdmin = `/admin${location.pathname}`;
          matchingRoute = routes.find(route => route.path === pathWithAdmin);
          console.log(`DEBUG PLUGIN ROUTE: Tried with admin prefix: ${pathWithAdmin}`);
        }

        // If still no match, try removing /admin from route paths
        if (!matchingRoute) {
          matchingRoute = routes.find(route => {
            const routeWithoutAdmin = route.path.startsWith('/admin/')
              ? route.path.substring('/admin/'.length)
              : route.path;
            return routeWithoutAdmin === location.pathname.substring(1); // remove leading slash
          });
        }

        if (!matchingRoute) {
          throw new Error(`No plugin route found for ${location.pathname}. Available: ${routes.map(r => r.path).join(', ')}`);
        }

        console.log(`DEBUG PLUGIN ROUTE: Found matching route:`, matchingRoute);

        const LoadedComponent = await pluginLoaderService.loadPlugin(matchingRoute.pluginId, matchingRoute.componentTag);
        setComponent(() => LoadedComponent);
        setLoading(false);
      } catch (err) {
        console.error('Failed to load plugin component:', err);
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

const AdminRoutes = ({ uiOptions }) => {
  const { routes: pluginRoutes, isLoading: pluginRoutesLoading } = usePluginRoutes();
  const { features } = useSystemFeatures();

  console.log(`DEBUG ADMIN ROUTES: Rendering with ${pluginRoutes.length} plugin routes:`, pluginRoutes);

  // Don't render until plugin routes have been loaded to prevent flicker
  if (pluginRoutesLoading) {
    return null;
  }

  return (
    <Routes>
      {mainAdminRoutes}
      {uiOptions?.show_sso_config && ssoRoutes}
      {features.feature_groups && groupRoutes}
      {features.feature_groups && catalogRoutes}

      {/* Dynamically registered plugin routes */}
      {pluginRoutes.map((route) => {
        console.log(`DEBUG ADMIN ROUTES: Registering route ${route.path} for plugin ${route.pluginId}`);

        // Strip /admin prefix if present since we're already in admin context
        const routePath = route.path.startsWith('/admin/')
          ? route.path.substring('/admin/'.length)
          : route.path;

        return (
          <Route
            key={`plugin-route-${route.pluginId}-${routePath}`}
            path={routePath}
            element={<PluginRouteHandler />}
          />
        );
      })}

      {/* Debug route for testing */}
      <Route
        path="test-dynamic"
        element={<div style={{padding: '20px'}}>✅ Dynamic route works!</div>}
      />
    </Routes>
  );
};

export default AdminRoutes;
