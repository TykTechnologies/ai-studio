import React from 'react';
import {
  Dashboard,
  Person,
  People,
  SmartToy,
  DataObject,
  Web,
  SettingsInputComponent,
} from '@mui/icons-material';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';

const Drawer = () => {
  const { features, loading } = useSystemFeatures();

  if (loading) {
    return null;
  }

  const getMenuItems = () => [
    { 
      id: 'dashboard',
      text: 'Dashboard', 
      icon: <Dashboard />, 
      path: '/admin/dash' 
    },
    {
      id: 'team',
      text: 'Team',
      icon: <People />,
      subItems: [
        { 
          id: 'users',
          text: 'Users', 
          path: '/admin/users' 
        },
        ...(!features.feature_gateway ||
        features.feature_portal ||
        features.feature_chat
          ? [{ 
              id: 'groups',
              text: 'Groups', 
              path: '/admin/groups' 
            }]
          : []),
      ],
    },
    {
      text: 'AI',
      icon: <SmartToy />,
      subItems: [
        { text: 'LLMs', path: '/admin/llms' },
        ...(features.feature_chat
          ? [
              {
                text: 'Call Settings',
                path: '/admin/llm-settings',
              },
            ]
          : []),
        {
          text: 'Model Prices',
          path: '/admin/model-prices',
        },
      ],
    },
    {
      text: 'Data',
      icon: <DataObject />,
      subItems: [
        {
          text: 'Vector Sources',
          path: '/admin/datasources',
        },
        ...(features.feature_chat
          ? [{ text: 'Tools', path: '/admin/tools' }]
          : []),
      ],
    },
    ...(features.feature_gateway
      ? [
          {
            text: 'Gateway',
            icon: <SettingsInputComponent />,
            subItems: [
              {
                text: 'Filters',
                path: '/admin/filters',
              },
              { text: 'Secrets', path: '/admin/secrets' },
            ],
          },
        ]
      : []),
    ...(features.feature_gateway &&
    !features.feature_portal &&
    !features.feature_chat
      ? [
          {
            text: 'Apps and Credentials',
            icon: <Web />,
            subItems: [
              { text: 'Apps', path: '/admin/apps' },
            ],
          },
        ]
      : [
          {
            text: 'Portal',
            icon: <Web />,
            subItems: [
              ...(features.feature_portal || features.feature_gateway
                ? [{ text: 'Apps', path: '/admin/apps' }]
                : []),
              ...(features.feature_chat
                ? [
                    {
                      text: 'Chat Rooms',
                      path: '/admin/chats',
                    },
                  ]
                : []),
              ...(features.feature_portal || features.feature_chat
                ? [
                    {
                      text: 'Catalogs',
                      subItems: [
                        ...(features.feature_portal
                          ? [
                              {
                                text: 'LLMs',
                                path: '/admin/catalogs/llms',
                              },
                            ]
                          : []),
                        {
                          text: 'Data',
                          path: '/admin/catalogs/data',
                        },
                        ...(features.feature_chat
                          ? [
                              {
                                text: 'Tools',
                                path: '/admin/catalogs/tools',
                              },
                            ]
                          : []),
                      ],
                    },
                  ]
                : []),
            ],
          },
        ]),
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
