import React from 'react';
import { Chip, Tooltip, Typography } from '@mui/material';

/**
 * Small chip for a role: system roles get a distinct look so it is obvious
 * they cannot be edited. A role held through a team (`via: "group"`) is
 * always outlined and names the team, so the list reads as effective roles
 * without hiding where each one comes from.
 */
const RoleBadge = ({ role, size = 'small', sx = {} }) => {
  if (!role) return null;
  const name = role.name ?? role.attributes?.name ?? '';
  const isSystem = role.is_system ?? role.attributes?.is_system ?? false;
  const via = role.via ?? role.attributes?.via ?? 'direct';
  const viaTeam = via === 'group';
  const teamName = role.group_name ?? role.attributes?.group_name ?? '';
  const viaLabel = viaTeam ? `via team ${teamName || role.group_id || ''}`.trim() : '';
  const chip = (
    <Chip
      label={
        viaTeam ? (
          <>
            {name}
            <Typography component="span" variant="inherit" sx={{ opacity: 0.7, ml: 0.5 }}>
              {viaLabel}
            </Typography>
          </>
        ) : (
          name
        )
      }
      size={size}
      variant={isSystem && !viaTeam ? 'filled' : 'outlined'}
      color={isSystem ? 'primary' : 'default'}
      sx={{ mr: 0.5, mb: 0.5, ...sx }}
      data-testid={`role-badge-${role.slug ?? role.attributes?.slug ?? name}`}
      data-via={via}
    />
  );
  if (!viaTeam) return chip;
  return <Tooltip title={`Inherited from the team ${teamName || role.group_id || ''}`.trim()}>{chip}</Tooltip>;
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
