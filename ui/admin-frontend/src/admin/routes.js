import React from "react";
import Users from "./pages/Users";
import UserDetails from "./components/users/UserDetails";
import UserForm from "./components/users/UserForm";

import Groups from "./pages/groups/Groups";
import GroupDetail from "./components/groups/GroupDetail";
import GroupForm from "./components/groups/GroupForm";

import LLMList from "./pages/LLMList";
import LLMDetails from "./components/llms/LLMDetails";
import LLMModelDetails from "./components/llms/LLMModelDetails";
import LLMForm from "./components/llms/LLMForm";

import LLMSettingsList from "./pages/LLMSettingsList";
import LLMSettingsDetails from "./components/llm-settings/LLMSettingsDetails";
import LLMSettingsForm from "./components/llm-settings/LLMSettingsForm";

import ModelPriceList from "./pages/ModelPriceList";
import ModelPriceDetail from "./components/model-prices/ModelPriceDetail";
import ModelPriceForm from "./components/model-prices/ModelPriceForm";

import DatasourceList from "./pages/DatasourceList";
import DatasourceDetails from "./components/datasources/DatasourceDetails";
import DatasourceForm from "./components/datasources/DatasourceForm";

import ToolList from "./pages/ToolList";
import ToolDetails from "./components/tools/ToolDetails";
import ToolForm from "./components/tools/ToolForm";

import MetadataSchemas from "./pages/MetadataSchemas";
import MetadataSchemaForm from "./pages/MetadataSchemaForm";
import MetadataVocabularies from "./pages/MetadataVocabularies";
import MetadataCompliance from "./pages/MetadataCompliance";

import CatalogueList from "./pages/CatalogueList";
import CatalogueDetails from "./components/catalogues/CatalogueDetails";
import CatalogueForm from "./components/catalogues/CatalogueForm";

import DataCatalogList from "./pages/DataCatalogList";
import DataCatalogDetail from "./components/data-catalogs/DataCatalogDetail";
import DataCatalogForm from "./components/data-catalogs/DataCatalogForm";

import ToolCatalogueList from "./pages/ToolCatalogueList";
import ToolCatalogueDetails from "./components/tool-catalogues/ToolCatalogueDetails";
import ToolCatalogueForm from "./components/tool-catalogues/ToolCatalogueForm";

import FilterList from "./pages/FilterList";
import FilterDetails from "./components/filters/FilterDetails";
import FilterForm from "./components/filters/FilterForm";

import AppList from "./pages/AppList";
import AppDetails from "./components/apps/AppDetails";
import AppForm from "./components/apps/AppForm";

import ChatList from "./pages/ChatList";
import ChatDetails from "./components/chats/ChatDetails";
import ChatForm from "./components/chats/ChatForm";
import UserMessageLog from "./components/users/UserMessageLog";

import Secrets from "./pages/Secrets";
import SecretDetails from "./components/secrets/SecretDetails";
import SecretForm from "./components/secrets/SecretForm";

import Dashboard from "./pages/Dashboard";
import Overview from "./pages/Overview";

import SSOProfiles from "./pages/SSOProfiles";
import SSOProfileEditor from "./components/sso-profiles/SSOProfileEditor";
import SSOProfileDetails from "./components/sso-profiles/SSOProfileDetails";

import EdgeGatewaysPage from "./pages/EdgeGatewaysPage";
import PluginsPage from "./pages/PluginsPage";

import AgentList from "./pages/AgentList";
import AgentDetail from "./components/agents/AgentDetail";
import AgentForm from "./components/agents/AgentForm";

import Marketplace from "./components/marketplace/Marketplace";
import MarketplaceSettings from "./pages/MarketplaceSettings";
import BrandingSettings from "./pages/BrandingSettings";

import ModelRouterList from "./pages/ModelRouterList";
import ModelRouterDetails from "./components/model-routers/ModelRouterDetails";
import ModelRouterForm from "./components/model-routers/ModelRouterForm";

