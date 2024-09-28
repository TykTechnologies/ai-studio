import React, { useState, useEffect } from "react";
import { ThemeProvider } from "@mui/material/styles";

import portalTheme from "./portal/theme/portalTheme";

import theme from "./admin/theme";
import {
  BrowserRouter as Router,
  Routes,
  Route,
  Navigate,
} from "react-router-dom";
import CssBaseline from "@mui/material/CssBaseline";
import pubClient from "./admin/utils/pubClient";
import CircularProgress from "@mui/material/CircularProgress";
import Box from "@mui/material/Box";

import MainLayout from "./admin/components/layout/MainLayout";
import Login from "./portal/pages/Login";
import Register from "./portal/pages/Register";
import ForgotPassword from "./portal/pages/ForgotPassword";
import adminRoutes from "./admin/routes";
import portalRoutes from "./portal/routes";
import ResetPassword from "./portal/pages/ResetPassword";
import PortalLayout from "./portal/layouts/PortalLayout";

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isAdmin, setIsAdmin] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const checkAuth = async () => {
      const response = await pubClient.get("/common/me", {
        validateStatus: () => true,
      });

      if (response.status === 401) {
        setIsAuthenticated(false);
        setIsAdmin(false);
      } else if (response.status === 200) {
        setIsAuthenticated(true);
        setIsAdmin(response.data?.attributes.is_admin || false);
      } else {
        console.error("Unexpected status code:", response.status);
        setIsAuthenticated(false);
        setIsAdmin(false);
      }

      setLoading(false);
    };

    checkAuth();
  }, []);

  if (loading) {
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

  return (
    <ThemeProvider theme={isAdmin ? theme : portalTheme}>
      <Router>
        <CssBaseline />
        <Routes>
          <Route
            path="/"
            element={
              isAuthenticated ? (
                <Navigate
                  to={isAdmin ? "/admin/dashboard" : "/portal/dashboard"}
                  replace
                />
              ) : (
                <Login />
              )
            }
          />
          <Route
            path="/login"
            element={
              isAuthenticated ? (
                <Navigate
                  to={isAdmin ? "/admin/dashboard" : "/portal/dashboard"}
                  replace
                />
              ) : (
                <Login />
              )
            }
          />
          {/* Allow access to register and forgot-password without authentication */}
          <Route path="/register" element={<Register />} />
          <Route path="/forgot-password" element={<ForgotPassword />} />
          <Route path="/reset-password" element={<ResetPassword />} />
          <Route
            path="/admin/*"
            element={
              isAuthenticated && isAdmin ? (
                <MainLayout />
              ) : (
                <Navigate to="/login" replace />
              )
            }
          >
            {adminRoutes}
          </Route>
          <Route
            path="/portal/*"
            element={
              isAuthenticated ? (
                <PortalLayout /> // Change this to PortalLayout
              ) : (
                <Navigate to="/login" replace />
              )
            }
          >
            {portalRoutes}
          </Route>
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </Router>
    </ThemeProvider>
  );
}

export default App;
