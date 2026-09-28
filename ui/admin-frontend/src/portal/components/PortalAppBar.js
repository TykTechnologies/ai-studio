import React, { useState } from "react";
import { AppBar, Toolbar, Typography, IconButton, Box } from "@mui/material";
import LogoutIcon from "@mui/icons-material/Logout";
import apiClient from "../../admin/utils/pubClient";
import { redirectToLogout } from "../../admin/utils/authRedirect";
import { withBase } from "../../runtimeConfig";

const PortalAppBar = () => {
  const [isLoggedOut, setIsLoggedOut] = useState(false);

  const handleLogout = async () => {
    try {
      await apiClient.post("/common/logout");
    } catch (error) {
      console.error("Logout failed:", error);
    }
    localStorage.removeItem("token");
    setIsLoggedOut(true); // Force a re-render
    // A full page load, so nothing of the signed-in session lingers; to the
    // host application's sign-out when it authenticates users.
    redirectToLogout();
  };

  if (isLoggedOut) {
    return null;
  }

  return (
    <AppBar
      position="fixed"
      sx={(theme) => ({
        zIndex: theme.zIndex.drawer + 1,
        background: "linear-gradient(91deg, #03031C 12.29%, #8438FA 92.06%, #B421FA 105%)",
        boxShadow: "none",
        borderBottom: "none",
      })}
    >
      <Toolbar>
        <Box sx={{ display: "flex", alignItems: "center", flexGrow: 1 }}>
          <img
            src={withBase("/logos/tyk-portal-logo.png")}
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
