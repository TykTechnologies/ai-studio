import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import PortalDashboard from "../portal/pages/PortalDashboard";
import LLMListView from "../portal/components/LLMListView";
import DataSourceListView from "../portal/components/DataSourceListView";
import ToolListView from "../portal/components/ToolListView";
import AppBuilder from "../portal/components/AppBuilder";
import AppListView from "../portal/components/AppListView";
import AppDetailView from "../portal/components/AppDetailView";
import ToolDocumentationPage from "../portal/pages/ToolDocumentationPage";
import MyContributions from "../portal/pages/MyContributions";
import SubmissionForm from "../portal/pages/SubmissionForm";
import SubmissionDetail from "../portal/pages/SubmissionDetail";
import { usePortalPluginRoutes, PortalPluginRouteHandler } from "../portal/components/plugins/DynamicPortalPluginRoute";

const PortalRoutes = () => {
  const { routes: pluginRoutes, isLoading: pluginRoutesLoading } = usePortalPluginRoutes();

  // Don't render until plugin routes have been loaded to prevent flicker
  if (pluginRoutesLoading) {
    return null;
  }

  return (
    <Routes>
      <Route path="/" element={<Navigate to="/portal/dashboard" />} />
      <Route path="/dashboard" element={<PortalDashboard />} />
      <Route path="/llms/:catalogueId" element={<LLMListView />} />
      <Route path="/databases/:catalogueId" element={<DataSourceListView />} />
      <Route path="/tools/:catalogueId" element={<ToolListView />} />
      <Route path="/tools/:id/docs" element={<ToolDocumentationPage />} />
      <Route path="/app/new" element={<AppBuilder />} />
      <Route path="/apps" element={<AppListView />} />
      <Route path="/apps/:id" element={<AppDetailView />} />
      <Route path="/contributions" element={<MyContributions />} />
      <Route path="/submissions/new" element={<SubmissionForm />} />
      <Route path="/submissions/edit/:id" element={<SubmissionForm />} />
      <Route path="/submissions/:id" element={<SubmissionDetail />} />

      {/* Dynamically registered portal plugin routes */}
      {pluginRoutes.map((route) => {
        const routePath = route.path.startsWith('/portal/')
          ? route.path.substring('/portal/'.length)
          : route.path;

        return (
          <Route
            key={`portal-plugin-${route.pluginId}-${routePath}`}
            path={routePath}
            element={<PortalPluginRouteHandler />}
          />
        );
      })}
    </Routes>
  );
};

export default PortalRoutes;
