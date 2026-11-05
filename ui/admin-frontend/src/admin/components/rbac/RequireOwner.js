import React from 'react';
import { usePermissions } from '../../context/PermissionsContext';
import PermissionDeniedPanel from './PermissionDeniedPanel';

/**
 * Route-level guard for pages only an Owner may open (the License page).
 * Ownership is a role, not a permission, so RequirePermission cannot say it.
 */
const RequireOwner = ({ children }) => {
  const { isOwner } = usePermissions();
  if (!isOwner) {
    return (
      <PermissionDeniedPanel
        title="Only an Owner can view this page"
        description="The licence is managed by the installation's Owners. Ask an Owner if you need its details."
      />
    );
  }
  return children;
};

export default RequireOwner;
