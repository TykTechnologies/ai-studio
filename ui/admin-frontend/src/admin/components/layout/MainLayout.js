import React from "react";
import Box from "@mui/material/Box";
import Toolbar from "@mui/material/Toolbar";
import MyAppBar from "./AppBar";
import MyDrawer from "./Drawer";

const MainLayout = ({ children }) => {
  return (
    <Box sx={{ display: "flex" }}>
      <MyAppBar />
      <MyDrawer />

      <Box component="main" sx={{ flexGrow: 1, p: 3 }}>
        <Toolbar />
        {children}
      </Box>
    </Box>
  );
};

export default MainLayout;
