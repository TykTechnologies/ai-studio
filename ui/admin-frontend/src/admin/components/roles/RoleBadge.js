import React from 'react';
import { Chip } from '@mui/material';

/**
 * Small chip for a role: system roles get a distinct look so it is obvious
 * they cannot be edited.
 */
const RoleBadge = ({ role, size = 'small', sx = {} }) => {
  if (!role) return null;
  const name = role.name ?? role.attributes?.name ?? '';
  const isSystem = role.is_system ?? role.attributes?.is_system ?? false;
  return (
    <Chip
      label={name}
      size={size}
      variant={isSystem ? 'filled' : 'outlined'}
      color={isSystem ? 'primary' : 'default'}
      sx={{ mr: 0.5, mb: 0.5, ...sx }}
      data-testid={`role-badge-${role.slug ?? role.attributes?.slug ?? name}`}
    />
  );
};

export const SystemBadge = ({ isSystem }) => (
  <Chip
    label={isSystem ? 'System' : 'Custom'}
    size="small"
    color={isSystem ? 'primary' : 'default'}
    variant={isSystem ? 'filled' : 'outlined'}
  />
);

export default RoleBadge;
