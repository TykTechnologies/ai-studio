import React from "react";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import MyAppBar from "./AppBar";
import Drawer from "./Drawer";
import { Outlet } from "react-router-dom";
import SyncStatusBanner from "../common/SyncStatusBanner";
import { CONTENT_MAX_WIDTH } from "../../../constants/layout";

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
        sx={{ flexGrow: 1, p: 3, minWidth: 0 }}
      >
        {/* Constrain page content to a readable width (same as the Plugin Marketplace),
            centred in the space beside the drawer. Pages keep their own horizontal
            gutters, so no gutters are added here. */}
        <Container maxWidth={CONTENT_MAX_WIDTH} disableGutters>
          {/* Sync status banner for edge gateway configuration sync */}
          <SyncStatusBanner />
          <Outlet />
        </Container>
      </Box>
    </Box>
  );
};

export default MainLayout;
