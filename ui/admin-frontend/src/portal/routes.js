import React from "react";
import { Route } from "react-router-dom";
import AuthLayout from "./layouts/AuthLayout";
import Login from "./pages/Login";
import Register from "./pages/Register";
import ForgotPassword from "./pages/ForgotPassword";
import ResetPassword from "./pages/ResetPassword";
import PortalDashboard from "./pages/PortalDashboard";
import LLMListView from "./components/LLMListView";
import DataSourceListView from "./components/DataSourceListView";

const portalRoutes = [
  <Route key="auth" element={<AuthLayout />}>
    <Route index element={<Login />} />
    <Route path="login" element={<Login />} />
    <Route path="register" element={<Register />} />
    <Route path="forgot-password" element={<ForgotPassword />} />
    <Route path="reset-password" element={<ResetPassword />} />
  </Route>,
  <Route key="portal">
    <Route path="dashboard" element={<PortalDashboard />} />
    <Route path="llms/:catalogueId" element={<LLMListView />} />
    <Route path="databases/:catalogueId" element={<DataSourceListView />} />
    {/* Add other portal routes that require the drawer here */}
  </Route>,
];

export default portalRoutes;
