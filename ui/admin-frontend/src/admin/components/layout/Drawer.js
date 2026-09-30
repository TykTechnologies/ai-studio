import React, { useState, useEffect, useCallback, useMemo } from 'react';
import BaseDrawer from './base-drawer';
import Icon from '../../../components/common/Icon';
import pubClient from '../../utils/pubClient';
import { usePermissions } from '../../context/PermissionsContext';
import { toArray } from '../../rbac/permissions';

/**
 * The admin menu comes from GET /common/nav (api/nav.go), the same manifest
 * a host that draws Studio's navigation itself builds its menu from: which
 * groups exist, their order, feature gates and plugin sections are decided
 * there, filtered by the user's permissions. nav.golden.json holds the full
 * menu, checked against routes.js by nav.golden.test.js.
 */
export const toMenuItems = (items = []) =>
  items.map((item) => ({
    id: item.id,
    text: item.text,
    path: item.path,
    title: item.title,
    exact: item.exact,
    permission: item.permission,
    icon: item.icon ? <Icon name={item.icon} /> : undefined,
    ...(item.items ? { subItems: toMenuItems(item.items) } : {}),
  }));

const Drawer = () => {
  const { permissions, canAny } = usePermissions();
  const [items, setItems] = useState(null);

  const loadMenu = useCallback(async () => {
    try {
      const response = await pubClient.get('/common/nav');
      setItems(response.data?.admin || []);
    } catch (error) {
      console.error('Failed to load the admin menu:', error);
      // Keep the menu already shown; with none yet, show an empty drawer.
      setItems((current) => current || []);
    }
  }, []);

  // Reload when the user's permissions change (a role was granted or
  // revoked) and when a plugin's UI is installed or removed. The key is a
  // string so a new Set with the same contents does not reload.
  const permissionKey = [...(permissions || [])].sort().join(',');
  useEffect(() => {
    loadMenu();
  }, [loadMenu, permissionKey]);

  useEffect(() => {
    window.addEventListener('plugin-loader-refreshed', loadMenu);
    return () => window.removeEventListener('plugin-loader-refreshed', loadMenu);
  }, [loadMenu]);

  // The server has already filtered the menu; this hides an entry at once
  // when a permission is revoked, before the reload lands.
  const isItemAllowed = useCallback(
    (item) => !item.permission || canAny(toArray(item.permission)),
    [canAny]
  );

  const menuItems = useMemo(() => toMenuItems(items || []), [items]);

  if (items === null) {
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
