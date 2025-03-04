import pubClient from '../../../../admin/utils/pubClient';
import { parseServerMessage } from '../utils/chatMessageUtils';
import { reorderAndMergeToolMessages } from '../utils/toolMessageProcessor';

/**
 * Fetches chat history for a given session
 * @param {string} sessionId - The session ID
 * @returns {Promise<Array>} - The chat history messages
 */
export const fetchChatHistory = async (sessionId) => {
  try {
    const response = await pubClient.get(`/common/sessions/${sessionId}/messages?limit=100`);
    if (!response.data || !Array.isArray(response.data)) {
      return [];
    }

    const historicalMessages = response.data
      .map(parseServerMessage)
      .filter(msg => msg !== null);

    const reorderedMessages = reorderAndMergeToolMessages(historicalMessages);

    return reorderedMessages;
  } catch (error) {
    console.error("Error fetching chat history:", error);
    return [{
      id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
      type: "system",
      content: ":::system Error: Failed to load chat history:::",
      isComplete: true,
    }];
  }
};

/**
 * Formats chat history messages for the server
 * @param {Array} messages - The chat history messages
 * @returns {string} - JSON string of formatted messages
 */
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

/**
 * Sends a message to the chat
 * @param {string} chatId - The chat ID
 * @param {string} sessionId - The session ID
 * @param {Object} message - The message to send
 * @returns {Promise<boolean>} - Whether the message was sent successfully
 */
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
