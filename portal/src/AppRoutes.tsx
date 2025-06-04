import React from 'react';
import { Route, Routes } from 'react-router-dom'; // Removed BrowserRouter
import ToolsCatalogPage from './pages/tools/ToolsCatalogPage';

const AppRoutes: React.FC = () => {
  return (
    // Router was removed from here
    <Routes>
      {/* Other routes can be added here */}
      <Route path="/tools" element={<ToolsCatalogPage />} />
    </Routes>
  );
};

export default AppRoutes;
