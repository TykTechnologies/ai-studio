import React, { useState, useEffect } from "react"; // Add useState and useEffect
import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import LogoutIcon from "@mui/icons-material/Logout";
import LaunchIcon from "@mui/icons-material/Launch";
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";
import { styled } from "@mui/material/styles";
import { StyledIconButton } from "../../styles/sharedStyles";
import pubClient, { logout } from "../../utils/pubClient";
import { useEdition } from "../../context/EditionContext";

const StyledLink = styled("a")(({ theme }) => ({
  color: "white",
  textDecoration: "none",
  display: "flex",
  alignItems: "center",
  marginRight: theme.spacing(2),
  cursor: "pointer",
}));

const MyAppBar = () => {
  const [docsUrl, setDocsUrl] = useState(""); // Add state for docs URL
  const { version, isEnterprise } = useEdition(); // Get edition info

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
            title={version ? `Tyk AI Studio ${version}` : "Tyk AI Studio"}
            style={{
              height: "25px",
              marginRight: "10px",
              cursor: "pointer",
            }}
          />
          <Chip
            label={isEnterprise ? "Enterprise Edition" : "Community Edition"}
            size="small"
            sx={{
              backgroundColor: isEnterprise ? "#FFD700" : "#424242",
              color: isEnterprise ? "#000" : "#fff",
              fontWeight: 600,
              fontSize: "0.7rem",
              height: "20px",
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

        <StyledIconButton color="inherit" onClick={logout}>
          <LogoutIcon />
        </StyledIconButton>
      </Toolbar>
    </AppBar>
  );
};

export default MyAppBar;
