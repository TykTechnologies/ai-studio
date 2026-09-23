import React, { useState, useEffect } from 'react';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import Icon from '../../../components/common/Icon';
import portalPluginLoaderService from '../../../portal/services/portalPluginLoaderService';
import pubClient from '../../utils/pubClient';

/**
 * Portal navigation. "Browse" is the one place to find something to build
 * with (UX review D4): the unified catalog, then one entry per asset type
 * and per plugin resource type, each a filtered view of the same catalog.
 * Which catalog holds an asset is a filter inside those pages, not a level
 * of navigation.
 */
const PortalDrawer = ({ open }) => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const { uiOptions, loading: entitlementsLoading } = useUserEntitlements();
  const [pluginMenuItems, setPluginMenuItems] = useState([]);
  const [pluginResourceTypes, setPluginResourceTypes] = useState([]);

  // Load portal plugin sidebar items
  useEffect(() => {
    const loadPluginMenuItems = async () => {
      try {
        const menuItems = await portalPluginLoaderService.getSidebarMenuItems();
        setPluginMenuItems(menuItems);
      } catch (error) {
        console.error('Failed to load portal plugin menu items:', error);
      }
    };
    loadPluginMenuItems();

    const handlePluginRefresh = () => loadPluginMenuItems();
    window.addEventListener('portal-plugin-loader-refreshed', handlePluginRefresh);
    return () => window.removeEventListener('portal-plugin-loader-refreshed', handlePluginRefresh);
  }, []);

  // Plugin resource types with at least one instance the user can use get
  // their own Browse entry.
  useEffect(() => {
    const loadPluginResourceTypes = async () => {
      try {
        const response = await pubClient.get('/common/accessible-plugin-resources');
        const types = response.data?.data || [];
        setPluginResourceTypes(types.filter(t => (t.instances || []).length > 0));
      } catch {
        // Plugin resources not available — that's fine
      }
    };
    loadPluginResourceTypes();
  }, []);

  if (featuresLoading || entitlementsLoading) {
    return null;
  }

  const showPortalFeatures = features.feature_portal || features.feature_gateway;

  const getMenuItems = () => {
    if (!showPortalFeatures || !uiOptions?.show_portal) {
      return [];
    }

    return [
      {
        id: 'dashboard',
        text: 'Overview',
        icon: <Icon name="house" />,
        path: '/portal/dashboard'
      },
      {
        id: 'my-apps',
        text: 'Apps',
        icon: <Icon name="grid-2-plus" />,
        path: '/portal/apps'
      },
      {
        id: 'browse',
        text: 'Browse',
        icon: <Icon name="rectangle-history" />,
        subItems: [
          // Exact, so a detail page under /portal/catalog/llms/3 highlights
          // "LLM providers" (the longest match) and not "All assets".
          { id: 'browse-all', text: 'All assets', path: '/portal/catalog', exact: true },
          { id: 'browse-llms', text: 'LLM providers', path: '/portal/catalog/llms' },
          { id: 'browse-datasources', text: 'Data sources', path: '/portal/catalog/datasources' },
          { id: 'browse-tools', text: 'Tools', path: '/portal/catalog/tools' },
          ...(features.feature_model_router
            ? [{ id: 'browse-model-routers', text: 'Model routers', path: '/portal/catalog/model-routers' }]
            : []),
          ...(features.feature_semantic_router
            ? [{ id: 'browse-semantic-routers', text: 'Semantic routers', path: '/portal/catalog/semantic-routers' }]
            : []),
          ...(features.feature_tyk_mcp
            ? [{ id: 'browse-mcp-servers', text: 'MCP servers', path: '/portal/catalog/mcp-servers' }]
            : []),
          ...pluginResourceTypes.map(rt => ({
            id: `browse-resource-${rt.plugin_id}-${rt.slug}`,
            text: rt.name,
            path: `/portal/catalog/resources/${rt.plugin_id}/${rt.slug}`
          }))
        ]
      },
      {
        id: 'contributions',
        text: 'Community',
        icon: <Icon name="puzzle-piece" />,
        subItems: [
          {
            id: 'my-contributions',
            text: 'My Contributions',
            path: '/portal/contributions'
          },
          {
            id: 'submit-resource',
            text: 'Submit Resource',
            path: '/portal/submissions/new'
          }
        ]
      },
      // Portal plugin sidebar items (dynamically loaded from plugins with portal_ui capability)
      ...pluginMenuItems.map(pluginSection => ({
        id: pluginSection.id,
        text: pluginSection.label,
        icon: <Icon name="puzzle-piece" />,
        ...(pluginSection.sub_items && pluginSection.sub_items.length === 1
          ? { path: pluginSection.sub_items[0].path }
          : {
              subItems: (pluginSection.sub_items || []).map(subItem => ({
                id: subItem.id,
                text: subItem.text,
                path: subItem.path,
                // A plugin page whose path is a prefix of a sibling page
                // (e.g. /portal/plugins/x and /portal/plugins/x/mine) must
                // match exactly, otherwise both light up on the child route.
                exact: (pluginSection.sub_items || []).some(
                  other => other !== subItem && other.path && subItem.path && other.path.startsWith(`${subItem.path}/`)
                ),
              }))
            }
        )
      })),
    ];
  };

  return (
    <BaseDrawer
      id="portal"
      menuItems={getMenuItems()}
      showToolbar={false}
      customStyles={{
        marginTop: '64px'
      }}
      defaultOpen={open !== false}
      defaultExpandedItems={{
        'browse': true,
      }}
    />
  );
};

export default PortalDrawer;
