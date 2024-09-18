import React from "react";
import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Typography from "@mui/material/Typography";

import NotificationsIcon from "@mui/icons-material/Notifications";
import LogoutIcon from "@mui/icons-material/Logout";
import { useNavigate } from "react-router-dom";
import { styled } from "@mui/material/styles";
import logo from "./logo.svg"; // Make sure this path is correct
import { StyledIconButton } from "../../styles/sharedStyles";

const Logo = styled("img")(({ theme }) => ({
  height: "40px", // Adjust this value to fit your needs
  marginRight: theme.spacing(2),
}));

const MyAppBar = () => {
  const navigate = useNavigate();

  const handleLogout = () => {
    localStorage.removeItem("token");
    navigate("/login");
  };

  return (
    <AppBar
      position="fixed"
      sx={{
        zIndex: (theme) => theme.zIndex.drawer + 1,
        backgroundColor: "#03031c", // Teal color
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

        <StyledIconButton color="inherit" onClick={handleLogout}>
          <LogoutIcon />
        </StyledIconButton>
      </Toolbar>
    </AppBar>
  );
};

export default MyAppBar;
