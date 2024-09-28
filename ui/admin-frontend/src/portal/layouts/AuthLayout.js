import React from "react";
import { Outlet } from "react-router-dom";
import { Box, CssBaseline } from "@mui/material";
import PortalAppBar from "../components/PortalAppBar";
import { useTheme } from "@mui/material/styles";

const AuthLayout = () => {
  const theme = useTheme();

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100vh" }}>
      <CssBaseline />
      <PortalAppBar />
      <Box
        component="main"
        sx={{
          flexGrow: 1,
          p: 3,
          mt: theme.spacing(8),
          backgroundColor: theme.palette.background.default,
        }}
      >
        <Outlet />
      </Box>
    </Box>
  );
};

export default AuthLayout;
