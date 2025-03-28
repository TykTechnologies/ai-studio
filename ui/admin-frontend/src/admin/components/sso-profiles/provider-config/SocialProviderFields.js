import React from "react";
import { Box, Typography, Stack } from "@mui/material";

/**
 * Component for displaying Social Provider specific configuration fields
 * 
 * @param {Object} props - Component props
 * @param {Object} props.profileData - The profile data to display
 * @param {Function} props.handleCopyToClipboard - Function to handle copying to clipboard
 * @returns {React.ReactElement}
 */
const SocialProviderFields = ({ profileData, handleCopyToClipboard }) => {
  return (
    <Stack spacing={2}>
      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '25%' }}}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Social Provider
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' }}}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            {profileData.ProviderConfig?.UseProviders?.[0]?.Name || "-"}
          </Typography>
        </Box>
      </Stack>

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
            </Box>
          </Box>
        </Box>
        
        <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center"}}>
          <Box sx={{ width: { xs: '50%', md: '50%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Secret
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '50%', md: '50%' }, pt: 1 }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {"*".repeat(8)}
            </Typography>
          </Box>
        </Box>
      </Stack>
    </Stack>
  );
};

export default SocialProviderFields;