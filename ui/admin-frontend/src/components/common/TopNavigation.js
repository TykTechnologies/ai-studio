import React from "react";
import LogoutIcon from "@mui/icons-material/Logout";
import { logout } from "../../admin/utils/pubClient";
import {
  StyledAppBar,
  StyledToolbar,
  NavigationContainer,
  LogoContainer,
  Logo,
  TabsContainer,
  StyledTabs,
  StyledTab,
  StyledLogoutButton,
  TabIndicatorProps,
} from "./styles";

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
      label: "Chat",
      value: "chat",
      dataTestId: "chat-tab",
    });
  }

  if (showPortal) {
    tabs.push({
      label: "Developer Portal",
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

  return (
    <StyledAppBar position="fixed">
      <StyledToolbar>
        <NavigationContainer>
          <LogoContainer>
            <Logo
              src="/logos/tyk-portal-logo.png"
              alt="Logo"
            />
          </LogoContainer>
          <TabsContainer>
            <StyledTabs
              value={currentTab}
              onChange={(e, value) => onTabChange(value)}
              textColor="inherit"
              TabIndicatorProps={TabIndicatorProps}
            >
              {tabs.map((tab) => (
                <StyledTab
                  key={tab.value}
                  label={tab.label}
                  value={tab.value}
                  data-testid={tab.dataTestId}
                />
              ))}
            </StyledTabs>
          </TabsContainer>
        </NavigationContainer>
        <StyledLogoutButton onClick={logout}>
          <LogoutIcon />
        </StyledLogoutButton>
      </StyledToolbar>
    </StyledAppBar>
  );
};

export default TopNavigation;
