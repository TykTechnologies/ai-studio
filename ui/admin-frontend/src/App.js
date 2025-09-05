import React, { useState, useEffect, Suspense } from "react";
import { ThemeProvider } from "@mui/material/styles";
import {
  BrowserRouter as Router,
  Routes,
  Route,
  Navigate,
} from "react-router-dom";
import CssBaseline from "@mui/material/CssBaseline";
import CircularProgress from "@mui/material/CircularProgress";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import SuccessBanner from "./admin/components/common/SuccessBanner";

// Configurations and utilities
import { loadConfig } from "./config";
import { reinitializeApiClient } from "./admin/utils/apiClient";
import { reinitializePubClient } from "./admin/utils/pubClient";
import pubClient from "./admin/utils/pubClient";

// Themes
import adminTheme from "./admin/theme";

// Pages (add OAuthConsentPage)
import OAuthConsentPage from "./portal/pages/OAuthConsentPage";

// Components

// Layouts
import MainLayout from "./layouts/MainLayout";

// Context providers
import { NotificationProvider } from "./admin/context/NotificationContext";

// Lazy loaded routes for code splitting
const AdminRoutes = React.lazy(() => import("./routes/AdminRoutes"));
const PortalRoutes = React.lazy(() => import("./routes/PortalRoutes"));
const ChatRoutes = React.lazy(() => import("./routes/ChatRoutes"));
const Login = React.lazy(() => import("./portal/pages/Login"));
const Register = React.lazy(() => import("./portal/pages/Register"));
const ForgotPassword = React.lazy(() => import("./portal/pages/ForgotPassword"));
const ResetPassword = React.lazy(() => import("./portal/pages/ResetPassword"));
const NotificationsPage = React.lazy(() => import("./pages/NotificationsPage"));
const ToolDocumentationPage = React.lazy(() => import("./portal/pages/ToolDocumentationPage"));

// Component to redirect OAuth requests to backend
const BackendRedirect = () => {
  const backendUrl = process.env.REACT_APP_API_BASE_URL || 'http://localhost:8080';
  const currentPath = window.location.pathname;
  const queryString = window.location.search;
  
  // Redirect to backend URL
  window.location.href = `${backendUrl}${currentPath}${queryString}`;
  
  return <CircularProgress />;
};

