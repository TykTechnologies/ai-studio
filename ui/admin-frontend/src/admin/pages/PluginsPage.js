import React from 'react';
import { Routes, Route } from 'react-router-dom';
import PluginList from '../components/plugins/PluginList';
import PluginForm from '../components/plugins/PluginForm';
import PluginDetail from '../components/plugins/PluginDetail';
import PluginRouter from '../components/plugins/PluginRouter';
import PluginCreationWizard from '../components/plugins/PluginCreationWizard';

const PluginsPage = () => {
  return (
    <Routes>
      {/* Standard plugin management routes */}
      <Route path="/" element={<PluginList />} />
      <Route path="/create" element={<PluginCreationWizard />} />
      <Route path="/:id" element={<PluginDetail />} />
      <Route path="/:id/edit" element={<PluginForm mode="edit" />} />

      {/* Plugin-contributed UI routes - catch-all for any plugin routes */}
      <Route path="/ui/*" element={<PluginRouter />} />
    </Routes>
  );
};

export default PluginsPage;