import ComplianceOverview from "./pages/ComplianceOverview";
import AuditTrail from "./pages/AuditTrail";
import Webhooks from "./pages/Webhooks";
import WebhookDeliveries from "./pages/WebhookDeliveries";

import SubmissionReviewQueue from "./pages/SubmissionReviewQueue";
import SubmissionReview from "./pages/SubmissionReview";
import AttestationTemplates from "./pages/AttestationTemplates";

import Roles from "./pages/roles/Roles";
import RoleDetail from "./components/roles/RoleDetail";
import RoleForm from "./components/roles/RoleForm";

import { P } from "./rbac/permissions";

/**
 * Admin route descriptors: { path | index, element, permission? }.
 * `permission` is the catalogue permission the page needs; list/detail pages
 * need read, new/edit forms need write. AdminRoutes.js wraps each element in
 * RequirePermission, which renders a denial panel instead of a blank page.
 */
const mainAdminRoutes = [
  { index: true, element: <Overview /> },
  { path: "dash", element: <Dashboard />, permission: P.ANALYTICS_READ },
  { path: "dashboard", element: <Dashboard />, permission: P.ANALYTICS_READ },
  { path: "branding", element: <BrandingSettings />, permission: P.BRANDING_WRITE },
  { path: "users", element: <Users />, permission: P.USERS_READ },
  { path: "users/:id", element: <UserDetails />, permission: P.USERS_READ },
  { path: "users/:id/chat-log/:sessionId", element: <UserMessageLog />, permission: P.CHAT_HISTORY_READ },
  { path: "users/edit/:id", element: <UserForm />, permission: P.USERS_WRITE },
  { path: "users/new", element: <UserForm />, permission: P.USERS_WRITE },

  { path: "llms", element: <LLMList />, permission: P.LLMS_READ },
  { path: "llms/:id", element: <LLMDetails />, permission: P.LLMS_READ },
  { path: "llms/:id/models", element: <LLMModelDetails />, permission: P.LLMS_READ },
  { path: "llms/edit/:id", element: <LLMForm />, permission: P.LLMS_WRITE },
  { path: "llms/new", element: <LLMForm />, permission: P.LLMS_WRITE },

  { path: "llm-settings", element: <LLMSettingsList />, permission: P.LLM_SETTINGS_READ },
  { path: "llm-settings/:id", element: <LLMSettingsDetails />, permission: P.LLM_SETTINGS_READ },
  { path: "llm-settings/edit/:id", element: <LLMSettingsForm />, permission: P.LLM_SETTINGS_WRITE },
  { path: "llm-settings/new", element: <LLMSettingsForm />, permission: P.LLM_SETTINGS_WRITE },

  { path: "plugins/*", element: <PluginsPage />, permission: P.PLUGINS_READ },

  { path: "marketplace", element: <Marketplace />, permission: P.MARKETPLACE_READ },
  { path: "marketplace-settings", element: <MarketplaceSettings />, permission: P.MARKETPLACE_WRITE },

  { path: "model-prices", element: <ModelPriceList />, permission: P.MODEL_PRICES_READ },
  { path: "model-prices/:id", element: <ModelPriceDetail />, permission: P.MODEL_PRICES_READ },
  { path: "model-prices/edit/:id", element: <ModelPriceForm />, permission: P.MODEL_PRICES_WRITE },
  { path: "model-prices/new", element: <ModelPriceForm />, permission: P.MODEL_PRICES_WRITE },

  { path: "datasources", element: <DatasourceList />, permission: P.DATASOURCES_READ },
  { path: "datasources/:id", element: <DatasourceDetails />, permission: P.DATASOURCES_READ },
  { path: "datasources/edit/:id", element: <DatasourceForm />, permission: P.DATASOURCES_WRITE },
  { path: "datasources/new", element: <DatasourceForm />, permission: P.DATASOURCES_WRITE },

  { path: "tools", element: <ToolList />, permission: P.TOOLS_READ },
  { path: "tools/:id", element: <ToolDetails />, permission: P.TOOLS_READ },
  { path: "tools/edit/:id", element: <ToolForm />, permission: P.TOOLS_WRITE },
  { path: "tools/new", element: <ToolForm />, permission: P.TOOLS_WRITE },

  { path: "apps", element: <AppList />, permission: P.APPS_READ },
  { path: "apps/:id", element: <AppDetails />, permission: P.APPS_READ },
  { path: "apps/edit/:id", element: <AppForm />, permission: P.APPS_WRITE },
  { path: "apps/new", element: <AppForm />, permission: P.APPS_WRITE },

  { path: "edge-gateways/*", element: <EdgeGatewaysPage />, permission: P.EDGES_READ },

  { path: "agents", element: <AgentList />, permission: P.AGENTS_READ },
  { path: "agents/:id", element: <AgentDetail />, permission: P.AGENTS_READ },
  { path: "agents/edit/:id", element: <AgentForm />, permission: P.AGENTS_WRITE },
  { path: "agents/new", element: <AgentForm />, permission: P.AGENTS_WRITE },

  { path: "chats", element: <ChatList />, permission: P.CHATS_READ },
  { path: "chats/:id", element: <ChatDetails />, permission: P.CHATS_READ },
  { path: "chats/edit/:id", element: <ChatForm />, permission: P.CHATS_WRITE },
  { path: "chats/new", element: <ChatForm />, permission: P.CHATS_WRITE },

  { path: "secrets", element: <Secrets />, permission: P.SECRETS_READ },
  { path: "secrets/:id", element: <SecretDetails />, permission: P.SECRETS_READ },
  { path: "secrets/edit/:id", element: <SecretForm />, permission: P.SECRETS_WRITE },
  { path: "secrets/new", element: <SecretForm />, permission: P.SECRETS_WRITE },

  { path: "filters", element: <FilterList />, permission: P.FILTERS_READ },
  { path: "filters/:id", element: <FilterDetails />, permission: P.FILTERS_READ },
  { path: "filters/edit/:id", element: <FilterForm />, permission: P.FILTERS_WRITE },
  { path: "filters/new", element: <FilterForm />, permission: P.FILTERS_WRITE },

  { path: "compliance", element: <ComplianceOverview />, permission: P.COMPLIANCE_READ },
  { path: "audit", element: <AuditTrail />, permission: P.AUDIT_READ },
  // Webhooks (Enterprise only; pages self-gate via /webhooks/status)
  { path: "webhooks", element: <Webhooks />, permission: P.WEBHOOKS_READ },
  { path: "webhooks/deliveries", element: <WebhookDeliveries />, permission: P.WEBHOOKS_READ },

  { path: "submissions", element: <SubmissionReviewQueue />, permission: P.SUBMISSIONS_READ },
  { path: "submissions/:id", element: <SubmissionReview />, permission: P.SUBMISSIONS_READ },
  { path: "attestation-templates", element: <AttestationTemplates />, permission: P.ATTESTATION_TEMPLATES_READ },

  // Governed metadata (Enterprise only; pages self-gate via /metadata/available)
  { path: "metadata/schemas", element: <MetadataSchemas />, permission: P.METADATA_READ },
  { path: "metadata/schemas/new", element: <MetadataSchemaForm />, permission: P.METADATA_WRITE },
  { path: "metadata/schemas/edit/:id", element: <MetadataSchemaForm />, permission: P.METADATA_WRITE },
  { path: "metadata/vocabularies", element: <MetadataVocabularies />, permission: P.METADATA_READ },
  { path: "metadata/compliance", element: <MetadataCompliance />, permission: P.METADATA_READ },
];

