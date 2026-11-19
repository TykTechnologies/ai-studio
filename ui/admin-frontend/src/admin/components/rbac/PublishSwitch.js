import React from 'react';
import { Checkbox, FormControlLabel, Switch, Tooltip } from '@mui/material';
import { usePermissions } from '../../context/PermissionsContext';
import { permissionLabel } from '../../rbac/permissions';

/**
 * A live switch (Active / Enabled) gated by the resource's publish
 * permission. Without it the control is disabled and explains why; the API
 * rejects the change as well, so this is a courtesy, not the enforcement.
 *
 *   <PublishSwitch permission={P.LLMS_PUBLISH} checked={llm.active} onChange={...} name="active" label="Active" />
 */
const PublishSwitch = ({ permission, label, checked, onChange, name, disabled = false, control = 'switch', workspaceId, ...rest }) => {
  const { can, canIn, canAnywhere } = usePermissions();
  // workspaceId: the object's Workspace (publish there); null for an object
  // being created in a Workspace not chosen yet (publish anywhere). Omitted:
  // the global permission, as before.
  let allowed = !permission;
  if (permission) {
    if (workspaceId === undefined) allowed = can(permission);
    else if (workspaceId === null || workspaceId === '') allowed = canAnywhere ? canAnywhere(permission) : can(permission);
    else allowed = canIn ? canIn(workspaceId, permission) : can(permission);
  }
  const Control = control === 'checkbox' ? Checkbox : Switch;
  const element = (
    <FormControlLabel
      control={
        <Control
          checked={Boolean(checked)}
          onChange={onChange}
          name={name}
          color="primary"
          disabled={disabled || !allowed}
          inputProps={{ 'aria-label': label, 'data-publish-gated': allowed ? undefined : 'true' }}
          {...rest}
        />
      }
      label={label}
    />
  );
  if (allowed) return element;
  return (
    <Tooltip title={`Requires the "${permissionLabel(permission)}" permission`}>
      <span data-testid="publish-switch-locked">{element}</span>
    </Tooltip>
  );
};

export default PublishSwitch;
