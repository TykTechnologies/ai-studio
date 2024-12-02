import React, { useState } from "react";
import { AppBar, Toolbar, Typography, IconButton, Box } from "@mui/material";
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

  if (isLoggedOut) {
    return null;
  }

  return (
    <AppBar
      position="fixed"
      sx={(theme) => ({
        zIndex: theme.zIndex.drawer + 1,
        backgroundColor: "#03031c",
        boxShadow: "none",
        borderBottom: "none",
      })}
    >
      <Toolbar>
        <Box sx={{ display: "flex", alignItems: "center", flexGrow: 1 }}>
          <img
            src="/logos/tyk-portal-logo.png"
            alt="Midsommar Logo"
            style={{
              height: "25px",
              marginRight: "5px",
            }}
          />
        </Box>
        <IconButton onClick={handleLogout} sx={{ color: "black" }}>
          <LogoutIcon style={{ color: 'white'}} />
        </IconButton>
      </Toolbar>
    </AppBar>
  );
};

export default PortalAppBar;
