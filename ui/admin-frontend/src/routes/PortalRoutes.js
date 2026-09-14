import React from "react";
import { Routes, Route, Navigate, useParams } from "react-router-dom";
import PortalDashboard from "../portal/pages/PortalDashboard";
import CatalogBrowse from "../portal/pages/CatalogBrowse";
import AssetDetail from "../portal/pages/AssetDetail";
import AppBuilder from "../portal/components/AppBuilder";
import AppListView from "../portal/components/AppListView";
import AppDetailView from "../portal/components/AppDetailView";
import ToolDocumentationPage from "../portal/pages/ToolDocumentationPage";
import MyContributions from "../portal/pages/MyContributions";
import SubmissionForm from "../portal/pages/SubmissionForm";
import SubmissionDetail from "../portal/pages/SubmissionDetail";
import { usePortalPluginRoutes, PortalPluginRouteHandler } from "../portal/components/plugins/DynamicPortalPluginRoute";
import { CATALOG_TYPES, browsePath } from "../portal/utils/catalog";

// The per-catalog pages (/portal/llms/:catalogueId and friends) became the
// unified catalog filtered by that catalog, so old bookmarks keep working.
const LegacyCatalogRedirect = ({ type }) => {
  const { catalogueId } = useParams();
  return <Navigate replace to={`${browsePath(type)}?catalog=${type}:${catalogueId}`} />;
};

const LegacyResourceRedirect = () => {
  const { pluginId, slug } = useParams();
  return <Navigate replace to={`/portal/catalog/resources/${pluginId}/${slug}`} />;
};

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

      {/* Browse: one catalog, optionally fixed to a type, and its detail pages */}
      <Route path="/catalog" element={<CatalogBrowse />} />
      <Route path="/catalog/llms" element={<CatalogBrowse type={CATALOG_TYPES.LLM} />} />
      <Route path="/catalog/llms/:id" element={<AssetDetail type={CATALOG_TYPES.LLM} />} />
      <Route path="/catalog/datasources" element={<CatalogBrowse type={CATALOG_TYPES.DATASOURCE} />} />
      <Route path="/catalog/datasources/:id" element={<AssetDetail type={CATALOG_TYPES.DATASOURCE} />} />
      <Route path="/catalog/tools" element={<CatalogBrowse type={CATALOG_TYPES.TOOL} />} />
      <Route path="/catalog/tools/:id" element={<AssetDetail type={CATALOG_TYPES.TOOL} />} />
      <Route path="/catalog/resources/:pluginId/:slug" element={<CatalogBrowse type={CATALOG_TYPES.PLUGIN_RESOURCE} />} />
      <Route path="/catalog/resources/:pluginId/:slug/:instanceId" element={<AssetDetail type={CATALOG_TYPES.PLUGIN_RESOURCE} />} />

      {/* Legacy per-catalog routes */}
      <Route path="/llms/:catalogueId" element={<LegacyCatalogRedirect type={CATALOG_TYPES.LLM} />} />
      <Route path="/databases/:catalogueId" element={<LegacyCatalogRedirect type={CATALOG_TYPES.DATASOURCE} />} />
      <Route path="/tools/:catalogueId" element={<LegacyCatalogRedirect type={CATALOG_TYPES.TOOL} />} />
      <Route path="/resources/:pluginId/:slug" element={<LegacyResourceRedirect />} />

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
