import React from "react";
import Box from "@mui/material/Box";
import Toolbar from "@mui/material/Toolbar";
import MyAppBar from "./AppBar";
import MyDrawer from "./Drawer";
import { Outlet } from "react-router-dom";

const MainLayout = () => {
  return (
    <Box sx={{ display: "flex" }}>
      <MyAppBar />
      <MyDrawer />

      <Box component="main" style={{
        padding: "64px 0 24px 0",
      }} sx={{ flexGrow: 1, p: 3 }}>
        <Outlet />
      </Box>
    </Box>
  );
};

export default MainLayout;
