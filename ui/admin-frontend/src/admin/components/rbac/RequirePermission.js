import React from 'react';
import { usePermissions } from '../../context/PermissionsContext';
import { toArray } from '../../rbac/permissions';
import PermissionDeniedPanel from './PermissionDeniedPanel';

/**
 * Route-level guard: renders the page when the user holds the permission,
 * otherwise a "you don't have permission" panel instead of a blank page.
 *
 *   <RequirePermission permission={P.LLMS_READ}><LLMList /></RequirePermission>
 *   <RequirePermission anyOf={[P.A, P.B]}>...</RequirePermission>
 */
const RequirePermission = ({ permission, anyOf, children }) => {
  const { can, canAny } = usePermissions();
  const required = permission ? toArray(permission) : toArray(anyOf);
  const allowed = permission ? can(permission) : anyOf ? canAny(required) : true;
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
