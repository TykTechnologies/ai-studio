import React from "react";
import { AppBar, Toolbar, Tabs, Tab, Box, IconButton } from "@mui/material";
import LogoutIcon from "@mui/icons-material/Logout";
import { styled } from "@mui/material/styles";

import { logout } from "../../admin/utils/pubClient";

const StyledTab = styled(Tab)(({ theme }) => ({
  color: "white",
  "&.Mui-selected": {
    color: "#21ecba",
  },
  minHeight: "64px",
}));

const TopNavigation = ({
  showAdmin,
  showChat,
  showPortal,
  currentTab,
  onTabChange,
  onLogout,
}) => {
  const tabs = [];

  if (showChat) {
    tabs.push({
      label: "Chat Studio",
      value: "chat",
      dataTestId: "chat-tab",
    });
  }

  if (showPortal) {
    tabs.push({
      label: "AI Developer Portal",
      value: "portal",
      dataTestId: "portal-tab",
    });
  }

  if (showAdmin) {
    tabs.push({
      label: "Administration",
      value: "admin",
      dataTestId: "admin-tab",
    });
  }

  const handleLogout = async () => {
    await logout();
  };

  return (
    <AppBar
      position="fixed"
      sx={{
        zIndex: (theme) => theme.zIndex.drawer + 1,
        background:
          "linear-gradient(91deg, #03031C 12.29%, #8438FA 92.06%, #B421FA 105%)",
      }}
    >
      <Toolbar>
        <Box sx={{ display: "flex", alignItems: "center", flexGrow: 1 }}>
          <img
            src="/logos/tyk-portal-logo.png"
            alt="Logo"
            style={{
              height: "25px",
              marginRight: "20px",
            }}
          />
          <Tabs
            value={currentTab}
            onChange={(e, value) => {
              console.log("Tab clicked:", value); // Add this log
              onTabChange(value);
            }}
            textColor="inherit"
            TabIndicatorProps={{
              style: {
                backgroundColor: "#21ecba",
              },
            }}
          >
            {tabs.map((tab) => (
              <StyledTab
                key={tab.value}
                label={tab.label}
                value={tab.value}
                data-testid={tab.dataTestId}
              />
            ))}
          </Tabs>
        </Box>
        <IconButton onClick={handleLogout} sx={{ color: "white" }}>
          <LogoutIcon />
        </IconButton>
      </Toolbar>
    </AppBar>
  );
};

export default TopNavigation;
