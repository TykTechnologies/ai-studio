import React from "react";
import { Outlet } from "react-router-dom";
import { Box, CssBaseline, Toolbar } from "@mui/material";
import PortalAppBar from "../components/PortalAppBar";
import PortalDrawer from "../components/PortalDrawer";

const PortalLayout = () => {
  return (
    <Box sx={{ display: "flex" }}>
      <CssBaseline />
      <PortalAppBar />
      <PortalDrawer />
      <Box component="main" sx={{ flexGrow: 1, p: 3 }}>
        <Toolbar />
        <Outlet />
      </Box>
    </Box>
  );
};

export default PortalLayout;
