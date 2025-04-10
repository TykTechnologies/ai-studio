import React from "react";
import { Box, Typography, Stack } from "@mui/material";
import AdvancedSettingsSection from "../AdvancedSettingsSection";

/**
 * Component for displaying LDAP specific provider configuration fields
 * 
 * @param {Object} props - Component props
 * @param {Object} props.profileData - The profile data to display
 * @returns {React.ReactElement}
 */
const LDAPFields = ({ profileData }) => {
  return (
    <Stack spacing={2}>
      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center" }}>
          <Box sx={{ width: { xs: '50%', md: '50%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Server
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '50%', md: '50%' } }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.LDAPServer || "-"}
            </Typography>
          </Box>
        </Box>
        
        <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center", mt: { xs: 2, md: 0 } }}>
          <Box sx={{ width: { xs: '50%', md: '50%' } }}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Port
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '50%', md: '50%' } }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.LDAPPort || "-"}
            </Typography>
          </Box>
        </Box>
      </Stack>

      <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
        <Box sx={{ width: { xs: '100%', md: '25%' }}}>
          <Typography variant="bodyLargeBold" color="text.primary">
            User DN
          </Typography>
        </Box>
        <Box sx={{ width: { xs: '100%', md: '75%' }}}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            {profileData.ProviderConfig?.LDAPUserDN || "-"}
          </Typography>
        </Box>
      </Stack>

      {/* Advanced settings for LDAP */}
      <AdvancedSettingsSection>
        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '25%' }}}>
            <Typography variant="bodyLargeBold" color="text.primary">
              LDAP attributes
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '100%', md: '75%' }}}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.LDAPAttributes?.join(", ") || "-"}
            </Typography>
          </Box>
        </Stack>

        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '25%' }}}>
            <Typography variant="bodyLargeBold" color="text.primary">
              Use SSL
            </Typography>
          </Box>
          <Box sx={{ width: { xs: '100%', md: '75%' }}}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              {profileData.ProviderConfig?.LDAPUseSSL?.toString() || "false"}
            </Typography>
          </Box>
        </Stack>
      </AdvancedSettingsSection>
    </Stack>
  );
};

export default LDAPFields;