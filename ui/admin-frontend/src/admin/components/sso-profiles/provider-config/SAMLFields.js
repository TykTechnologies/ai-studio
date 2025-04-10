import React from "react";
import { Box, Typography, Stack, IconButton } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import AdvancedSettingsSection from "../AdvancedSettingsSection";

/**
 * Component for displaying SAML specific provider configuration fields
 * 
 * @param {Object} props - Component props
 * @param {Object} props.profileData - The profile data to display
 * @param {Function} props.handleCopyToClipboard - Function to handle copying to clipboard
 * @returns {React.ReactElement}
 */
const SAMLFields = ({ profileData, handleCopyToClipboard }) => {
  return (
    <Stack spacing={2}>
      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '25%' } }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Certificate path
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.CertLocation || "-"}
            </Typography>
            {profileData.ProviderConfig?.CertLocation && (
              <IconButton
                size="small"
                onClick={() => handleCopyToClipboard(profileData.ProviderConfig.CertLocation, "Certificate path")}
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
            IDP metadata URL
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' } }}>
          <Box sx={{ display: "flex", alignItems: "center" }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.IDPMetaDataURL || "-"}
            </Typography>
            {profileData.ProviderConfig?.IDPMetaDataURL && (
              <IconButton
                size="small"
                onClick={() => handleCopyToClipboard(profileData.ProviderConfig.IDPMetaDataURL, "IDP metadata URL")}
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

      {/* Advanced settings for SAML */}
      <AdvancedSettingsSection>
        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '25%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              SAML email claim
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '100%', md: '75%' } }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.SAMLEmailClaim || "-"}
            </Typography>
          </Box>
        </Stack>

        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center" }}>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                SAML forename
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {profileData.ProviderConfig?.SAMLForenameClaim || "-"}
              </Typography>
            </Box>
          </Box>
          
          <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center"}}>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                SAML surname
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {profileData.ProviderConfig?.SAMLSurnameClaim || "-"}
              </Typography>
            </Box>
          </Box>
        </Stack>

        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '25%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Force authentication
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '100%', md: '75%' } }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.ForceAuthentication?.toString() || "false"}
            </Typography>
          </Box>
        </Stack>

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
              Provider domain
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '100%', md: '75%' } }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConstraintsDomain || "-"}
            </Typography>
          </Box>
        </Stack>
      </AdvancedSettingsSection>
    </Stack>
  );
};

export default SAMLFields;