import React from "react";
import { Route } from "react-router-dom";
import PortalLayout from "./layouts/PortalLayout";
import Login from "./pages/Login";
import Register from "./pages/Register";
import ForgotPassword from "./pages/ForgotPassword";
import EmailVerification from "./pages/EmailVerification";

const portalRoutes = (
  <Route element={<PortalLayout />}>
    <Route index element={<Login />} />
    <Route path="login" element={<Login />} />
    <Route path="register" element={<Register />} />
    <Route path="forgot-password" element={<ForgotPassword />} />
    <Route path="verify-email" element={<EmailVerification />} />
  </Route>
);

export default portalRoutes;
