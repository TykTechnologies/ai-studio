import React from 'react';
import {
  Chat,
} from '@mui/icons-material';
import { SvgIcon } from '@mui/material';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import { ReactComponent as HouseIcon } from '../../../common/fontawesome/house.svg';

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
        id: 'dashboard',
        text: 'Overview',
        icon: <SvgIcon component={HouseIcon} inheritViewBox />,
        path: '/chat/dashboard'
      },
      {
        id: 'chat-rooms',
        text: 'Chats',
        icon: <Chat />,
        subItems: userEntitlements?.chats?.map((chat) => ({
          id: `chat-${chat.id}`,
          text: chat.attributes.name,
          path: `/chat/${chat.id}`
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
