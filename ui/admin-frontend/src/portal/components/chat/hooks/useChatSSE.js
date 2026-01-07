import { useEffect, useRef, useState, useCallback } from 'react';
import { useLocation } from 'react-router-dom';
import { generateTempId } from '../utils/chatMessageUtils';
import { fetchChatHistory, sendChatMessage } from '../services/chatHistoryService';
import { setupSSEConnection } from '../services/sseConnectionService';

export const useChatSSE = ({ chatId, onMessageReceived }) => {
  const location = useLocation();
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

  // Extract continue_id from location.search to use as dependency
  const searchParams = new URLSearchParams(location.search);
  const continueId = searchParams.get("continue_id");

  useEffect(() => {
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
            content: ":::system Connection timeout. Please try again.:::",
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
  }, [chatId, continueId, closeConnection, onMessageReceived]);

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
