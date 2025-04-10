import React from "react";
import { Box, Typography, Stack, IconButton } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import AdvancedSettingsSection from "../AdvancedSettingsSection";

/**
 * Component for displaying OpenID Connect specific provider configuration fields
 * 
 * @param {Object} props - Component props
 * @param {Object} props.profileData - The profile data to display
 * @param {Function} props.handleCopyToClipboard - Function to handle copying to clipboard
 * @returns {React.ReactElement}
 */
const OpenIDConnectFields = ({ profileData, handleCopyToClipboard }) => {
  return (
    <Stack spacing={2}>
      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center" }}>
          <Box sx={{ width: { xs: '50%', md: '50%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Client ID/Key
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '50%', md: '50%' } }}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {profileData.ProviderConfig?.UseProviders?.[0]?.Key || "-"}
              </Typography>
              {profileData.ProviderConfig?.UseProviders?.[0]?.Key && (
                <IconButton
                  size="small"
                  onClick={() => handleCopyToClipboard(profileData.ProviderConfig.UseProviders[0].Key, "Client ID/Key")}
                  sx={{ ml: 1 }}
                >
                  <ContentCopyIcon sx={{
                    color: (theme) => theme.palette.text.defaultSubdued,
                    width: 16,
                    height: 16,
                  }} />
                </IconButton>
              )}
            </Box>
          </Box>
        </Box>
        
        <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center"}}>
          <Box sx={{ width: { xs: '50%', md: '50%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Secret
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '50%', md: '50%' }}}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {"*".repeat(8)}
            </Typography>
          </Box>
        </Box>
      </Stack>

      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Discover URL (well known endpoint)
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.UseProviders?.[0]?.DiscoverURL || "-"}
            </Typography>
            {profileData.ProviderConfig?.UseProviders?.[0]?.DiscoverURL && (
              <IconButton
                size="small"
                onClick={() => handleCopyToClipboard(profileData.ProviderConfig.UseProviders[0].DiscoverURL, "Discover URL")}
                sx={{ ml: 1 }}
              >
                <ContentCopyIcon sx={{
                  color: (theme) => theme.palette.text.defaultSubdued,
                  width: 16,
                  height: 16,
                }} />
              </IconButton>
            )}
          </Box>
        </Box>
      </Stack>

      {/* Advanced settings for OpenID Connect */}
      <AdvancedSettingsSection>
        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center" }}>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Custom email
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {profileData.CustomEmailField || "-"}
              </Typography>
            </Box>
          </Box>
          
          <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center" }}>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Custom ID
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {profileData.CustomUserIDField || "-"}
              </Typography>
            </Box>
          </Box>
        </Stack>

        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '25%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Skip user info request
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '100%', md: '75%' } }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.UseProviders?.[0]?.SkipUserInfoRequest?.toString() || "false"}
            </Typography>
          </Box>
        </Stack>

        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '25%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Scopes
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '100%', md: '75%' } }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.UseProviders?.[0]?.Scopes?.join(", ") || "-"}
            </Typography>
          </Box>
        </Stack>
      </AdvancedSettingsSection>
    </Stack>
  );
};

export default OpenIDConnectFields;