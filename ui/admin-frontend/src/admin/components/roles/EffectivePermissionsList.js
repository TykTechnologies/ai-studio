import React, { useMemo } from 'react';
import { Box, Chip, Typography } from '@mui/material';
import usePermissionCatalogue from '../../hooks/usePermissionCatalogue';
import { FULL_ADMIN, resourceOf, actionOf } from '../../rbac/permissions';

/**
 * Renders a permission list grouped by catalogue group and resource, e.g.
 * "LLM providers: read, write". A wildcard renders as a single line.
 */
const EffectivePermissionsList = ({ permissions = [] }) => {
  const { grouped, byKey } = usePermissionCatalogue();

  const rows = useMemo(() => {
    if (permissions.includes(FULL_ADMIN)) return null;
    const byResource = new Map();
    permissions.forEach((p) => {
      const res = resourceOf(p);
      if (!byResource.has(res)) byResource.set(res, []);
      byResource.get(res).push(actionOf(p));
    });
    const out = [];
    grouped.forEach(({ group, resources }) => {
      const items = resources
        .filter((r) => byResource.has(r.key))
        .map((r) => ({ key: r.key, label: r.label, actions: byResource.get(r.key) }));
      if (items.length > 0) out.push({ group, items });
    });
    // Resources unknown to the catalogue (e.g. a plugin permission) still show.
    const known = new Set(grouped.flatMap((g) => g.resources.map((r) => r.key)));
    const other = [...byResource.entries()].filter(([k]) => !known.has(k) && !byKey.has(k));
    if (other.length > 0) {
      out.push({ group: 'Other', items: other.map(([k, actions]) => ({ key: k, label: k, actions })) });
    }
    return out;
  }, [permissions, grouped, byKey]);

  if (rows === null) {
    return <Typography>Full administrator access (all permissions).</Typography>;
  }
  if (rows.length === 0) {
    return <Typography color="text.secondary">No permissions.</Typography>;
  }
  return (
    <Box data-testid="effective-permissions">
      {rows.map(({ group, items }) => (
        <Box key={group} sx={{ mb: 2 }}>
          <Typography variant="subtitle2" sx={{ mb: 0.5 }}>{group}</Typography>
          {items.map((item) => (
            <Box key={item.key} sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5, flexWrap: 'wrap' }}>
              <Typography variant="body2" sx={{ minWidth: 180 }}>{item.label}</Typography>
              {item.actions.map((a) => (
                <Chip key={a} label={a} size="small" variant="outlined" />
              ))}
            </Box>
          ))}
        </Box>
      ))}
    </Box>
  );
};

export default EffectivePermissionsList;
