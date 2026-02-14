import React, { useState, useEffect } from 'react';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import Icon from '../../../components/common/Icon';
import portalPluginLoaderService from '../../../portal/services/portalPluginLoaderService';

const PortalDrawer = ({ catalogues, dataCatalogues, toolCatalogues, open }) => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const {
    userEntitlements,
    uiOptions,
    loading: entitlementsLoading
  } = useUserEntitlements();
  const [pluginMenuItems, setPluginMenuItems] = useState([]);

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
      {
        id: 'catalogs',
        text: 'Catalogs',
        icon: <Icon name="rectangle-history" />,
        subItems: [
          {
            id: 'llms',
            text: 'LLM providers',
            subItems: userEntitlements?.catalogues?.map(catalogue => ({
              id: `llm-${catalogue.id}`,
              text: catalogue.attributes.name,
              path: `/portal/llms/${catalogue.id}`
            })) || []
          },
          {
            id: 'data-sources',
            text: 'Data sources',
            subItems: userEntitlements?.data_catalogues?.map(catalogue => ({
              id: `db-${catalogue.id}`,
              text: catalogue.attributes.name,
              path: `/portal/databases/${catalogue.id}`
            })) || []
          },
          {
            id: 'tools',
            text: 'Tools',
            subItems: userEntitlements?.tool_catalogues?.map(catalogue => ({
              id: `tool-${catalogue.id}`,
              text: catalogue.attributes.name,
              path: `/portal/tools/${catalogue.id}`
            })) || []
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
      defaultExpandedItems={{
        'resources': true,
        'llms': false,
        'databases': false
      }}
    />
  );
};

export default PortalDrawer;
