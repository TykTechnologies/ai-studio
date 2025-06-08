import React, { useState, useEffect } from "react";
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

// Routes
import AdminRoutes from "./routes/AdminRoutes";
import PortalRoutes from "./routes/PortalRoutes";
import ChatRoutes from "./routes/ChatRoutes";
import Login from "./portal/pages/Login";
import Register from "./portal/pages/Register";
import ForgotPassword from "./portal/pages/ForgotPassword";
import ResetPassword from "./portal/pages/ResetPassword";
import NotificationsPage from "./pages/NotificationsPage";
import ToolDocumentationPage from "./portal/pages/ToolDocumentationPage"; // Import the new page
import { NotificationProvider } from "./admin/context/NotificationContext";

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
                  <Login />
                )
              }
            />
            <Route
              path="/register"
              element={
                isAuthenticated ? (
                  <Navigate to="/portal/dashboard" replace />
                ) : (
                  <Register />
                )
              }
            />
            <Route
              path="/forgot-password"
              element={
                isAuthenticated ? (
                  <Navigate to="/portal/dashboard" replace />
                ) : (
                  <ForgotPassword />
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
                  <ResetPassword />
                )
              }
            />
            <Route
              path="/auth/reset-password"
              element={<Navigate to="/reset-password" replace state={{ preserveQuery: true }} />}
            />

            {/* OAuth Consent Page Route - public layout, backend handles auth check */}
            <Route path="/oauth/consent" element={<OAuthConsentPage />} />


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
              <Route path="/portal/*" element={<PortalRoutes />} />

              {/* Chat Routes */}
              <Route path="/chat/*" element={<ChatRoutes />} />

              {/* Admin Routes */}
              <Route path="/admin/*" element={<AdminRoutes uiOptions={entitlements?.ui_options} />} />

              {/* Common Routes */}
              <Route path="/notifications" element={<NotificationsPage />} />
              <Route path="/common/tools/:id/docs" element={<ToolDocumentationPage />} /> {/* Add new route here */}

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
