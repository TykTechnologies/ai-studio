import React from "react";
import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Typography from "@mui/material/Typography";
import LogoutIcon from "@mui/icons-material/Logout";
import LaunchIcon from "@mui/icons-material/Launch"; // Changed to Launch icon
import { useNavigate } from "react-router-dom";
import { styled } from "@mui/material/styles";
import logo from "./logo.svg";
import { StyledIconButton } from "../../styles/sharedStyles";
import apiClient from "../../utils/apiClient";

const Logo = styled("img")(({ theme }) => ({
  height: "40px",
  marginRight: theme.spacing(2),
}));

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
        <Logo src={logo} alt="Logo" />
        <Typography
          variant="h4"
          color="white"
          noWrap
          sx={{ flexGrow: 1, fontWeight: "bold" }}
        >
          AI Portal
        </Typography>

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
