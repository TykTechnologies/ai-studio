import { memo } from "react";
import { Box, Typography, FormControlLabel } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import RoleRadioGroup from "./RoleRadioGroup";
import RolePermissionsDisplay from "./RolePermissionsDisplay";
import { LearnMoreLink, StyledSwitch } from "../../../styles/sharedStyles";
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
      <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
        Assign a role to this user to control their access levels to features and actions in the AI studio platform.
        <LearnMoreLink onClick={createDocsLinkHandler(getDocsLink, 'teams')} />
      </Typography>

      <Box position="relative" display="flex" mt={3} width="100%" gap={3}>
        <Box display="flex" flexDirection="column" width="50%">
          <RoleRadioGroup
            value={selectedRole}
            onChange={handleRoleChange}
            isSuperAdmin={isSuperAdmin}
          />

          {selectedRole === 'Admin' && (
            <Box mt={3} ml={5} display="flex" flexDirection="column" gap={1}>
              <FormControlLabel
                control={
                  <StyledSwitch
                    checked={notificationsEnabled}
                    onChange={(e) => setNotificationsEnabled(e.target.checked)}
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
                  <StyledSwitch
                    checked={accessToSSOConfig}
                    onChange={(e) => setAccessToSSOConfig(e.target.checked)}
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
        </Box>
        <RolePermissionsDisplay
          width="50%"
          selectedRole={selectedRole}
          isSuperAdmin={isSuperAdmin}
        />
      </Box>
    </CollapsibleSection>
  );
});

UserPermissionsSection.displayName = 'UserPermissionsSection';

export default UserPermissionsSection;