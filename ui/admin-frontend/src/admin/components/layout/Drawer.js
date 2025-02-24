import React from 'react';
import { SvgIcon } from '@mui/material';
import { ReactComponent as MonitorWaveformIcon } from '../../../common/fontawesome/monitor-waveform.svg';
import { ReactComponent as MicrochipAiIcon } from '../../../common/fontawesome/microchip-ai.svg';
import { ReactComponent as LayerGroupIcon } from '../../../common/fontawesome/layer-group.svg';
import { ReactComponent as ShieldIcon } from '../../../common/fontawesome/shield.svg';
import { ReactComponent as DisplayIcon } from '../../../common/fontawesome/display.svg';
import { ReactComponent as RectangleHistoryIcon } from '../../../common/fontawesome/rectangle-history.svg';
import { ReactComponent as MessageLinesIcon } from '../../../common/fontawesome/message-lines.svg';
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
      text: 'Analytics',
      icon: <SvgIcon component={MonitorWaveformIcon} inheritViewBox />,
      path: '/admin/dash'
    },
    {
      text: 'LLM management',
      icon: <SvgIcon component={MicrochipAiIcon} inheritViewBox />,
      subItems: [
        { text: 'LLM providers', path: '/admin/llms' },
        { text: 'Model prices', path: '/admin/model-prices' },
      ],
    },
    {
      text: 'Context management',
      icon: <SvgIcon component={LayerGroupIcon} inheritViewBox />,
      subItems: [
        { text: 'Data sources', path: '/admin/datasources' },
        ...(features.feature_chat
          ? [{ text: 'Tools', path: '/admin/tools' }]
          : []),
      ],
    },
    {
      text: 'Governance',
      icon: <SvgIcon component={ShieldIcon} inheritViewBox />,
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
            icon: <SvgIcon component={DisplayIcon} inheritViewBox />,
            subItems: [{ text: 'Apps', path: '/admin/apps' }],
          },
        ]
      : []),
    ...(features.feature_portal
      ? [
          {
            text: 'Portal',
            icon: <SvgIcon component={DisplayIcon} inheritViewBox />,
            subItems: [{ text: 'Apps', path: '/admin/apps' }],
          },
        ]
      : []),
    ...(features.feature_chat
      ? [
          {
            text: 'Chat',
            icon: <SvgIcon component={MessageLinesIcon} inheritViewBox />,
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
            icon: <SvgIcon component={RectangleHistoryIcon} inheritViewBox />,
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
