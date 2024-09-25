import React from "react";
import { ThemeProvider } from "@mui/material/styles";
import theme from "./admin/theme";
import { BrowserRouter as Router, Routes, Route } from "react-router-dom";
import CssBaseline from "@mui/material/CssBaseline";
import Box from "@mui/material/Box";
import Toolbar from "@mui/material/Toolbar";

import MyAppBar from "./admin/components/layout/AppBar";
import MyDrawer from "./admin/components/layout/Drawer";

import portalRoutes from "./portal/routes";
import adminRoutes from "./admin/routes";

function App() {
  return (
    <ThemeProvider theme={theme}>
      <Router>
        <Box sx={{ display: "flex" }}>
          <CssBaseline />
          <MyAppBar />
          <MyDrawer />
          <Box component="main" sx={{ flexGrow: 1, p: 3 }}>
            <Toolbar />
            <Routes>
              {adminRoutes}
              {portalRoutes}
            </Routes>
          </Box>
        </Box>
      </Router>
    </ThemeProvider>
  );
}

export default App;
