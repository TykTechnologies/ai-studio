import React from "react";
import { Box, Typography, Stack, IconButton } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";

/**
 * Component for displaying common provider configuration fields
 *
 * @param {Object} props - Component props
 * @param {Object} props.profileData - The profile data to display
 * @param {Object} props.profileMetadata - Additional profile metadata including URLs
 * @param {Function} props.handleCopyToClipboard - Function to handle copying to clipboard
 * @returns {React.ReactElement}
 */
const CommonProviderFields = ({ profileData, profileMetadata, handleCopyToClipboard }) => {
  return (
    <Stack spacing={2}>
      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Login URL
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileMetadata.loginUrl || "-"}
            </Typography>
            {profileMetadata.loginUrl && (
              <IconButton
                size="small"
                onClick={() => handleCopyToClipboard(profileMetadata.loginUrl, "Login URL")}
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

      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Callback URL
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileMetadata.callbackUrl || "-"}
            </Typography>
            {profileMetadata.callbackUrl && (
              <IconButton
                size="small"
                onClick={() => handleCopyToClipboard(profileMetadata.callbackUrl, "Callback URL")}
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

      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Access URL
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.CallbackBaseURL || profileData.ProviderConfig?.SAMLBaseURL || "-"}
            </Typography>
            {(profileData.ProviderConfig?.CallbackBaseURL || profileData.ProviderConfig?.SAMLBaseURL) && (
              <IconButton
                size="small"
                onClick={() => handleCopyToClipboard(profileData.ProviderConfig.CallbackBaseURL || profileData.ProviderConfig.SAMLBaseURL, "Access URL")}
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
    </Stack>
  );
};

export default CommonProviderFields;