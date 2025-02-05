// External dependencies
import React from "react";
import { Navigate } from "react-router-dom";

// Admin imports
import Dashboard from "../admin/pages/Dashboard";
import Users from "../admin/pages/Users";
import Groups from "../admin/pages/Groups";
import LLMList from "../admin/pages/LLMList";
import LLMSettingsList from "../admin/pages/LLMSettingsList";
import ModelPriceList from "../admin/pages/ModelPriceList";
import DatasourceList from "../admin/pages/DatasourceList";
import ToolList from "../admin/pages/ToolList";
import FilterList from "../admin/pages/FilterList";
import Secrets from "../admin/pages/Secrets";
import CatalogueList from "../admin/pages/CatalogueList";
import DataCatalogList from "../admin/pages/DataCatalogList";
import ToolCatalogueList from "../admin/pages/ToolCatalogueList";
import AppList from "../admin/pages/AppList";
import ChatList from "../admin/pages/ChatList";

// Admin component imports
import UserDetails from "../admin/components/users/UserDetails";
import UserForm from "../admin/components/users/UserForm";
import UserMessageLog from "../admin/components/users/UserMessageLog";
import GroupDetail from "../admin/components/groups/GroupDetail";
import GroupForm from "../admin/components/groups/GroupForm";
import LLMDetails from "../admin/components/llms/LLMDetails";
import LLMForm from "../admin/components/llms/LLMForm";
import LLMSettingsDetails from "../admin/components/llm-settings/LLMSettingsDetails";
import LLMSettingsForm from "../admin/components/llm-settings/LLMSettingsForm";
import ModelPriceDetail from "../admin/components/model-prices/ModelPriceDetail";
import ModelPriceForm from "../admin/components/model-prices/ModelPriceForm";
import DatasourceDetails from "../admin/components/datasources/DatasourceDetails";
import DatasourceForm from "../admin/components/datasources/DatasourceForm";
import ToolDetails from "../admin/components/tools/ToolDetails";
import ToolForm from "../admin/components/tools/ToolForm";
import FilterDetails from "../admin/components/filters/FilterDetails";
import FilterForm from "../admin/components/filters/FilterForm";
import SecretDetails from "../admin/components/secrets/SecretDetails";
import SecretForm from "../admin/components/secrets/SecretForm";
import CatalogueDetails from "../admin/components/catalogues/CatalogueDetails";
import CatalogueForm from "../admin/components/catalogues/CatalogueForm";
import DataCatalogDetail from "../admin/components/data-catalogs/DataCatalogDetail";
import DataCatalogForm from "../admin/components/data-catalogs/DataCatalogForm";
import ToolCatalogueDetails from "../admin/components/tool-catalogues/ToolCatalogueDetails";
import ToolCatalogueForm from "../admin/components/tool-catalogues/ToolCatalogueForm";
import AppDetails from "../admin/components/apps/AppDetails";
import AppForm from "../admin/components/apps/AppForm";
import ChatDetails from "../admin/components/chats/ChatDetails";
import ChatForm from "../admin/components/chats/ChatForm";

// Portal imports
import ChatView from "../portal/components/ChatView";
import PortalDashboard from "../portal/pages/PortalDashboard";
import LLMListView from "../portal/components/LLMListView";
import DataSourceListView from "../portal/components/DataSourceListView";
import AppBuilder from "../portal/components/AppBuilder";
import AppListView from "../portal/components/AppListView";
import AppDetailView from "../portal/components/AppDetailView";

// Auth pages
import Login from "../portal/pages/Login";
import Register from "../portal/pages/Register";
import ForgotPassword from "../portal/pages/ForgotPassword";
import ResetPassword from "../portal/pages/ResetPassword";

