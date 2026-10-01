import React, { useCallback, useMemo } from 'react';
import BaseDrawer from './base-drawer';
import useNavManifest, { toMenuItems } from '../../hooks/useNavManifest';
import { usePermissions } from '../../context/PermissionsContext';
import { toArray } from '../../rbac/permissions';

/**
 * The admin menu, from the navigation manifest (GET /common/nav): which
 * groups exist, their order, feature gates and plugin sections are decided
 * in api/nav.go, filtered by the user's permissions.
 */
const Drawer = () => {
  const { canAny } = usePermissions();
  const { manifest, loading } = useNavManifest();

  // The server has already filtered the menu; this hides an entry at once
  // when a permission is revoked, before the reload lands.
  const isItemAllowed = useCallback(
    (item) => !item.permission || canAny(toArray(item.permission)),
    [canAny]
  );

  const menuItems = useMemo(() => toMenuItems(manifest?.admin || []), [manifest]);

  if (loading) {
    return null;
  }

  return (
    <BaseDrawer
      id="admin"
      menuItems={menuItems}
      isCollapsible={true}
      isItemAllowed={isItemAllowed}
    />
  );
};

export default Drawer;
