import React, { useState, useEffect } from "react"; // Add useState and useEffect
import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Box from "@mui/material/Box";
import LogoutIcon from "@mui/icons-material/Logout";
import LaunchIcon from "@mui/icons-material/Launch";
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";
import { useNavigate } from "react-router-dom";
import { styled } from "@mui/material/styles";
import { StyledIconButton } from "../../styles/sharedStyles";
import pubClient, { logout } from "../../utils/pubClient";

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
  const [docsUrl, setDocsUrl] = useState(""); // Add state for docs URL

  // Fetch system settings when component mounts
  useEffect(() => {
    const fetchSystemSettings = async () => {
      try {
        const response = await pubClient.get("/common/system");
        setDocsUrl(response.data.features.docs_url + "/docs/quickstart");
      } catch (error) {
        console.error("Failed to fetch system settings:", error);
      }
    };

    fetchSystemSettings();
  }, []);

  const handleLogout = () => {
    logout();
  };

  return (
    <AppBar
      position="fixed"
      sx={{
        zIndex: (theme) => theme.zIndex.drawer + 1,
        background: "linear-gradient(91deg, #03031C 12.29%, #8438FA 92.06%, #B421FA 105%)",
      }}
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

        {docsUrl && ( // Conditionally render docs link
          <StyledLink href={docsUrl} target="_blank" rel="noopener noreferrer">
            <LibraryBooksIcon sx={{ mr: 1 }} />
            Docs
          </StyledLink>
        )}

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
