import {
  Box,
  Typography,
  RadioGroup,
  FormControlLabel,
  FormControl,
} from '@mui/material';
import { roleBadgeConfigs } from '../../groups/utils/roleBadgeConfig';
import { USER_ROLES } from '../utils/userRolesConfig';
import {
  RoleOptionBox,
  RoleBadge,
} from '../styles';
import { StyledRadio } from '../../../styles/sharedStyles';

const RoleRadioGroup = ({ value, onChange, isSuperAdmin }) => {
  const roles = [
    ...USER_ROLES.slice(0, 2),
    ...(isSuperAdmin ? [USER_ROLES[2]] : [])
  ];

  const handleRoleChange = (event) => {
    onChange(event.target.value);
  };

  return (
    <FormControl component="fieldset">
      <RadioGroup
        name="role"
        value={value}
        onChange={handleRoleChange}
      >
        {roles.map((role, index) => {
          const config = roleBadgeConfigs[role.value];
          
          return (
              <RoleOptionBox
                key={role.value}
                isLast={index === roles.length - 1}
                value={role.value}
                control={<StyledRadio />}
                label={
                  <Box display="flex" alignItems="center" gap={1}>
                    <RoleBadge bgColor={config.bgColor}>
                      <Typography
                        variant={config.textVariant}
                        color={config.textColor}
                      >
                        {config.text}
                      </Typography>
                    </RoleBadge>
                    <Typography
                      variant="bodyLargeDefault"
                      color="text.defaultSubdued"
                    >
                      {role.connector}
                    </Typography>
                    <Typography
                      variant="bodyLargeMedium"
                      color="text.defaultSubdued"
                    >
                      {role.main}
                    </Typography>
                  </Box>
                }
              />
          );
        })}
      </RadioGroup>
    </FormControl>
  );
};

export default RoleRadioGroup;