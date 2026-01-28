import React from "react";
import Box from "@mui/material/Box";
import Toolbar from "@mui/material/Toolbar";
import MyAppBar from "./AppBar";
import Drawer from "./Drawer";
import { Outlet } from "react-router-dom";
import SyncStatusBanner from "../common/SyncStatusBanner";

const MainLayout = ({ hideAppBar }) => {
  return (
    <Box sx={{ display: "flex" }}>
      {!hideAppBar && <MyAppBar />}
      <Drawer />

      <Box
        component="main"
        style={{
          padding: hideAppBar ? "0 0 24px 0" : "64px 0 24px 0",
        }}
        sx={{ flexGrow: 1, p: 3 }}
      >
        {/* Sync status banner for edge gateway configuration sync */}
        <SyncStatusBanner />
        <Outlet />
      </Box>
    </Box>
  );
};

export default MainLayout;
