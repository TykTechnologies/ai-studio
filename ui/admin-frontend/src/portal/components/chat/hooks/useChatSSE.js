import { useEffect, useRef, useState, useCallback } from 'react';
import { generateTempId } from '../utils/chatMessageUtils';
import { fetchChatHistory, sendChatMessage } from '../services/chatHistoryService';
import { setupSSEConnection } from '../services/sseConnectionService';

export const useChatSSE = ({ chatId, onMessageReceived }) => {
  const [isConnected, setIsConnected] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [sessionId, setSessionId] = useState(null);
  const [error, setError] = useState(null);
  const eventSource = useRef(null);
  const reconnectAttempts = useRef(0);
  const isConnectedRef = useRef(false);
  const loadingTimeoutRef = useRef(null);

  const closeConnection = useCallback(() => {
    if (eventSource.current) {
      eventSource.current.close();
      eventSource.current = null;
      setIsConnected(false);
      setSessionId(null);
    }
  }, []);

  const sendMessage = useCallback(async (message) => {
    console.log("Sending message with sessionId:", sessionId);
    return sendChatMessage(chatId, sessionId, message);
  }, [chatId, sessionId]);

  useEffect(() => {
    const searchParams = new URLSearchParams(window.location.search);
    const continueId = searchParams.get("continue_id");

    const maxReconnectAttempts = 5;
    const initialReconnectDelay = 500;

    setIsLoading(true);
    const timer = setTimeout(() => {
      setupSSEConnection({
        eventSourceRef: eventSource,
        chatId,
        continueId,
        onMessageReceived,
        setIsConnected,
        setSessionId,
        setError,
        setIsLoading,
        isConnectedRef,
        reconnectAttempts,
        loadingTimeoutRef,
        fetchChatHistory,
        maxReconnectAttempts,
        initialReconnectDelay
      });

      // Set loading timeout
      loadingTimeoutRef.current = setTimeout(() => {
        if (!isConnectedRef.current) {
          setIsLoading(false);
          onMessageReceived({
            id: generateTempId(),
            type: "system",
            payload: "Connection timeout. Please try again.",
            isComplete: true
          });
        }
      }, 10000);
    }, 1000);

    return () => {
      if (loadingTimeoutRef.current) {
        clearTimeout(loadingTimeoutRef.current);
      }
      clearTimeout(timer);
      closeConnection();
      setIsLoading(false);
      setIsConnected(false);
      isConnectedRef.current = false;
    };
  }, [chatId, closeConnection, onMessageReceived]);

  return {
    isConnected,
    isLoading,
    sessionId,
    sendMessage,
    closeConnection,
    error,
    setError
  };
};
