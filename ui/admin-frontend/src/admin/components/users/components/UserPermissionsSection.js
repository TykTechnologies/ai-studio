import React from "react";
import { Typography, Grid, Box, FormControlLabel, Switch } from "@mui/material";
import Section from "../../common/Section";
import { PermissionsContainer } from "../styles";

const UserPermissionsSection = React.memo(({
  isAdmin,
  setIsAdmin,
  showPortal,
  setShowPortal,
  showChat,
  setShowChat,
  emailVerified,
  setEmailVerified,
  notificationsEnabled,
  setNotificationsEnabled,
  accessToSSOConfig,
  setAccessToSSOConfig
}) => {
  return (
    <Section>
      <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 3 }}>
        User Permissions
      </Typography>
      
      <Grid container>
        <Grid item xs={12} md={6} lg={4}>
          <FormControlLabel
            control={
              <Switch
                checked={isAdmin}
                onChange={(e) => setIsAdmin(e.target.checked)}
                color="primary"
              />
            }
            label="Admin User"
          />
          
          <PermissionsContainer>
            <FormControlLabel
              control={
                <Switch
                  checked={showPortal}
                  onChange={(e) => setShowPortal(e.target.checked)}
                  color="primary"
                />
              }
              label="Show Portal"
            />
          </PermissionsContainer>
          
          <PermissionsContainer>
            <FormControlLabel
              control={
                <Switch
                  checked={showChat}
                  onChange={(e) => setShowChat(e.target.checked)}
                  color="primary"
                />
              }
              label="Show Chat"
            />
          </PermissionsContainer>
          
          <PermissionsContainer>
            <FormControlLabel
              control={
                <Switch
                  checked={emailVerified}
                  onChange={(e) => setEmailVerified(e.target.checked)}
                  color="primary"
                />
              }
              label="Email Verified"
            />
          </PermissionsContainer>
        </Grid>
        
        <Grid item xs={12} md={6} lg={8}>
          {isAdmin && (
            <>
              <FormControlLabel
                control={
                  <Switch
                    checked={notificationsEnabled}
                    onChange={(e) => setNotificationsEnabled(e.target.checked)}
                    color="primary"
                  />
                }
                label="Enable Notifications"
              />
              
              <PermissionsContainer>
                <FormControlLabel
                  control={
                    <Switch
                      checked={accessToSSOConfig}
                      onChange={(e) => setAccessToSSOConfig(e.target.checked)}
                      color="primary"
                    />
                  }
                  label="Enable access to IdP configuration"
                />
              </PermissionsContainer>
            </>
          )}
        </Grid>
      </Grid>
    </Section>
  );
});

export default UserPermissionsSection;