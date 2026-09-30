import React, { useState, useEffect, useCallback } from 'react';
import pubClient from '../utils/pubClient';
import { usePermissions } from '../context/PermissionsContext';
import Icon from '../../components/common/Icon';

/**
 * The navigation manifest (GET /common/nav, api/nav.go): the surfaces and
 * the admin, portal and chat menus the signed-in user may open. The console's
 * drawers render from it, and a host that draws Studio's navigation itself
 * builds its menu from the same response. nav.golden.json holds the full
 * menus, checked against the routes by nav.golden.test.js.
 *
 * It reloads when the user's permissions change (a role was granted or
 * revoked) and when plugin UIs are installed or removed. A failed reload
 * keeps the manifest already shown; with none yet, the menus are empty.
 */
const EMPTY = { surfaces: [], admin: [], portal: [], chat: [] };

const useNavManifest = () => {
  const { permissions } = usePermissions();
  const [manifest, setManifest] = useState(null);

  const load = useCallback(async () => {
    try {
      const response = await pubClient.get('/common/nav');
      setManifest({ ...EMPTY, ...(response.data || {}) });
    } catch (error) {
      console.error('Failed to load the navigation manifest:', error);
      setManifest((current) => current || EMPTY);
    }
  }, []);

  // A string key, so a new Set with the same contents does not reload.
  const permissionKey = [...(permissions || [])].sort().join(',');
  useEffect(() => {
    load();
  }, [load, permissionKey]);

  useEffect(() => {
    window.addEventListener('plugin-loader-refreshed', load);
    window.addEventListener('portal-plugin-loader-refreshed', load);
    return () => {
      window.removeEventListener('plugin-loader-refreshed', load);
      window.removeEventListener('portal-plugin-loader-refreshed', load);
    };
  }, [load]);

  return { manifest, loading: manifest === null, reload: load };
};

// toMenuItems turns manifest entries into BaseDrawer menu items.
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

export default useNavManifest;
