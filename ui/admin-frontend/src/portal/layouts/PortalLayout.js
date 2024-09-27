import React from "react";
import { Box, Toolbar } from "@mui/material";
import { Outlet } from "react-router-dom";
import PortalAppBar from "../components/PortalAppBar";
import PortalDrawer from "../components/PortalDrawer";

const PortalLayout = () => {
  return (
    <Box sx={{ display: "flex" }}>
      <PortalAppBar />
      <PortalDrawer />
      <Box component="main" sx={{ flexGrow: 1, p: 3 }}>
        <Toolbar /> {/* This creates space below the AppBar */}
        <Outlet />
      </Box>
    </Box>
  );
};

export default PortalLayout;
