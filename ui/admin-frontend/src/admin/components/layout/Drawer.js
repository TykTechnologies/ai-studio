import React from 'react';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import Icon from '../../../components/common/Icon';

const Drawer = () => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const { uiOptions, loading: entitlementsLoading } = useUserEntitlements();

  if (featuresLoading || entitlementsLoading) {
    return null;
  }

  const getMenuItems = () => [
    {
      id: 'dashboard',
      text: 'Analytics',
      icon: <Icon name="monitor-waveform" />,
      path: '/admin/dash'
    },
    {
      id: 'llm-management',
      text: 'LLM management',
      icon: <Icon name="microchip-ai" />,
      subItems: [
        { id: 'llms', text: 'LLM providers', path: '/admin/llms' },
        { id: 'model-prices', text: 'Model prices', path: '/admin/model-prices' },
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
        ...(!features.feature_gateway ||
        features.feature_portal ||
        features.feature_chat
          ? [{ id: 'groups', text: 'User groups', path: '/admin/groups' }]
          : []),
        ...(uiOptions?.show_sso_config
          ? [{ id: 'sso-profiles', text: 'SSO profiles', path: '/admin/sso-profiles' }]
          : []),
        { id: 'filters', text: 'Filters & Middleware', path: '/admin/filters' },
        { id: 'secrets', text: 'Secrets', path: '/admin/secrets' },
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
            subItems: [{ id: 'portal-apps', text: 'Apps', path: '/admin/apps' }],
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
              { id: 'llm-settings', text: 'Model call settings', path: '/admin/llm-settings' },
            ],
          },
        ]
      : []),
    ...((features.feature_portal || features.feature_chat)
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
