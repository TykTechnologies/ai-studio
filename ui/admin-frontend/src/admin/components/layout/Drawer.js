import React, { useState, useEffect } from 'react';
import BaseDrawer from './base-drawer';
import useAdminData from '../../hooks/useAdminData';
import Icon from '../../../components/common/Icon';
import pluginLoaderService from '../../services/pluginLoaderService';

const Drawer = () => {
  const { features, uiOptions, config, loading, error } = useAdminData();
  const [pluginMenuItems, setPluginMenuItems] = useState([]);

  useEffect(() => {
    loadPluginMenuItems();

    // Listen for plugin loader refresh events
    const handlePluginRefresh = () => {
      console.log('Drawer received plugin refresh event, reloading menu items');
      loadPluginMenuItems();
    };

    window.addEventListener('plugin-loader-refreshed', handlePluginRefresh);

    return () => {
      window.removeEventListener('plugin-loader-refreshed', handlePluginRefresh);
    };
  }, []);

  const loadPluginMenuItems = async () => {
    try {
      const menuItems = await pluginLoaderService.getSidebarMenuItems();
      setPluginMenuItems(menuItems);
    } catch (error) {
      console.error('Failed to load plugin menu items:', error);
    }
  };

  if (loading || error) {
    return null;
  }

  const getMenuItems = () => [
    {
      id: 'overview',
      text: 'Overview',
      icon: <Icon name="house" />,
      path: '/admin',
      exact: true
    },
    {
      id: 'dashboard',
      text: 'Analytics',
      icon: <Icon name="monitor-waveform" />,
      path: '/admin/dash'
    },
    {
      id: 'plugins',
      text: 'Plugins',
      icon: <Icon name="screwdriver-wrench" />,
      subItems: [
        { id: 'marketplace', text: 'Marketplace', path: '/admin/marketplace' },
        { id: 'plugin-list', text: 'Installed Plugins', path: '/admin/plugins' },
        ...(config?.is_enterprise
          ? [{ id: 'marketplace-settings', text: 'Marketplace Sources', path: '/admin/marketplace-settings' }]
          : []),
      ],
    },
    {
      id: 'llm-management',
      text: 'LLM management',
      icon: <Icon name="microchip-ai" />,
      subItems: [
        { id: 'llms', text: 'LLM providers', path: '/admin/llms' },
        { id: 'model-prices', text: 'Model prices', path: '/admin/model-prices' },
        ...(features.feature_model_router
          ? [{ id: 'model-routers', text: 'Model Routers', path: '/admin/model-routers' }]
          : []),
      ],
    },
    {
      id: 'context-management',
      text: 'Context management',
      icon: <Icon name="layer-group" />,
      subItems: [
        { id: 'datasources', text: 'Data sources', path: '/admin/datasources' },
        ...(features.feature_chat
          ? [{ id: 'tools', text: 'Tools', path: '/admin/tools' }]
          : []),
      ],
    },
    {
      id: 'Governance',
      text: 'Governance',
      icon: <Icon name="shield" />,
      subItems: [
        { id: 'users', text: 'Users', path: '/admin/users' },
        ...(features.feature_groups && (!features.feature_gateway ||
        features.feature_portal ||
        features.feature_chat)
          ? [{ id: 'groups', text: 'Teams', path: '/admin/groups' }]
          : []),
        ...(uiOptions?.show_sso_config && config?.tibEnabled
          ? [{ id: 'sso-profiles', text: 'Identity providers', path: '/admin/sso-profiles' }]
          : []),
        ...(config?.is_enterprise
          ? [{ id: 'filters', text: 'Filters', path: '/admin/filters' }]
          : []),
        { id: 'secrets', text: 'Secrets', path: '/admin/secrets' },
        { id: 'branding', text: 'Branding', path: '/admin/branding' },
      ],
    },
    ...(features.feature_gateway &&
    !features.feature_portal &&
    !features.feature_chat
      ? [
          {
            id: 'apps-credentials',
            text: 'Apps & credentials',
            icon: <Icon name="grid-2-plus" />,
            subItems: [{ id: 'apps', text: 'Apps', path: '/admin/apps' }],
          },
        ]
      : []),
    ...(features.feature_portal
      ? [
          {
            id: 'ai-portal',
            text: 'AI Portal',
            icon: <Icon name="display" />,
            subItems: [
              { id: 'portal-apps', text: 'Apps', path: '/admin/apps' },
              { id: 'edge-gateways', text: 'Edge Gateways', path: '/admin/edge-gateways' },
            ],
          },
        ]
      : []),
    ...(features.feature_chat
      ? [
          {
            id: 'chat',
            text: 'Chat',
            icon: <Icon name="message-lines" />,
            subItems: [
              { id: 'chats', text: 'Chats', path: '/admin/chats' },
              { id: 'agents', text: 'Agents', path: '/admin/agents' },
              { id: 'llm-settings', text: 'Model call settings', path: '/admin/llm-settings' },
            ],
          },
        ]
      : []),
    ...(features.feature_groups && (features.feature_portal || features.feature_chat)
      ? [
          {
            id: 'catalogs',
            text: 'Catalogs',
            icon: <Icon name="rectangle-history" />,
            subItems: [
              ...(features.feature_portal
                ? [{ id: 'catalog-llms', text: 'LLM providers', path: '/admin/catalogs/llms' }]
                : []),
              { id: 'catalog-data', text: 'Data sources', path: '/admin/catalogs/data' },
              ...(features.feature_chat
                ? [{ id: 'catalog-tools', text: 'Tools', path: '/admin/catalogs/tools' }]
                : []),
            ],
          },
        ]
      : []),
    // Add plugin-contributed menu items
    ...pluginMenuItems.map(item => ({
      id: item.id,
      text: item.label,
      icon: <Icon name="puzzle-piece" />, // Default icon for plugins
      path: item.path,
      title: item.title,
      subItems: item.sub_items?.map(subItem => ({
        id: subItem.id,
        text: subItem.text,
        path: subItem.path
      })) || []
    }))
  ];

  return (
    <BaseDrawer
      id="admin"
      menuItems={getMenuItems()}
      isCollapsible={true}
    />
  );
};

export default Drawer;
