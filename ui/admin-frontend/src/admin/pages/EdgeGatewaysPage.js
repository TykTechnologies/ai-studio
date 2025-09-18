import React from 'react';
import { Routes, Route } from 'react-router-dom';
import EdgeGatewayList from '../components/edge-gateways/EdgeGatewayList';
import EdgeGatewayDetail from '../components/edge-gateways/EdgeGatewayDetail';

const EdgeGatewaysPage = () => {
  return (
    <Routes>
      <Route path="/" element={<EdgeGatewayList />} />
      <Route path="/:id" element={<EdgeGatewayDetail />} />
    </Routes>
  );
};

export default EdgeGatewaysPage;