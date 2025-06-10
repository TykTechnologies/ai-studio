import { memo } from "react";
import { Box, Typography, FormControlLabel, Switch } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import RoleRadioGroup from "./RoleRadioGroup";
import { LearnMoreLink } from "../../../styles/sharedStyles";
import { createDocsLinkHandler } from "../../../utils/docsLinkUtils";
import useConfig from "../../../hooks/useConfig";

const UserPermissionsSection = memo(({
  isSuperAdmin,
  notificationsEnabled,
  setNotificationsEnabled,
  accessToSSOConfig,
  setAccessToSSOConfig,
  selectedRole,
  setSelectedRole
}) => {
  const { getDocsLink } = useConfig();

  const handleRoleChange = (role) => {
    setSelectedRole(role);
  };

  return (
    <CollapsibleSection title="Roles & permissions*" defaultExpanded={true}>
      <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mb: 3 }}>
        Assign a role to this user to control their access levels to features and actions in the AI studio platform.
        <LearnMoreLink onClick={createDocsLinkHandler(getDocsLink, 'teams')} />
      </Typography>

      <RoleRadioGroup
        value={selectedRole}
        onChange={handleRoleChange}
        isSuperAdmin={isSuperAdmin}
      />

      {selectedRole === 'Admin' && (
        <Box sx={{ mt: 3, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <FormControlLabel
            control={
              <Switch
                checked={notificationsEnabled}
                onChange={(e) => setNotificationsEnabled(e.target.checked)}
                sx={{
                  '& .MuiSwitch-switchBase.Mui-checked': {
                    color: theme => theme.palette.background.buttonPrimaryDefault
                  },
                  '& .MuiSwitch-switchBase.Mui-checked + .MuiSwitch-track': {
                    backgroundColor: theme => theme.palette.background.buttonPrimaryDefault
                  }
                }}
              />
            }
            label={
              <Typography variant="bodyLargeBold" color="text.primary">
                Enable Notifications
              </Typography>
            }
          />

          <FormControlLabel
            control={
              <Switch
                checked={accessToSSOConfig}
                onChange={(e) => setAccessToSSOConfig(e.target.checked)}
                sx={{
                  '& .MuiSwitch-switchBase.Mui-checked': {
                    color: theme => theme.palette.background.buttonPrimaryDefault
                  },
                  '& .MuiSwitch-switchBase.Mui-checked + .MuiSwitch-track': {
                    backgroundColor: theme => theme.palette.background.buttonPrimaryDefault
                  }
                }}
              />
            }
            label={
              <Typography variant="bodyLargeBold" color="text.primary">
                Allow Identity provider configuration
              </Typography>
            }
          />
        </Box>
      )}
    </CollapsibleSection>
  );
});

UserPermissionsSection.displayName = 'UserPermissionsSection';

export default UserPermissionsSection;