// Loading component for lazy-loaded routes
const RouteLoadingFallback = () => (
  <Box
    sx={{
      display: "flex",
      justifyContent: "center",
      alignItems: "center",
      height: "50vh",
    }}
  >
    <CircularProgress />
  </Box>
);

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [loading, setLoading] = useState(true);
  const [configLoaded, setConfigLoaded] = useState(false);
  const [error, setError] = useState(null);
  const [entitlements, setEntitlements] = useState(null);
  const [showSuccessBanner, setShowSuccessBanner] = useState(true);

  const handleCloseBanner = () => {
    setShowSuccessBanner(false);
  };

  useEffect(() => {
    const initialize = async () => {
      try {
        await loadConfig();
        reinitializeApiClient();
        reinitializePubClient();
        setConfigLoaded(true);

        // Skip auth check for password reset and forgot password routes
        const currentPath = window.location.pathname;
        if (currentPath === '/reset-password' ||
          currentPath === '/auth/reset-password' ||
          currentPath === '/forgot-password' ||
          currentPath === '/auth/forgot-password') {
          setIsAuthenticated(false);
          return;
        }

        try {
          const response = await pubClient.get("/common/me");
          setIsAuthenticated(true);
          const attributes = response.data.attributes;
          setEntitlements({
            is_admin: attributes.is_admin,
            ui_options: attributes.ui_options,
            entitlements: attributes.entitlements,
          });
          console.log("Is admin:", attributes.is_admin);
        } catch (authError) {
          if (authError.response && authError.response.status === 401) {
            setIsAuthenticated(false);
          } else {
            console.error("Authentication check failed:", authError);
            setError("Failed to check authentication status.");
          }
        }
      } catch (error) {
        console.error("Configuration initialization failed:", error);
        setError("Failed to initialize application configuration.");
      } finally {
        setLoading(false);
      }
    };

    initialize();
  }, []);

  if (loading || !configLoaded) {
    return (
      <Box
        sx={{
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
          height: "100vh",
        }}
      >
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Box
        sx={{
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
          height: "100vh",
        }}
      >
        <div>{error}</div>
      </Box>
    );
  }

  return (
    <Router>
      <NotificationProvider>
        <ThemeProvider theme={adminTheme}>
          <CssBaseline />
          <Routes>
            {/* Public Routes */}
            <Route
              path="/login"
              element={
                isAuthenticated ? (
                  <Navigate
                    to={
                      entitlements?.is_admin ? "/admin/dash" : "/portal/dashboard"
                    }
                    replace
                  />
                ) : (
                  <Suspense fallback={<RouteLoadingFallback />}>
                    <Login />
                  </Suspense>
                )
              }
            />
            <Route
              path="/register"
              element={
                isAuthenticated ? (
                  <Navigate to="/portal/dashboard" replace />
                ) : (
                  <Suspense fallback={<RouteLoadingFallback />}>
                    <Register />
                  </Suspense>
                )
              }
            />
            <Route
              path="/forgot-password"
              element={
                isAuthenticated ? (
                  <Navigate to="/portal/dashboard" replace />
                ) : (
                  <Suspense fallback={<RouteLoadingFallback />}>
                    <ForgotPassword />
                  </Suspense>
                )
              }
            />
            {/* Handle both /reset-password and /auth/reset-password */}
            <Route
              path="/reset-password"
              element={
                isAuthenticated ? (
                  <Navigate to="/portal/dashboard" replace />
                ) : (
                  <Suspense fallback={<RouteLoadingFallback />}>
                    <ResetPassword />
                  </Suspense>
                )
              }
            />
            <Route
              path="/auth/reset-password"
              element={<Navigate to="/reset-password" replace state={{ preserveQuery: true }} />}
            />

            {/* OAuth Consent Page Route - public layout, backend handles auth check */}
            <Route path="/oauth/consent" element={<OAuthConsentPage />} />
            
            {/* OAuth Backend Routes - redirect to backend to handle these */}
            <Route path="/oauth/authorize" element={<BackendRedirect />} />
            <Route path="/oauth/token" element={<BackendRedirect />} />
            <Route path="/oauth/register_client" element={<BackendRedirect />} />
            <Route path="/.well-known/oauth-authorization-server" element={<BackendRedirect />} />


            {/* Protected Routes with MainLayout */}
            <Route
              element={
                isAuthenticated ? (
                  <MainLayout />
                ) : (
                  <Navigate to="/login" replace />
                )
              }
            >
              {/* Portal Routes */}
              <Route path="/portal/*" element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <PortalRoutes />
                </Suspense>
              } />

              {/* Chat Routes */}
              <Route path="/chat/*" element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <ChatRoutes />
                </Suspense>
              } />

              {/* Admin Routes */}
              <Route path="/admin/*" element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <AdminRoutes uiOptions={entitlements?.ui_options} />
                </Suspense>
              } />

              {/* Common Routes */}
              <Route path="/notifications" element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <NotificationsPage />
                </Suspense>
              } />
              <Route path="/common/tools/:id/docs" element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <ToolDocumentationPage />
                </Suspense>
              } />

              {/* Default redirect */}
              <Route
                path="/"
                element={
                  isAuthenticated ? (
                    entitlements?.is_admin === true ? (
                      <Navigate to="/admin/dash" replace />
                    ) : entitlements?.ui_options?.show_portal ? (
                      <Navigate to="/portal/dashboard" replace />
                    ) : entitlements?.ui_options?.show_chat ? (
                      <Navigate to="/chat/dashboard" replace />
                    ) : (
                      <Box sx={{ p: 7, display: "flex", flexDirection: "column", gap: 2 }}>
                        <Typography variant="headingXLarge">
                          Welcome to Tyk AI Studio!
                        </Typography>
                        {showSuccessBanner && (
                          <SuccessBanner
                            title="Tyk AI studio account"
                            message="You'll receive an email once your role is assigned and access is ready. If there's a delay, contact your organization admin"
                            onClose={handleCloseBanner}
                          />
                        )}
                      </Box>
                    )
                  ) : (
                    <Navigate to="/login" replace />
                  )
                }
              />
            </Route>

            {/* Catch all route */}
            <Route
              path="*"
              element={
                <Navigate
                  to={
                    isAuthenticated
                      ? "/admin"
                      : "/login"
                  }
                  replace
                />
              }
            />
          </Routes>
        </ThemeProvider>
      </NotificationProvider>
    </Router>
  );
}

export default App;
