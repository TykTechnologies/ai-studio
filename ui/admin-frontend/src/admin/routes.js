import React from "react";
import { Route } from "react-router-dom";
import Users from "./pages/Users";
import UserDetails from "./components/users/UserDetails";
import UserForm from "./components/users/UserForm";

import Groups from "./pages/Groups";
import GroupDetail from "./components/groups/GroupDetail";
import GroupForm from "./components/groups/GroupForm";

import LLMList from "./pages/LLMList";
import LLMDetails from "./components/llms/LLMDetails";
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

import Dashboard from "./pages/Dashboard";

const adminRoutes = (
  <>
    <Route path="dash" element={<Dashboard />} />
    <Route path="dashboard" element={<Dashboard />} />
    <Route path="users" element={<Users />} />
    <Route path="users/:id" element={<UserDetails />} />
    <Route path="users/edit/:id" element={<UserForm />} />
    <Route path="users/new" element={<UserForm />} />

    <Route path="groups" element={<Groups />} />
    <Route path="groups/:id" element={<GroupDetail />} />
    <Route path="groups/edit/:id" element={<GroupForm />} />
    <Route path="groups/new" element={<GroupForm />} />

    <Route path="llms" element={<LLMList />} />
    <Route path="llms/:id" element={<LLMDetails />} />
    <Route path="llms/edit/:id" element={<LLMForm />} />
    <Route path="llms/new" element={<LLMForm />} />

    <Route path="llm-settings" element={<LLMSettingsList />} />
    <Route path="llm-settings/:id" element={<LLMSettingsDetails />} />
    <Route path="llm-settings/edit/:id" element={<LLMSettingsForm />} />
    <Route path="llm-settings/new" element={<LLMSettingsForm />} />

    <Route path="model-prices" element={<ModelPriceList />} />
    <Route path="model-prices/:id" element={<ModelPriceDetail />} />
    <Route path="model-prices/edit/:id" element={<ModelPriceForm />} />
    <Route path="model-prices/new" element={<ModelPriceForm />} />

    <Route path="datasources" element={<DatasourceList />} />
    <Route path="datasources/:id" element={<DatasourceDetails />} />
    <Route path="datasources/edit/:id" element={<DatasourceForm />} />
    <Route path="datasources/new" element={<DatasourceForm />} />

    <Route path="tools" element={<ToolList />} />
    <Route path="tools/:id" element={<ToolDetails />} />
    <Route path="tools/edit/:id" element={<ToolForm />} />
    <Route path="tools/new" element={<ToolForm />} />

    <Route path="catalogs/llms" element={<CatalogueList />} />
    <Route path="catalogs/llms/:id" element={<CatalogueDetails />} />
    <Route path="catalogs/llms/edit/:id" element={<CatalogueForm />} />
    <Route path="catalogs/llms/new" element={<CatalogueForm />} />
    <Route path="catalogs/data" element={<DataCatalogList />} />
    <Route path="catalogs/data/:id" element={<DataCatalogDetail />} />
    <Route path="catalogs/data/edit/:id" element={<DataCatalogForm />} />
    <Route path="catalogs/data/new" element={<DataCatalogForm />} />
    <Route path="catalogs/tools" element={<ToolCatalogueList />} />
    <Route path="catalogs/tools/:id" element={<ToolCatalogueDetails />} />
    <Route path="catalogs/tools/edit/:id" element={<ToolCatalogueForm />} />
    <Route path="catalogs/tools/new" element={<ToolCatalogueForm />} />

    <Route path="filters" element={<FilterList />} />
    <Route path="filters/:id" element={<FilterDetails />} />
    <Route path="filters/edit/:id" element={<FilterForm />} />
    <Route path="filters/new" element={<FilterForm />} />

    <Route path="apps" element={<AppList />} />
    <Route path="apps/:id" element={<AppDetails />} />
    <Route path="apps/edit/:id" element={<AppForm />} />
    <Route path="apps/new" element={<AppForm />} />

    <Route path="chats" element={<ChatList />} />
    <Route path="chats/:id" element={<ChatDetails />} />
    <Route path="chats/edit/:id" element={<ChatForm />} />
    <Route path="chats/new" element={<ChatForm />} />
  </>
);

export default adminRoutes;
