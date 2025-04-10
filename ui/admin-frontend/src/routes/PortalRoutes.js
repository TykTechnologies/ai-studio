import React from "react";
import { Routes, Route } from "react-router-dom";
import PortalDashboard from "../portal/pages/PortalDashboard";
import LLMListView from "../portal/components/LLMListView";
import DataSourceListView from "../portal/components/DataSourceListView";
import AppBuilder from "../portal/components/AppBuilder";
import AppListView from "../portal/components/AppListView";
import AppDetailView from "../portal/components/AppDetailView";
import MCPServerListView from "../portal/components/MCPServerListView";
import MCPServerDetailView from "../portal/components/MCPServerDetailView";
import MCPServerCreation from "../portal/components/MCPServerCreation";
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
    <Route path="/mcp-servers" element={<MCPServerListView />} />
    <Route path="/mcp-servers/new" element={<MCPServerCreation />} />
    <Route path="/mcp-servers/:id" element={<MCPServerDetailView />} />
  </Routes>
);

export default PortalRoutes;
