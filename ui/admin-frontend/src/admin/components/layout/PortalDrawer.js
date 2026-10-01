import React, { useMemo } from 'react';
import BaseDrawer from './base-drawer';
import useNavManifest, { toMenuItems } from '../../hooks/useNavManifest';

/**
 * Portal navigation, from the navigation manifest (GET /common/nav,
 * api/nav.go portalNav). "Browse" is the one place to find something to
 * build with (UX review D4): the unified catalog, then one entry per asset
 * type and per plugin resource type, each a filtered view of the same
 * catalog. Which catalog holds an asset is a filter inside those pages, not
 * a level of navigation. Portal plugin sections follow.
 */
const PortalDrawer = ({ open }) => {
  const { manifest, loading } = useNavManifest();
  const menuItems = useMemo(() => toMenuItems(manifest?.portal || []), [manifest]);

  if (loading) {
    return null;
  }

  return (
    <BaseDrawer
      id="portal"
      menuItems={menuItems}
      showToolbar={false}
      customStyles={{
        marginTop: 'var(--studio-header-height)'
      }}
      defaultOpen={open !== false}
      defaultExpandedItems={{
        'browse': true,
      }}
    />
  );
};

export default PortalDrawer;
