import React, { useEffect, useMemo, useState } from 'react';
import { FormControl, FormHelperText, InputLabel, MenuItem, Select } from '@mui/material';
import { usePermissions } from '../../context/PermissionsContext';
import { hasPermission } from '../../rbac/permissions';
import { listWorkspaces } from '../../services/workspacesService';

/**
 * WorkspaceSelect picks the Workspace an object is created in or moved to:
 * the Workspaces where the caller holds `permission` (a write permission,
 * e.g. P.LLMS_WRITE). A global holder may pick any Workspace (listed from
 * the API); anyone else the Workspaces their scoped roles give them it in.
 * With one choice it is preselected and disabled. `error` (a message, e.g.
 * from workspaceFieldError) marks the field and replaces the helper text.
 * Community Edition
 * (rbacEnabled false) has no Workspaces, so nothing is rendered.
 */
const WorkspaceSelect = ({
  permission,
  value,
  onChange,
  label = 'Workspace',
  helperText,
  error = '',
  disabled = false,
  name = 'workspace_id',
}) => {
  const { rbacEnabled, can, workspaces: scoped } = usePermissions();
  const global = rbacEnabled && can(permission);
  const [all, setAll] = useState(null);

  useEffect(() => {
    if (!global) return undefined;
    let live = true;
    listWorkspaces()
      .then((list) => {
        if (live) setAll(list.map((w) => ({ id: Number(w.id), name: w.attributes?.name || `Workspace ${w.id}` })));
      })
      .catch(() => {
        if (live) setAll([]);
      });
    return () => {
      live = false;
    };
  }, [global]);

  const options = useMemo(() => {
    if (!rbacEnabled) return [];
    if (global) return all || [];
    return (scoped || [])
      .filter((w) => hasPermission(w.permissions, permission))
      .map((w) => ({ id: w.id, name: w.name || `Workspace ${w.id}` }));
  }, [rbacEnabled, global, all, scoped, permission]);

  const optionIds = useMemo(() => new Set(options.map((o) => o.id)), [options]);
  const only = options.length === 1 ? options[0].id : null;
  useEffect(() => {
    if (only !== null && Number(value) !== only && onChange) onChange(only);
  }, [only, value, onChange]);

  if (!rbacEnabled) return null;

  const labelId = `${name}-label`;
  return (
    <FormControl fullWidth disabled={disabled || options.length <= 1} error={Boolean(error)}>
      <InputLabel id={labelId}>{label}</InputLabel>
      <Select
        labelId={labelId}
        label={label}
        name={name}
        data-testid="workspace-select"
        value={optionIds.has(Number(value)) ? Number(value) : ''}
        onChange={(e) => onChange && onChange(Number(e.target.value))}
      >
        {options.map((o) => (
          <MenuItem key={o.id} value={o.id}>
            {o.name}
          </MenuItem>
        ))}
      </Select>
      {(error || helperText) && <FormHelperText>{error || helperText}</FormHelperText>}
    </FormControl>
  );
};

export default WorkspaceSelect;
