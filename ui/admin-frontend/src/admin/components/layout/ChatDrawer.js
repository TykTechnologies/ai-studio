import React, { useState, useEffect } from 'react';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import Icon from '../../../components/common/Icon';
import agentService from '../../../portal/services/agentService';
import pubClient from '../../utils/pubClient';

const HISTORY_CACHE_KEY = "chatHistoryDrawer";
const HISTORY_CACHE_EXPIRY = 30000; // 30s

const ChatDrawer = () => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const {
    userEntitlements,
    uiOptions,
    loading: entitlementsLoading
  } = useUserEntitlements();

  const [agents, setAgents] = useState([]);
  const [chatHistory, setChatHistory] = useState([]);

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

  useEffect(() => {
    const fetchChatHistory = async () => {
      const cachedData = localStorage.getItem(HISTORY_CACHE_KEY);
      if (cachedData) {
        const { data, timestamp } = JSON.parse(cachedData);
        if (Date.now() - timestamp < HISTORY_CACHE_EXPIRY) {
          setChatHistory(data);
          return;
        }
      }

      try {
        const response = await pubClient.get('/common/history?page_size=5&page=1');
        const records = response.data.data || [];
        setChatHistory(records);
        localStorage.setItem(
          HISTORY_CACHE_KEY,
          JSON.stringify({ data: records, timestamp: Date.now() })
        );
      } catch (error) {
        console.error("Failed to fetch chat history:", error);
      }
    };

    if (!featuresLoading && !entitlementsLoading) {
      fetchChatHistory();
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

    // Add past conversations section if there are any
    if (chatHistory.length > 0) {
      menuItems.push({
        id: 'past-conversations',
        text: 'Past Conversations',
        icon: <Icon name="rectangle-history" />,
        subItems: [
          ...chatHistory.map((record) => ({
            id: `history-${record.id}`,
            text: record.attributes.name,
            path: `/chat/${record.attributes.chat_id}?continue_id=${record.attributes.session_id}`,
            exact: true
          })),
          {
            id: 'view-all-conversations',
            text: 'View all conversations',
            path: '/chat/dashboard',
            exact: true
          }
        ]
      });
    }

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
