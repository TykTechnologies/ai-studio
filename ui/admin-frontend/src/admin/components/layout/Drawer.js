import React, { useState, useEffect, useCallback } from 'react';
import BaseDrawer from './base-drawer';
import useAdminData from '../../hooks/useAdminData';
import Icon from '../../../components/common/Icon';
import pluginLoaderService from '../../services/pluginLoaderService';
import { usePermissions } from '../../context/PermissionsContext';
import { P, toArray } from '../../rbac/permissions';

const Drawer = () => {
  const { features, uiOptions, config, loading, error } = useAdminData();
  const { canAny } = usePermissions();
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

  // Items carry the permission that unlocks them; groups with nothing left
  // to show are dropped by BaseDrawer.
  const isItemAllowed = useCallback(
    (item) => !item.permission || canAny(toArray(item.permission)),
    [canAny]
  );

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
      path: '/admin/dash',
      permission: P.ANALYTICS_READ,
    },
    {
      id: 'plugins',
      text: 'Plugins',
      icon: <Icon name="screwdriver-wrench" />,
      subItems: [
        { id: 'marketplace', text: 'Marketplace', path: '/admin/marketplace', permission: P.MARKETPLACE_READ },
        { id: 'plugin-list', text: 'Installed Plugins', path: '/admin/plugins', permission: P.PLUGINS_READ },
        ...(config?.is_enterprise
          ? [{ id: 'marketplace-settings', text: 'Marketplace Sources', path: '/admin/marketplace-settings', permission: P.MARKETPLACE_WRITE }]
          : []),
      ],
    },
    {
      id: 'llm-management',
      text: 'LLM management',
      icon: <Icon name="microchip-ai" />,
      subItems: [
        { id: 'llms', text: 'LLM providers', path: '/admin/llms', permission: P.LLMS_READ },
        { id: 'model-prices', text: 'Model prices', path: '/admin/model-prices', permission: P.MODEL_PRICES_READ },
        ...(features.feature_model_router
          ? [{ id: 'model-routers', text: 'Model Routers', path: '/admin/model-routers', permission: P.MODEL_ROUTERS_READ }]
          : []),
      ],
    },
    {
      id: 'context-management',
      text: 'Context management',
      icon: <Icon name="layer-group" />,
      subItems: [
        { id: 'datasources', text: 'Data sources', path: '/admin/datasources', permission: P.DATASOURCES_READ },
        ...(features.feature_chat
          ? [{ id: 'tools', text: 'Tools', path: '/admin/tools', permission: P.TOOLS_READ }]
          : []),
        ...(config?.is_enterprise
          ? [{ id: 'filters', text: 'Filters', path: '/admin/filters', permission: P.FILTERS_READ }]
          : []),
      ],
    },
    ...(features.feature_portal
      ? [
          {
            id: 'community',
            text: 'Community',
            icon: <Icon name="puzzle-piece" />,
            subItems: [
              { id: 'submission-queue', text: 'Submission Queue', path: '/admin/submissions', permission: P.SUBMISSIONS_READ },
              { id: 'attestation-templates', text: 'Attestation Templates', path: '/admin/attestation-templates', permission: P.ATTESTATION_TEMPLATES_READ },
            ],
          },
        ]
      : []),
    {
      id: 'access',
      text: 'Access',
      icon: <Icon name="users" />,
      subItems: [
        { id: 'users', text: 'Users', path: '/admin/users', permission: P.USERS_READ },
        ...(features.feature_groups && (!features.feature_gateway ||
        features.feature_portal ||
        features.feature_chat)
          ? [{ id: 'groups', text: 'Teams', path: '/admin/groups', permission: P.GROUPS_READ }]
          : []),
        ...(features.feature_rbac
          ? [{ id: 'roles', text: 'Roles', path: '/admin/roles', permission: P.ROLES_READ }]
          : []),
        ...(uiOptions?.show_sso_config && config?.tibEnabled
          ? [{ id: 'sso-profiles', text: 'Identity providers', path: '/admin/sso-profiles', permission: P.SSO_PROFILES_READ }]
          : []),
      ],
    },
    // Governance only holds Enterprise pages, so the whole group is hidden in
    // the Community Edition rather than showing an empty section.
    ...(config?.is_enterprise
      ? [
          {
            id: 'governance',
            text: 'Governance',
            icon: <Icon name="shield" />,
            subItems: [
              { id: 'compliance', text: 'Compliance overview', path: '/admin/compliance', permission: P.COMPLIANCE_READ },
              { id: 'audit', text: 'Audit trail', path: '/admin/audit', permission: P.AUDIT_READ },
              { id: 'metadata-schemas', text: 'Metadata schemas', path: '/admin/metadata/schemas', permission: P.METADATA_READ },
              { id: 'metadata-vocabularies', text: 'Metadata vocabularies', path: '/admin/metadata/vocabularies', permission: P.METADATA_READ },
              { id: 'metadata-compliance', text: 'Metadata coverage', path: '/admin/metadata/compliance', permission: P.METADATA_READ },
            ],
          },
        ]
      : []),
    {
      id: 'settings',
      text: 'Settings',
      icon: <Icon name="gear" />,
      subItems: [
        { id: 'secrets', text: 'Secrets', path: '/admin/secrets', permission: P.SECRETS_READ },
        { id: 'branding', text: 'Branding', path: '/admin/branding', permission: P.BRANDING_WRITE },
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
            subItems: [{ id: 'apps', text: 'Apps', path: '/admin/apps', permission: P.APPS_READ }],
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
              { id: 'portal-apps', text: 'Apps', path: '/admin/apps', permission: P.APPS_READ },
              { id: 'edge-gateways', text: 'Edge Gateways', path: '/admin/edge-gateways', permission: P.EDGES_READ },
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
              { id: 'chats', text: 'Chats', path: '/admin/chats', permission: P.CHATS_READ },
              { id: 'agents', text: 'Agents', path: '/admin/agents', permission: P.AGENTS_READ },
              { id: 'llm-settings', text: 'Model call settings', path: '/admin/llm-settings', permission: P.LLM_SETTINGS_READ },
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
                ? [{ id: 'catalog-llms', text: 'LLM providers', path: '/admin/catalogs/llms', permission: P.CATALOGUES_READ }]
                : []),
              { id: 'catalog-data', text: 'Data sources', path: '/admin/catalogs/data', permission: P.DATA_CATALOGUES_READ },
              ...(features.feature_chat
                ? [{ id: 'catalog-tools', text: 'Tools', path: '/admin/catalogs/tools', permission: P.TOOL_CATALOGUES_READ }]
                : []),
            ],
          },
        ]
      : []),
    // Add plugin-contributed menu items. Plugin pages call plugin RPCs, so
    // they default to plugins:execute unless the manifest names a permission.
    ...pluginMenuItems.map(item => ({
      id: item.id,
      text: item.label,
      icon: <Icon name="puzzle-piece" />, // Default icon for plugins
      path: item.path,
      title: item.title,
      permission: item.required_permission || P.PLUGINS_EXECUTE,
      subItems: item.sub_items?.map(subItem => ({
        id: subItem.id,
        text: subItem.text,
        path: subItem.path,
        permission: subItem.required_permission || item.required_permission || P.PLUGINS_EXECUTE,
        // Exact-match a page whose path is a prefix of a sibling page so both
        // do not highlight on the child route.
        exact: (item.sub_items || []).some(
          other => other !== subItem && other.path && subItem.path && other.path.startsWith(`${subItem.path}/`)
        ),
      })) || []
    }))
  ];

  return (
    <BaseDrawer
      id="admin"
      menuItems={getMenuItems()}
      isCollapsible={true}
      isItemAllowed={isItemAllowed}
    />
  );
};

export default Drawer;