// SSO profile routes that will be conditionally rendered based on uiOptions.show_sso_config
const ssoRoutes = [
  { path: "sso-profiles", element: <SSOProfiles />, permission: P.SSO_PROFILES_READ },
  { path: "sso-profiles/new", element: <SSOProfileEditor />, permission: P.SSO_PROFILES_WRITE },
  { path: "sso-profiles/edit/:profileId", element: <SSOProfileEditor />, permission: P.SSO_PROFILES_WRITE },
  { path: "sso-profiles/:profileId", element: <SSOProfileDetails />, permission: P.SSO_PROFILES_READ },
];

// Group routes that will be conditionally rendered based on features.feature_groups (ENT only)
const groupRoutes = [
  { path: "groups", element: <Groups />, permission: P.GROUPS_READ },
  { path: "groups/:id", element: <GroupDetail />, permission: P.GROUPS_READ },
  { path: "groups/edit/:id", element: <GroupForm />, permission: P.GROUPS_WRITE },
  { path: "groups/new", element: <GroupForm />, permission: P.GROUPS_WRITE },
];

// Catalog routes that will be conditionally rendered based on features.feature_groups (ENT only)
const catalogRoutes = [
  { path: "catalogs/llms", element: <CatalogueList />, permission: P.CATALOGUES_READ },
  { path: "catalogs/llms/:id", element: <CatalogueDetails />, permission: P.CATALOGUES_READ },
  { path: "catalogs/llms/edit/:id", element: <CatalogueForm />, permission: P.CATALOGUES_WRITE },
  { path: "catalogs/llms/new", element: <CatalogueForm />, permission: P.CATALOGUES_WRITE },
  { path: "catalogs/data", element: <DataCatalogList />, permission: P.DATA_CATALOGUES_READ },
  { path: "catalogs/data/:id", element: <DataCatalogDetail />, permission: P.DATA_CATALOGUES_READ },
  { path: "catalogs/data/edit/:id", element: <DataCatalogForm />, permission: P.DATA_CATALOGUES_WRITE },
  { path: "catalogs/data/new", element: <DataCatalogForm />, permission: P.DATA_CATALOGUES_WRITE },
  { path: "catalogs/tools", element: <ToolCatalogueList />, permission: P.TOOL_CATALOGUES_READ },
  { path: "catalogs/tools/:id", element: <ToolCatalogueDetails />, permission: P.TOOL_CATALOGUES_READ },
  { path: "catalogs/tools/edit/:id", element: <ToolCatalogueForm />, permission: P.TOOL_CATALOGUES_WRITE },
  { path: "catalogs/tools/new", element: <ToolCatalogueForm />, permission: P.TOOL_CATALOGUES_WRITE },
];

// Model Router routes (Enterprise only - requires feature_model_router)
const modelRouterRoutes = [
  { path: "model-routers", element: <ModelRouterList />, permission: P.MODEL_ROUTERS_READ },
  { path: "model-routers/:id", element: <ModelRouterDetails />, permission: P.MODEL_ROUTERS_READ },
  { path: "model-routers/edit/:id", element: <ModelRouterForm />, permission: P.MODEL_ROUTERS_WRITE },
  { path: "model-routers/new", element: <ModelRouterForm />, permission: P.MODEL_ROUTERS_WRITE },
];

// Role routes (Enterprise only; the pages show an upgrade prompt in CE)
const roleRoutes = [
  { path: "roles", element: <Roles />, permission: P.ROLES_READ },
  { path: "roles/:id", element: <RoleDetail />, permission: P.ROLES_READ },
  { path: "roles/edit/:id", element: <RoleForm />, permission: P.ROLES_WRITE },
  { path: "roles/new", element: <RoleForm />, permission: P.ROLES_WRITE },
];

export { mainAdminRoutes, ssoRoutes, groupRoutes, catalogRoutes, modelRouterRoutes, roleRoutes };
export default mainAdminRoutes;
