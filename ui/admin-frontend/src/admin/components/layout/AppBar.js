import React from "react";
import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Box from "@mui/material/Box";
import LogoutIcon from "@mui/icons-material/Logout";
import LaunchIcon from "@mui/icons-material/Launch"; // Changed to Launch icon
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";
import { useNavigate } from "react-router-dom";
import { styled } from "@mui/material/styles";
import { StyledIconButton } from "../../styles/sharedStyles";
import apiClient from "../../utils/apiClient";

const StyledLink = styled("a")(({ theme }) => ({
  color: "white",
  textDecoration: "none",
  display: "flex",
  alignItems: "center",
  marginRight: theme.spacing(2),
  cursor: "pointer",
}));

const MyAppBar = () => {
  const navigate = useNavigate();

  const handleLogout = async () => {
    try {
      await apiClient.post("/logout");
      localStorage.removeItem("token");
      navigate("/login");
    } catch (error) {
      console.error("Logout failed:", error);
    }
  };

  return (
    <AppBar
      position="fixed"
      sx={{
        zIndex: (theme) => theme.zIndex.drawer + 1,
        backgroundColor: "#03031c",
      }}
    >
      <Toolbar>
        <Box sx={{ display: "flex", alignItems: "center", flexGrow: 1 }}>
          <img
            src="/logos/tyk-portal-logo.png"
            alt="Midsommar Logo"
            style={{
              height: "40px",
              marginRight: "5px",
            }}
          />
        </Box>

        <StyledLink
          href={`${window.location.protocol}//${window.location.hostname}:8989/docs/quickstart`}
          target="_blank"
          rel="noopener noreferrer"
        >
          <LibraryBooksIcon sx={{ mr: 1 }} />
          Docs
        </StyledLink>
        <StyledLink
          href="/portal/dashboard"
          target="_blank"
          rel="noopener noreferrer"
        >
          <LaunchIcon sx={{ mr: 1 }} />
          Portal Dashboard
        </StyledLink>

        <StyledIconButton color="inherit" onClick={handleLogout}>
          <LogoutIcon />
        </StyledIconButton>
      </Toolbar>
    </AppBar>
  );
};

export default MyAppBar;
