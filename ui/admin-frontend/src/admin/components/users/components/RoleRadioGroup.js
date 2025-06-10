import React, { useState } from 'react';
import {
  Box,
  Typography,
  RadioGroup,
  FormControlLabel,
  Radio,
  FormControl,
} from '@mui/material';
import { roleBadgeConfigs } from '../../groups/utils/roleBadgeConfig';
import Icon from '../../../../components/common/Icon';

const RoleRadioGroup = ({ value, onChange, isSuperAdmin }) => {
  const [hoveredRole, setHoveredRole] = useState(null);

  const roles = [
    {
      value: 'Chat user',
      label: 'Chat user',
      connector: 'can access',
      main: 'Chats',
      permissions: {
        sections: [
          {
            title: 'Access to Chats',
            items: [
              'Interact with Chats',
              'Add data sources and tools available in their catalogs to chats'
            ]
          }
        ]
      }
    },
    {
      value: 'Developer',
      label: 'Developer',
      connector: 'can access',
      main: 'AI portal and Chats',
      permissions: {
        sections: [
          {
            title: 'Access to Chats',
            items: [
              'Interact with Chats',
              'Add data sources and tools available in their catalogs to chats'
            ]
          },
          {
            title: 'Access to AI Portal',
            items: [
              'Use Apps created by the admin',
              'Create and delete their own apps with LLM providers and data sources available in their catalogs'
            ]
          }
        ]
      }
    },
    ...(isSuperAdmin ? [{
      value: 'Admin',
      label: 'Admin',
      connector: 'can access',
      main: 'Admin, AI portal and Chats',
      permissions: {
        sections: [
          {
            title: 'Access to Chats',
            items: [
              'Interact with Chats',
              'Add data sources and tools available in their catalogs to chats'
            ]
          },
          {
            title: 'Access to AI Portal',
            items: [
              'Use Apps created by the admin',
              'Create and delete their own apps with LLM providers and data sources available in their catalogs'
            ]
          },
          {
            title: 'Access to Administration',
            items: [
              'CRUD LLM providers, data sources, tools, filters, middleware, Apps, Chats and catalogs',
              'Add, edit and delete Chat users and Developers.',
              'Add, edit and delete Teams.',
              'Monitor usage, iterations, and costs (set up budgets).'
            ]
          }
        ]
      }
    }] : [])
  ];

  const handleRoleChange = (event) => {
    onChange(event.target.value);
  };

  return (
    <Box sx={{ position: 'relative', display: 'flex', gap: 3 }}>
      <Box sx={{ flex: 1 }}>
        <FormControl component="fieldset" sx={{ width: '100%' }}>
          <RadioGroup
            name="role"
            value={value}
            onChange={handleRoleChange}
          >
            {roles.map((role, index) => {
              const config = roleBadgeConfigs[role.value];
              
              return (
                <Box
                  key={role.value}
                  onMouseEnter={() => setHoveredRole(role.value)}
                  onMouseLeave={() => setHoveredRole(null)}
                  sx={{
                    border: '1px solid',
                    borderColor: 'border.neutralDefault',
                    borderRadius: '8px',
                    p: 2,
                    mb: index < roles.length - 1 ? 2 : 0,
                    '&:hover': {
                      backgroundColor: 'background.surfaceNeutralHover'
                    }
                  }}
                >
                  <FormControlLabel
                    value={role.value}
                    control={
                      <Radio
                        sx={{
                          '&.Mui-checked': {
                            color: theme => theme.palette.background.buttonPrimaryDefault
                          }
                        }}
                      />
                    }
                    label={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Box
                          sx={{
                            backgroundColor: config.bgColor,
                            borderRadius: '6px',
                            px: 1,
                            py: 0.5
                          }}
                        >
                          <Typography
                            variant={config.textVariant}
                            sx={{ color: config.textColor }}
                          >
                            {config.text}
                          </Typography>
                        </Box>
                        <Typography
                          variant="bodyLargeDefault"
                          sx={{ color: 'text.defaultSubdued' }}
                        >
                          {role.connector}
                        </Typography>
                        <Typography
                          variant="bodyLargeMedium"
                          sx={{ color: 'text.defaultSubdued' }}
                        >
                          {role.main}
                        </Typography>
                      </Box>
                    }
                    sx={{ margin: 0, width: '100%' }}
                  />
                </Box>
              );
            })}
          </RadioGroup>
        </FormControl>
      </Box>

      {/* Hover Info Container */}
      {hoveredRole && (
        <Box
          sx={{
            position: 'absolute',
            right: { xs: '-280px', lg: '-320px' },
            top: 0,
            width: { xs: '260px', lg: '300px' },
            border: '1px solid',
            borderColor: 'border.neutralDefault',
            borderRadius: '8px',
            backgroundColor: 'background.paper',
            p: 2,
            zIndex: 1000,
            boxShadow: 2,
            // Ensure tooltip stays within viewport on smaller screens
            '@media (max-width: 1200px)': {
              right: 'auto',
              left: '100%',
              ml: 2
            }
          }}
        >
          {roles.map(role => {
            if (role.value !== hoveredRole) return null;
            
            return (
              <Box key={role.value}>
                {role.permissions.sections.map((section, sectionIndex) => (
                  <Box key={sectionIndex} sx={{ mb: sectionIndex < role.permissions.sections.length - 1 ? 3 : 0 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
                      <Icon
                        name="circle-check"
                        sx={{
                          width: 16,
                          height: 16,
                          color: theme => theme.palette.background.iconSuccessDefault
                        }}
                      />
                      <Typography
                        variant="bodyMediumSemiBold"
                        sx={{ color: 'text.defaultSubdued' }}
                      >
                        {section.title}
                      </Typography>
                    </Box>
                    
                    <Box component="ul" sx={{ margin: 0, paddingLeft: 2 }}>
                      {section.items.map((item, itemIndex) => (
                        <Box
                          component="li"
                          key={itemIndex}
                          sx={{ mb: 0.5 }}
                        >
                          <Typography
                            variant="bodySmallDefault"
                            sx={{ color: 'text.defaultSubdued' }}
                          >
                            {item}
                          </Typography>
                        </Box>
                      ))}
                    </Box>
                  </Box>
                ))}
              </Box>
            );
          })}
        </Box>
      )}
    </Box>
  );
};

export default RoleRadioGroup; 