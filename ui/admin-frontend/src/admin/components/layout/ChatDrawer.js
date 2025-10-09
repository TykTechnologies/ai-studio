import React, { useState, useEffect } from 'react';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import Icon from '../../../components/common/Icon';
import agentService from '../../../portal/services/agentService';

const ChatDrawer = () => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const {
    userEntitlements,
    uiOptions,
    loading: entitlementsLoading
  } = useUserEntitlements();

  const [agents, setAgents] = useState([]);

  useEffect(() => {
    // Fetch accessible agents
    const fetchAgents = async () => {
      try {
        const agentsData = await agentService.listAccessibleAgents();
        // Filter to only active agents
        const activeAgents = agentsData.filter(agent => agent.isActive);
        setAgents(activeAgents);
      } catch (err) {
        console.error('Error fetching agents for drawer:', err);
        setAgents([]);
      }
    };

    if (!featuresLoading && !entitlementsLoading) {
      fetchAgents();
    }
  }, [featuresLoading, entitlementsLoading]);

  if (featuresLoading || entitlementsLoading) {
    return null;
  }

  const getMenuItems = () => {
    if (!features.feature_chat || !uiOptions?.show_chat) {
      return [];
    }

    const menuItems = [
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

    // Add agents section if there are any active agents
    if (agents.length > 0) {
      menuItems.push({
        id: 'agents',
        text: 'Agents',
        icon: <Icon name="microchip-ai" />,
        subItems: agents
          .sort((a, b) => a.name.localeCompare(b.name))
          .map((agent) => ({
            id: `agent-${agent.id}`,
            text: agent.name,
            path: `/chat/agent/${agent.id}`
          }))
      });
    }

    return menuItems;
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
        'chat-rooms': true,
        'agents': true
      }}
    />
  );
};

export default ChatDrawer;
