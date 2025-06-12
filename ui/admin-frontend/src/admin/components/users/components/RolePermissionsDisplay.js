import { Box, Typography } from '@mui/material';
import { USER_ROLES } from '../utils/userRolesConfig';
import {
  PermissionsTooltipBox,
  StyledPermissionIcon
} from '../styles';

const RolePermissionsDisplay = ({ selectedRole, isSuperAdmin }) => {
  if (!selectedRole) return null;

  const roles = [
    ...USER_ROLES.slice(0, 2),
    ...(isSuperAdmin ? [USER_ROLES[2]] : [])
  ];

  const currentRole = roles.find(role => role.value === selectedRole);
  if (!currentRole) return null;

  return (
    <PermissionsTooltipBox width="50%">
      <Box>
        {currentRole.permissions.sections.map((section, sectionIndex) => (
          <Box key={sectionIndex} mb={sectionIndex < currentRole.permissions.sections.length - 1 ? 2 : 0}>
            <Box display="flex" alignItems="center" gap={1} mb={0.5}>
              <StyledPermissionIcon name="circle-check" />
              <Typography
                variant="bodyMediumSemiBold"
                color="text.defaultSubdued"
              >
                {section.title}
              </Typography>
            </Box>
            
            <Box component="ul" m={0} pl={3}>
              {section.items.map((item, itemIndex) => (
                <Box
                  component="li"
                  key={itemIndex}
                >
                  <Typography
                    variant="bodySmallDefault"
                    color="text.defaultSubdued"
                  >
                    {item}
                  </Typography>
                </Box>
              ))}
            </Box>
          </Box>
        ))}
      </Box>
    </PermissionsTooltipBox>
  );
};

export default RolePermissionsDisplay; 