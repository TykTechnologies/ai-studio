import React from "react";
import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import NotificationsIcon from "@mui/icons-material/Notifications";
import LogoutIcon from "@mui/icons-material/Logout";
import { useNavigate } from "react-router-dom";
import { styled } from "@mui/material/styles";
import logo from "./logo.svg"; // Make sure this path is correct

const Logo = styled("img")(({ theme }) => ({
  height: "40px", // Adjust this value to fit your needs
  marginRight: theme.spacing(2),
}));

const StyledIconButton = styled(IconButton)(({ theme }) => ({
  "&:hover": {
    backgroundColor: "#20B2AA", // Light teal color for hover
  },
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
        backgroundColor: "#008080", // Teal color
      }}
    >
      <Toolbar>
        <Logo src={logo} alt="Logo" />
        <Typography variant="h5" noWrap sx={{ flexGrow: 1 }}>
          AI Portal
        </Typography>
        <StyledIconButton color="inherit">
          <NotificationsIcon />
        </StyledIconButton>
        <StyledIconButton color="inherit" onClick={handleLogout}>
          <LogoutIcon />
        </StyledIconButton>
      </Toolbar>
    </AppBar>
  );
};

export default MyAppBar;
