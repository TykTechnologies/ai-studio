import React from 'react';
import { usePermissions } from '../../context/PermissionsContext';
import { toArray } from '../../rbac/permissions';
import PermissionDeniedPanel from './PermissionDeniedPanel';

/**
 * Route-level guard: renders the page when the user holds the permission
 * (globally, or in any Workspace for a Workspace resource), otherwise a
 * "you don't have permission" panel instead of a blank page.
 *
 *   <RequirePermission permission={P.LLMS_READ}><LLMList /></RequirePermission>
 *   <RequirePermission anyOf={[P.A, P.B]}>...</RequirePermission>
 *   <RequirePermission permission={{ test: (access) => ..., label: 'plugins:read' }}>...</RequirePermission>
 *
 * The object form is for pages whose permission is only known at runtime
 * (plugin configuration: the platform grant or the grant on that plugin).
 */
const RequirePermission = ({ permission, anyOf, children }) => {
  const access = usePermissions();
  // A route is a capability gate, as on the server: a permission on a
  // Workspace resource passes when held in any Workspace (the page then
  // checks each object). Workspace roles never carry global permissions,
  // so for those this is the global check. canAnywhere may be absent where
  // tests mock usePermissions.
  const can = access.canAnywhere || access.can;
  const canAny = (perms) => toArray(perms).some(can);
  let required;
  let allowed;
  if (permission && typeof permission === 'object' && typeof permission.test === 'function') {
    required = toArray(permission.label);
    allowed = permission.test(access);
  } else {
    required = permission ? toArray(permission) : toArray(anyOf);
    allowed = permission ? can(permission) : anyOf ? canAny(required) : true;
  }
  if (!allowed) {
    return <PermissionDeniedPanel required={required} />;
  }
  return children;
};

/** Wraps a route element in RequirePermission when a permission is given. */
export const withPermission = (element, permission) => {
  if (!permission) return element;
  return <RequirePermission permission={permission}>{element}</RequirePermission>;
};

export default RequirePermission;
