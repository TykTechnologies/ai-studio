import React from "react";
import { Box, Typography, Stack, IconButton } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";

/**
 * Component for displaying the profile details section
 *
 * @param {Object} props - Component props
 * @param {Object} props.profileData - The profile data to display
 * @param {Object} props.profileMetadata - Additional profile metadata including URLs
 * @param {Function} props.handleCopyToClipboard - Function to handle copying to clipboard
 * @returns {React.ReactElement}
 */
const ProfileDetailsSection = ({ profileData, profileMetadata, handleCopyToClipboard }) => {
  return (
    <Stack spacing={2}>
      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Profile name
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            {profileData.Name || "-"}
          </Typography>
        </Box>
      </Stack>

      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Profile type
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            {profileData.ActionType || "-"}
          </Typography>
        </Box>
      </Stack>

      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Provider type
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            {profileMetadata.selectedProviderType || "-"}
          </Typography>
        </Box>
      </Stack>

      <Stack direction={{ xs: 'column', md: 'row'}} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Redirect URL on failure
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' }}}>
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileMetadata.failureRedirectUrl || "-"}
            </Typography>
            {profileMetadata.failureRedirectUrl && (
              <IconButton
                size="small"
                onClick={() => handleCopyToClipboard(profileMetadata.failureRedirectUrl, "Redirect URL on failure")}
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

      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Default profile for SSO at Login page
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            { profileMetadata.useInLoginPage ? "Yes" : "No" }
          </Typography>
        </Box>
      </Stack>
    </Stack>
  );
};

export default ProfileDetailsSection;