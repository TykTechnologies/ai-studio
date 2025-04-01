import React from 'react';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import Icon from '../../../components/common/Icon';

const Drawer = () => {
  const { features, loading } = useSystemFeatures();

  if (loading) {
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
      text: 'LLM management',
      icon: <Icon name="microchip-ai" />,
      subItems: [
        { text: 'LLM providers', path: '/admin/llms' },
        { text: 'Model prices', path: '/admin/model-prices' },
      ],
    },
    {
      text: 'Context management',
      icon: <Icon name="layer-group" />,
      subItems: [
        { text: 'Data sources', path: '/admin/datasources' },
        ...(features.feature_chat
          ? [{ text: 'Tools', path: '/admin/tools' }]
          : []),
      ],
    },
    {
      text: 'Governance',
      icon: <Icon name="shield" />,
      subItems: [
        { id: 'users', text: 'Users', path: '/admin/users' },
        ...(!features.feature_gateway ||
        features.feature_portal ||
        features.feature_chat
          ? [{ id: 'groups', text: 'User groups', path: '/admin/groups' }]
          : []),
        { text: 'Filters & Middleware', path: '/admin/filters' },
        { text: 'Secrets', path: '/admin/secrets' },
      ],
    },
    ...(features.feature_gateway &&
    !features.feature_portal &&
    !features.feature_chat
      ? [
          {
            text: 'Apps & credentials',
            icon: <Icon name="grid-2-plus" />,
            subItems: [{ text: 'Apps', path: '/admin/apps' }],
          },
        ]
      : []),
    ...(features.feature_portal
      ? [
          {
            text: 'AI Portal',
            icon: <Icon name="display" />,
            subItems: [{ text: 'Apps', path: '/admin/apps' }],
          },
        ]
      : []),
    ...(features.feature_chat
      ? [
          {
            text: 'Chat',
            icon: <Icon name="message-lines" />,
            subItems: [
              { text: 'Chats', path: '/admin/chats' },
              { text: 'Model call settings', path: '/admin/llm-settings' },
            ],
          },
        ]
      : []),
    ...((features.feature_portal || features.feature_chat)
      ? [
          {
            text: 'Catalogs',
            icon: <Icon name="rectangle-history" />,
            subItems: [
              ...(features.feature_portal
                ? [{ text: 'LLM providers', path: '/admin/catalogs/llms' }]
                : []),
              { text: 'Data sources', path: '/admin/catalogs/data' },
              ...(features.feature_chat
                ? [{ text: 'Tools', path: '/admin/catalogs/tools' }]
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
