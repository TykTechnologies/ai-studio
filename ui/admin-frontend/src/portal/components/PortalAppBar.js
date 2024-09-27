import React, { useState } from "react";
import { AppBar, Toolbar, Typography, IconButton } from "@mui/material";
import { useNavigate } from "react-router-dom";
import LogoutIcon from "@mui/icons-material/Logout";
import apiClient from "../../admin/utils/pubClient";

const PortalAppBar = () => {
  const navigate = useNavigate();
  const [isLoggedOut, setIsLoggedOut] = useState(false);

  const handleLogout = async () => {
    try {
      await apiClient.post("/common/logout");
      localStorage.removeItem("token");
      setIsLoggedOut(true); // Force a re-render

      // Try React Router navigation first
      navigate("/login");

      // If that doesn't work, use a timeout and then try window.location
      setTimeout(() => {
        if (window.location.pathname !== "/login") {
          console.log("Fallback: using window.location for navigation");
          window.location.href = "/login";
        }
      }, 100); // Small delay to allow for React navigation first
    } catch (error) {
      console.error("Logout failed:", error);
      // Still try to navigate even if logout fails
      navigate("/login");
    }
  };

  // If logged out, render nothing (or a loading state)
  if (isLoggedOut) {
    return null;
  }

  return (
    <AppBar
      position="fixed"
      sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}
    >
      <Toolbar>
        <Typography variant="h6" component="div" sx={{ flexGrow: 1 }}>
          Midsommar Portal
        </Typography>
        <IconButton color="inherit" onClick={handleLogout}>
          <LogoutIcon />
        </IconButton>
      </Toolbar>
    </AppBar>
  );
};

export default PortalAppBar;
