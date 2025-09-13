import React from 'react';
import { Routes, Route } from 'react-router-dom';
import PluginList from '../components/plugins/PluginList';
import PluginForm from '../components/plugins/PluginForm';
import PluginDetail from '../components/plugins/PluginDetail';

const PluginsPage = () => {
  return (
    <Routes>
      <Route path="/" element={<PluginList />} />
      <Route path="/create" element={<PluginForm mode="create" />} />
      <Route path="/:id" element={<PluginDetail />} />
      <Route path="/:id/edit" element={<PluginForm mode="edit" />} />
    </Routes>
  );
};

export default PluginsPage;