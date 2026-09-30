import React, { useEffect, useState } from 'react';
import {
  Box,
  Checkbox,
  Chip,
  FormControl,
  FormHelperText,
  InputLabel,
  ListItemText,
  ListSubheader,
  MenuItem,
  OutlinedInput,
  Select,
  Tooltip,
} from '@mui/material';
import { Lock as LockIcon } from '@mui/icons-material';
import { listRoles, sortRoles } from '../../services/rbacService';

/**
 * Multi-select of roles, system roles first. `value` and `onChange` deal in
 * role IDs (numbers). Loads the role list itself unless `roles` is given.
 * `lockedIds` are roles that stay selected and cannot be removed here (the
 * host application assigns them).
 */
const RoleSelect = ({ value = [], onChange, roles: providedRoles, label = 'Roles', helperText, disabled = false, id = 'role-select', lockedIds = [] }) => {
  const [roles, setRoles] = useState(providedRoles || []);
  const [loading, setLoading] = useState(!providedRoles);

  useEffect(() => {
    if (providedRoles) {
      setRoles(providedRoles);
      return;
    }
    let cancelled = false;
    listRoles()
      .then((list) => {
        if (!cancelled) setRoles(sortRoles(list));
      })
      .catch((err) => console.error('Failed to load roles', err))
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [providedRoles]);

  const locked = (lockedIds || []).map(Number);
  const selected = (value || []).map(Number);
  const byId = new Map(roles.map((r) => [Number(r.id), r]));
  const system = roles.filter((r) => r.attributes.is_system);
  const custom = roles.filter((r) => !r.attributes.is_system);

  const renderItem = (role) => {
    const isLocked = locked.includes(Number(role.id));
    return (
      <MenuItem key={role.id} value={Number(role.id)} disabled={isLocked} data-testid={`role-option-${role.id}`}>
        <Checkbox size="small" checked={isLocked || selected.includes(Number(role.id))} />
        <ListItemText
          primary={role.attributes.name}
          secondary={isLocked ? 'Assigned by the host application' : role.attributes.description}
        />
      </MenuItem>
    );
  };

  return (
    <FormControl fullWidth size="small" disabled={disabled || loading}>
      <InputLabel id={`${id}-label`}>{label}</InputLabel>
      <Select
        labelId={`${id}-label`}
        id={id}
        multiple
        value={selected}
        onChange={(e) => onChange?.([...new Set([...e.target.value.map(Number), ...locked])])}
        input={<OutlinedInput label={label} />}
        renderValue={(ids) => (
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
            {ids.map((rid) => {
              const role = byId.get(Number(rid));
              const name = role?.attributes?.name || `#${rid}`;
              if (locked.includes(Number(rid))) {
                return (
                  <Tooltip key={rid} title="Assigned by the host application; change it there">
                    <Chip size="small" icon={<LockIcon fontSize="small" />} label={name} data-testid={`role-chip-locked-${rid}`} />
                  </Tooltip>
                );
              }
              return <Chip key={rid} size="small" label={name} />;
            })}
          </Box>
        )}
        inputProps={{ 'data-testid': 'role-select-input' }}
      >
        {system.length > 0 && <ListSubheader>System roles</ListSubheader>}
        {system.map(renderItem)}
        {custom.length > 0 && <ListSubheader>Custom roles</ListSubheader>}
        {custom.map(renderItem)}
      </Select>
      {helperText && <FormHelperText>{helperText}</FormHelperText>}
    </FormControl>
  );
};

export default RoleSelect;
