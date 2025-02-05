import React from "react";
import { Routes, Route } from "react-router-dom";
import PortalDashboard from "../portal/pages/PortalDashboard";
import LLMListView from "../portal/components/LLMListView";
import DataSourceListView from "../portal/components/DataSourceListView";
import AppBuilder from "../portal/components/AppBuilder";
import AppListView from "../portal/components/AppListView";
import AppDetailView from "../portal/components/AppDetailView";
import { Navigate } from "react-router-dom";

const PortalRoutes = () => (
  <Routes>
    <Route path="/" element={<Navigate to="/portal/dashboard" />} />
    <Route path="/dashboard" element={<PortalDashboard />} />
    <Route path="/llms/:catalogueId" element={<LLMListView />} />
    <Route path="/databases/:catalogueId" element={<DataSourceListView />} />
    <Route path="/app/new" element={<AppBuilder />} />
    <Route path="/apps" element={<AppListView />} />
    <Route path="/apps/:id" element={<AppDetailView />} />
  </Routes>
);

export default PortalRoutes;
