import React from "react";
import { Route } from "react-router-dom";
import AuthLayout from "./layouts/AuthLayout";
import PortalLayout from "./layouts/PortalLayout";
import Login from "./pages/Login";
import Register from "./pages/Register";
import ForgotPassword from "./pages/ForgotPassword";
import ResetPassword from "./pages/ResetPassword";
import PortalDashboard from "./pages/PortalDashboard";

const portalRoutes = [
  <Route key="auth" element={<AuthLayout />}>
    <Route index element={<Login />} />
    <Route path="login" element={<Login />} />
    <Route path="register" element={<Register />} />
    <Route path="forgot-password" element={<ForgotPassword />} />
    <Route path="reset-password" element={<ResetPassword />} />
  </Route>,
  <Route key="portal" element={<PortalLayout />}>
    <Route path="dashboard" element={<PortalDashboard />} />
    {/* Add other portal routes that require the drawer here */}
  </Route>,
];

export default portalRoutes;
