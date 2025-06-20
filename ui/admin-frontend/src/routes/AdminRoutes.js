import React from "react";
import { Routes } from "react-router-dom";
import { mainAdminRoutes, ssoRoutes } from "../admin/routes";

const AdminRoutes = ({ uiOptions }) => (
  <Routes>
    {mainAdminRoutes}
    {uiOptions?.show_sso_config && ssoRoutes}
  </Routes>
);

export default AdminRoutes;
