import React from 'react';
import { BrowserRouter as Router } from 'react-router-dom'; // Import Router
import AppRoutes from './AppRoutes';
import AppDrawer from './components/AppDrawer'; // Import AppDrawer
import AppBreadcrumbs from './components/AppBreadcrumbs'; // Import AppBreadcrumbs
import Box from '@mui/material/Box'; // For layout
import Drawer from '@mui/material/Drawer'; // Assuming a Drawer layout
import CssBaseline from '@mui/material/CssBaseline'; // For baseline styling

const drawerWidth = 240; // Define drawer width

const App: React.FC = () => {
  return (
    <Router> {/* Wrap with Router */}
      <Box sx={{ display: 'flex' }}>
        <CssBaseline />
        <Drawer
        variant="permanent"
        sx={{
          width: drawerWidth,
          flexShrink: 0,
          [`& .MuiDrawer-paper`]: { width: drawerWidth, boxSizing: 'border-box' },
        }}
      >
        <AppDrawer />
      </Drawer>
      <Box
        component="main"
        sx={{ flexGrow: 1, p: 3, width: `calc(100% - ${drawerWidth}px)` }}
      >
        {/* Add Toolbar spacer if using AppBar, e.g., <Toolbar /> */}
        <AppBreadcrumbs />
        <Box sx={{ mt: 2 }}> {/* Add some margin top for spacing */}
          <AppRoutes />
        </Box>
      </Box>
    </Router>
  );
};

export default App;
