import React from "react";
import LogoutIcon from "@mui/icons-material/Logout";
import { styled } from "@mui/material/styles";
import { Box, Typography } from "@mui/material";
import NotificationIcon from "../../admin/components/notifications/NotificationIcon";
import Icon from "./Icon";

import { logout } from "../../admin/utils/pubClient";
import { createDocsLinkHandler } from "../../admin/utils/docsLinkUtils";
import useOverviewData from "../../admin/hooks/useOverviewData";
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

const LicenseAlert = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  padding: theme.spacing(1, 2),
  marginRight: theme.spacing(10),
  border: '1px solid rgba(255, 255, 255, 0.32)',
  borderRadius: '8px',
  background: 'linear-gradient(90.01deg, rgba(3, 3, 28, 0.1) 32.62%, rgba(255, 255, 255, 0.1) 94.21%)',
  backdropFilter: 'blur(4px)',
  gap: theme.spacing(1),
}));

const TopNavigation = ({
  showAdmin,
  showChat,
  showPortal,
  currentTab,
  onTabChange,
  onLogout,
}) => {
  const { licenseDaysLeft, getDocsLink } = useOverviewData();
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
      label: "AI Portal",
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
        {licenseDaysLeft && (
          <LicenseAlert>
            <Icon
              name="triangle-exclamation"
              sx={{
                color: "background.iconWarningDefault",
                width: 16,
                height: 16
              }}
            />
            <Typography variant="bodyLargeDefault" color="white">
              You have {licenseDaysLeft} days left in your trial
            </Typography>
            <Typography
              component="span"
              variant="bodyMediumSemiBold"
              color="text.linkDefault"
              sx={{ cursor: 'pointer' }}
              onClick={createDocsLinkHandler(getDocsLink, 'get_intouch_form')}
            >
              Get in touch
            </Typography>
          </LicenseAlert>
        )}
        <NotificationIcon sx={{ mr: 1 }} />
        <StyledLogoutButton onClick={logout}>
          <LogoutIcon />
        </StyledLogoutButton>
      </StyledToolbar>
    </StyledAppBar>
  );
};

export default TopNavigation;
