import React from 'react';
import { Box, Chip, Typography } from '@mui/material';
import EffectivePermissionsList from '../roles/EffectivePermissionsList';

/**
 * A user's effective access, split by scope: the global permissions (which
 * cover every Workspace) and, per Workspace, what Workspace-scoped roles
 * grant there, with each role and where it comes from (direct or via a
 * team). `access` is GET /rbac/users/:id/effective.
 */
const roleSources = (ws) => {
  // Each role once, with the bindings that hold it (direct / via team).
  const byRole = new Map();
  Object.values(ws.sources || {}).forEach((list) =>
    (list || []).forEach((src) => {
      const key = `${src.role_id}:${src.via}:${src.group_id || ''}`;
      if (!byRole.has(key)) byRole.set(key, src);
    })
  );
  if (byRole.size === 0) {
    return (ws.roles || []).map((r) => ({ key: String(r.id), label: r.name }));
  }
  return [...byRole.entries()].map(([key, src]) => {
    const role = src.role_name || `Role #${src.role_id}`;
    const team = src.group_name || (src.group_id ? `#${src.group_id}` : '');
    return { key, label: src.via === 'group' ? `${role} via team ${team}` : `${role} (direct)` };
  });
};

const WorkspaceEffectivePermissions = ({ access }) => {
  const workspaces = access?.workspaces || [];
  return (
    <Box>
      <Typography variant="headingSmall" component="h3" sx={{ mb: 1 }}>
        Global (every Workspace)
      </Typography>
      <EffectivePermissionsList permissions={access?.permissions || []} sources={access?.sources || {}} />
      {workspaces.map((ws) => (
        <Box key={ws.id} mt={3} data-testid={`effective-workspace-${ws.id}`}>
          <Typography variant="headingSmall" component="h3" sx={{ mb: 1 }}>
            Workspace: {ws.name || `#${ws.id}`}
          </Typography>
          <Box sx={{ mb: 1, display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
            {roleSources(ws).map((r) => (
              <Chip key={r.key} size="small" label={r.label} />
            ))}
          </Box>
          <EffectivePermissionsList permissions={ws.permissions || []} sources={ws.sources || {}} />
        </Box>
      ))}
    </Box>
  );
};

export default WorkspaceEffectivePermissions;
