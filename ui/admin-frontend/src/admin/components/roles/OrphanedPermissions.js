import React, { useMemo } from 'react';
import { Box, Chip, Typography } from '@mui/material';
import usePermissionCatalogue from '../../hooks/usePermissionCatalogue';
import { FULL_ADMIN, resourceOf } from '../../rbac/permissions';

/**
 * Lists the permissions a role holds on resources that are not in the
 * catalogue right now: grants on a plugin that is uninstalled, disabled or
 * not yet loaded. They stay on the role and apply again when the plugin
 * returns. With `onRemove` each chip can be deleted from the role.
 */
const OrphanedPermissions = ({ permissions, onRemove }) => {
  const { byKey, loading } = usePermissionCatalogue();

  const orphaned = useMemo(() => {
    const held = permissions instanceof Set ? [...permissions] : permissions || [];
    return held.filter((p) => p !== FULL_ADMIN && !byKey.has(resourceOf(p))).sort();
  }, [permissions, byKey]);

  if (loading || orphaned.length === 0) return null;

  return (
    <Box sx={{ mt: 2 }} data-testid="orphaned-permissions">
      <Typography variant="subtitle2">Not installed</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        These permissions belong to a plugin that is not installed or not loaded. They have no effect until it is
        back{onRemove ? '; remove them if the plugin is gone for good.' : '.'}
      </Typography>
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
        {orphaned.map((perm) => (
          <Chip
            key={perm}
            label={perm}
            size="small"
            variant="outlined"
            onDelete={onRemove ? () => onRemove(perm) : undefined}
          />
        ))}
      </Box>
    </Box>
  );
};

export default OrphanedPermissions;
