import React, { useState, useEffect, Suspense } from 'react';
import { Routes, Route } from 'react-router-dom';
import { CircularProgress, Box, Alert } from '@mui/material';
import pluginLoaderService from '../../services/pluginLoaderService';

/**
 * PluginRouter - Handles dynamic routing for plugin-contributed UI components
 * This component integrates with React Router to provide plugin routes
 */
const PluginRouter = () => {
  const [pluginRoutes, setPluginRoutes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    initializePluginRoutes();
  }, []);

  const initializePluginRoutes = async () => {
    try {
      setLoading(true);
      setError(null);

      // Initialize plugin loader if not already initialized
      if (!pluginLoaderService.isInitialized) {
        await pluginLoaderService.initialize();
      }

      // Get plugin routes
      const routes = pluginLoaderService.getPluginRoutes();
      setPluginRoutes(routes);

      console.log(`Initialized ${routes.length} plugin routes`);
    } catch (err) {
      console.error('Failed to initialize plugin routes:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="200px">
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Alert severity="error" sx={{ m: 2 }}>
        Failed to load plugin routes: {error}
      </Alert>
    );
  }

  return (
    <Routes>
      {pluginRoutes.map((route) => (
        <Route
          key={route.path}
          path={route.path}
          element={
            <Suspense
              fallback={
                <Box display="flex" justifyContent="center" alignItems="center" minHeight="200px">
                  <CircularProgress />
                </Box>
              }
            >
              <PluginRouteComponent route={route} />
            </Suspense>
          }
        />
      ))}
    </Routes>
  );
};

/**
 * PluginRouteComponent - Wrapper component that loads and renders plugin components
 */
const PluginRouteComponent = ({ route }) => {
  const [Component, setComponent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    loadPluginComponent();
  }, [route.pluginId]);

  const loadPluginComponent = async () => {
    try {
      setLoading(true);
      setError(null);

      // Check if component is already loaded
      if (pluginLoaderService.isLoaded(route.component)) {
        const loadedComponent = pluginLoaderService.getComponent(route.component);
        setComponent(() => loadedComponent);
      } else {
        // Load the plugin component
        const loadedComponent = await pluginLoaderService.loadPlugin(route.pluginId);
        setComponent(() => loadedComponent);
      }

      console.log(`Loaded plugin component for route: ${route.path}`);
    } catch (err) {
      console.error(`Failed to load plugin component for route ${route.path}:`, err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
        <Box ml={2}>Loading {route.title}...</Box>
      </Box>
    );
  }

  if (error) {
    return (
      <Alert severity="error" sx={{ m: 2 }}>
        Failed to load plugin component: {error}
        <br />
        <small>Route: {route.path}</small>
      </Alert>
    );
  }

  if (!Component) {
    return (
      <Alert severity="warning" sx={{ m: 2 }}>
        Plugin component not found for route: {route.path}
      </Alert>
    );
  }

  // Render the plugin component
  return (
    <Box>
      <Component />
    </Box>
  );
};

export default PluginRouter;