export const protectedRoutes = [
  // Chat routes
  {
    path: "/chat",
    element: <Navigate to="/chat/rooms" replace />,
  },
  {
    path: "/chat/:chatId",
    element: <ChatView />,
  },

  // Portal routes
  {
    path: "/portal",
    element: <Navigate to="/portal/dashboard" replace />,
  },
  {
    path: "/portal/dashboard",
    element: <PortalDashboard />,
  },
  {
    path: "/portal/llms/:catalogueId",
    element: <LLMListView />,
  },
  {
    path: "/portal/databases/:catalogueId",
    element: <DataSourceListView />,
  },
  {
    path: "/portal/app/new",
    element: <AppBuilder />,
  },
  {
    path: "/portal/apps",
    element: <AppListView />,
  },
  {
    path: "/portal/apps/:id",
    element: <AppDetailView />,
  },

  // Admin routes
  {
    path: "/admin",
    element: <Navigate to="/admin/dashboard" replace />,
  },
  {
    path: "/admin/dashboard",
    element: <Dashboard />,
  },
  {
    path: "/admin/users",
    element: <Users />,
  },
  {
    path: "/admin/users/:id",
    element: <UserDetails />,
  },
  {
    path: "/admin/users/:id/chat-log/:sessionId",
    element: <UserMessageLog />,
  },
  {
    path: "/admin/users/edit/:id",
    element: <UserForm />,
  },
  {
    path: "/admin/users/new",
    element: <UserForm />,
  },
  {
    path: "/admin/groups",
    element: <Groups />,
  },
  {
    path: "/admin/groups/:id",
    element: <GroupDetail />,
  },
  {
    path: "/admin/groups/edit/:id",
    element: <GroupForm />,
  },
  {
    path: "/admin/groups/new",
    element: <GroupForm />,
  },
  {
    path: "/admin/llms",
    element: <LLMList />,
  },
  {
    path: "/admin/llms/:id",
    element: <LLMDetails />,
  },
  {
    path: "/admin/llms/edit/:id",
    element: <LLMForm />,
  },
  {
    path: "/admin/llms/new",
    element: <LLMForm />,
  },
  {
    path: "/admin/llm-settings",
    element: <LLMSettingsList />,
  },
  {
    path: "/admin/llm-settings/:id",
    element: <LLMSettingsDetails />,
  },
  {
    path: "/admin/llm-settings/edit/:id",
    element: <LLMSettingsForm />,
  },
  {
    path: "/admin/llm-settings/new",
    element: <LLMSettingsForm />,
  },
  {
    path: "/admin/model-prices",
    element: <ModelPriceList />,
  },
  {
    path: "/admin/model-prices/:id",
    element: <ModelPriceDetail />,
  },
  {
    path: "/admin/model-prices/edit/:id",
    element: <ModelPriceForm />,
  },
  {
    path: "/admin/model-prices/new",
    element: <ModelPriceForm />,
  },
  {
    path: "/admin/datasources",
    element: <DatasourceList />,
  },
  {
    path: "/admin/datasources/:id",
    element: <DatasourceDetails />,
  },
  {
    path: "/admin/datasources/edit/:id",
    element: <DatasourceForm />,
  },
  {
    path: "/admin/datasources/new",
    element: <DatasourceForm />,
  },
  {
    path: "/admin/tools",
    element: <ToolList />,
  },
  {
    path: "/admin/tools/:id",
    element: <ToolDetails />,
  },
  {
    path: "/admin/tools/edit/:id",
    element: <ToolForm />,
  },
  {
    path: "/admin/tools/new",
    element: <ToolForm />,
  },
  {
    path: "/admin/filters",
    element: <FilterList />,
  },
  {
    path: "/admin/filters/:id",
    element: <FilterDetails />,
  },
  {
    path: "/admin/filters/edit/:id",
    element: <FilterForm />,
  },
  {
    path: "/admin/filters/new",
    element: <FilterForm />,
  },
  {
    path: "/admin/secrets",
    element: <Secrets />,
  },
  {
    path: "/admin/secrets/:id",
    element: <SecretDetails />,
  },
  {
    path: "/admin/secrets/edit/:id",
    element: <SecretForm />,
  },
  {
    path: "/admin/secrets/new",
    element: <SecretForm />,
  },
  {
    path: "/admin/catalogs/llms",
    element: <CatalogueList />,
  },
  {
    path: "/admin/catalogs/llms/:id",
    element: <CatalogueDetails />,
  },
  {
    path: "/admin/catalogs/llms/edit/:id",
    element: <CatalogueForm />,
  },
  {
    path: "/admin/catalogs/llms/new",
    element: <CatalogueForm />,
  },
  {
    path: "/admin/catalogs/data",
    element: <DataCatalogList />,
  },
  {
    path: "/admin/catalogs/data/:id",
    element: <DataCatalogDetail />,
  },
  {
    path: "/admin/catalogs/data/edit/:id",
    element: <DataCatalogForm />,
  },
  {
    path: "/admin/catalogs/data/new",
    element: <DataCatalogForm />,
  },
  {
    path: "/admin/catalogs/tools",
    element: <ToolCatalogueList />,
  },
  {
    path: "/admin/catalogs/tools/:id",
    element: <ToolCatalogueDetails />,
  },
  {
    path: "/admin/catalogs/tools/edit/:id",
    element: <ToolCatalogueForm />,
  },
  {
    path: "/admin/catalogs/tools/new",
    element: <ToolCatalogueForm />,
  },
  {
    path: "/admin/apps",
    element: <AppList />,
  },
  {
    path: "/admin/apps/:id",
    element: <AppDetails />,
  },
  {
    path: "/admin/apps/edit/:id",
    element: <AppForm />,
  },
  {
    path: "/admin/apps/new",
    element: <AppForm />,
  },
  {
    path: "/admin/chats",
    element: <ChatList />,
  },
  {
    path: "/admin/chats/:id",
    element: <ChatDetails />,
  },
  {
    path: "/admin/chats/edit/:id",
    element: <ChatForm />,
  },
  {
    path: "/admin/chats/new",
    element: <ChatForm />,
  },
];

export const publicRoutes = [
  {
    path: "/login",
    element: <Login />,
  },
  {
    path: "/register",
    element: <Register />,
  },
  {
    path: "/forgot-password",
    element: <ForgotPassword />,
  },
  {
    path: "/reset-password",
    element: <ResetPassword />,
  },
];
