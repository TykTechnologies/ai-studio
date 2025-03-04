import pubClient from '../../../../admin/utils/pubClient';
import { parseServerMessage, generateTempId } from '../utils/chatMessageUtils';
import { reorderAndMergeToolMessages } from '../utils/toolMessageProcessor';

export const fetchChatHistory = async (sessionId) => {
  try {
    const response = await pubClient.get(`/common/sessions/${sessionId}/messages?limit=100`);
    if (!response.data || !Array.isArray(response.data)) {
      return [];
    }

    const historicalMessages = response.data
      .map(parseServerMessage)
      .filter(msg => msg !== null);

    // Get tools from userEntitlements, matching the original implementation
    const userEntitlements = JSON.parse(localStorage.getItem("userEntitlements") || "{}");
    const tools = userEntitlements?.data?.tool_catalogues?.[0]?.attributes?.tools || [];

    const reorderedMessages = reorderAndMergeToolMessages(historicalMessages, tools);

    return reorderedMessages;
  } catch (error) {
    console.error("Error fetching chat history:", error);
    return [{
      id: generateTempId(),
      type: "system",
      content: "Error: Failed to load chat history",
      isComplete: true,
    }];
  }
};

export const formatHistoryForServer = (messages) => {
  return JSON.stringify(messages.map(msg => ({
    id: msg.id,
    attributes: {
      content: JSON.stringify({
        role: msg.type === 'user' ? 'human' : msg.type === 'ai' ? 'ai' : 'system',
        text: msg.content
      })
    }
  })));
};

export const sendChatMessage = async (chatId, sessionId, message) => {
  if (!sessionId) {
    console.warn("Cannot send message: sessionId is null");
    return false;
  }

  try {
    await pubClient.post(`/common/chat/${chatId}/messages?session_id=${sessionId}`, message);
    return true;
  } catch (error) {
    console.error('Error sending message:', error);
    return false;
  }
};
