import React from 'react';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import Icon from '../../../components/common/Icon';

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
        icon: <Icon name="house" />,
        path: '/chat/dashboard'
      },
      {
        id: 'chat-rooms',
        text: 'Chats',
        icon: <Icon name="message-lines" />,
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
