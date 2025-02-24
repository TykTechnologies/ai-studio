import React from 'react';
import { SvgIcon } from '@mui/material';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import { ReactComponent as HouseIcon } from '../../../common/fontawesome/house.svg';
import { ReactComponent as MessageLinesIcon } from '../../../common/fontawesome/message-lines.svg';

const ChatDrawer = () => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const { 
    userEntitlements, 
    uiOptions, 
    loading: entitlementsLoading 
  } = useUserEntitlements();

  if (featuresLoading || entitlementsLoading) {
    return null;
  }

  const getMenuItems = () => {
    if (!features.feature_chat || !uiOptions?.show_chat) {
      return [];
    }

    return [
      {
        id: 'overview',
        text: 'Overview',
        icon: <SvgIcon component={HouseIcon} inheritViewBox />,
        path: '/chat/dashboard'
      },
      {
        id: 'chat-rooms',
        text: 'Chats',
        icon: <SvgIcon component={MessageLinesIcon} inheritViewBox />,
        subItems: userEntitlements?.chats?.map((chat) => ({
          id: `chat-${chat.id}`,
          text: chat.attributes.name,
          expandable: true,
          subItems: [
            ...(chat.attributes.recent_history || []).map(record => ({
              id: `conversation-${record.session_id}`,
              text: record.name,
              path: `/chat/${chat.id}?continue_id=${record.session_id}`
            })),
            {
              id: `see-all-${chat.id}`,
              text: 'See all',
              path: '/chat/dashboard'
            }
          ]
        }))
      }
    ];
  };

  return (
    <BaseDrawer
      id="chat"
      menuItems={getMenuItems()}
      showToolbar={false}
      customStyles={{
        marginTop: '64px'
      }}
      defaultExpandedItems={{
        'chat-rooms': true
      }}
    />
  );
};

export default ChatDrawer;
