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
} from '@mui/material';
import { listRoles, sortRoles } from '../../services/rbacService';

/**
 * Multi-select of roles, system roles first. `value` and `onChange` deal in
 * role IDs (numbers). Loads the role list itself unless `roles` is given.
 */
const RoleSelect = ({ value = [], onChange, roles: providedRoles, label = 'Roles', helperText, disabled = false, id = 'role-select' }) => {
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

  const selected = (value || []).map(Number);
  const byId = new Map(roles.map((r) => [Number(r.id), r]));
  const system = roles.filter((r) => r.attributes.is_system);
  const custom = roles.filter((r) => !r.attributes.is_system);

  const renderItem = (role) => (
    <MenuItem key={role.id} value={Number(role.id)}>
      <Checkbox size="small" checked={selected.includes(Number(role.id))} />
      <ListItemText primary={role.attributes.name} secondary={role.attributes.description} />
    </MenuItem>
  );

  return (
    <FormControl fullWidth size="small" disabled={disabled || loading}>
      <InputLabel id={`${id}-label`}>{label}</InputLabel>
      <Select
        labelId={`${id}-label`}
        id={id}
        multiple
        value={selected}
        onChange={(e) => onChange?.(e.target.value.map(Number))}
        input={<OutlinedInput label={label} />}
        renderValue={(ids) => (
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
            {ids.map((rid) => {
              const role = byId.get(Number(rid));
              return <Chip key={rid} size="small" label={role?.attributes?.name || `#${rid}`} />;
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
