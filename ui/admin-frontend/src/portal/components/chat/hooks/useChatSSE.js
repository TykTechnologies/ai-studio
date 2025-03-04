import { useEffect, useRef, useState, useCallback } from 'react';
import pubClient from '../../../../admin/utils/pubClient';
import { createSystemMessage, detectErrorType, generateTempId } from '../utils/chatMessageUtils';
import { processToolsAndDatasources } from '../utils/toolMessageProcessor';
import { fetchChatHistory, formatHistoryForServer, sendChatMessage } from '../services/chatHistoryService';

/**
 * Hook for managing chat communication via Server-Sent Events
 * @param {Object} options - Configuration options
 * @param {string} options.chatId - The chat ID
 * @param {Function} options.onMessageReceived - Callback for received messages
 * @returns {Object} - Chat state and control functions
 */
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

    let keepAliveInterval;
    const maxReconnectAttempts = 5;
    const initialReconnectDelay = 500;

    const setupEventSource = () => {
      const baseUrl = pubClient.defaults.baseURL;
      const token = localStorage.getItem('token');
      const params = new URLSearchParams();
      if (continueId) {
        params.append('session_id', continueId);
      }
      if (token) {
        params.append('token', token);
      }
      const url = `${baseUrl}/common/chat/${chatId}${params.toString() ? `?${params.toString()}` : ''}`;
      console.log('Setting up SSE connection to:', url);

      if (!eventSource.current || eventSource.current.readyState === EventSource.CLOSED) {
        console.log('Creating new EventSource');
        eventSource.current = new EventSource(url, {
          withCredentials: true
        });
      }

      eventSource.current.onopen = () => {
        console.log('SSE connection established');
        setIsConnected(true);
        isConnectedRef.current = true;
        reconnectAttempts.current = 0;
        setError(null);
        setIsLoading(false);

        if (loadingTimeoutRef.current) {
          clearTimeout(loadingTimeoutRef.current);
          loadingTimeoutRef.current = null;
        }
      };

      // Listen specifically for session_id events
      eventSource.current.addEventListener('session_id', (event) => {
        try {
          console.log('SSE session_id event received:', event.data);
          const data = JSON.parse(event.data);
          console.log('Processing session_id message:', data);
          const newSessionId = data.payload;
          console.log('Setting new sessionId:', newSessionId);
          setSessionId(newSessionId);
          // Update URL with new session ID
          const newUrl = `/chat/${chatId}?continue_id=${newSessionId}`;
          console.log('Updating URL to:', newUrl);
          try {
            window.history.replaceState({}, "", newUrl);
            console.log('URL updated successfully');
          } catch (err) {
            console.error('Failed to update URL:', err);
          }

          // Process tools and datasources
          const processedData = processToolsAndDatasources(data);
          // Forward the entire session_id message
          onMessageReceived(processedData);

          if (continueId) {
            setIsLoading(true);
            fetchChatHistory(continueId)
              .then((messages) => {
                if (Array.isArray(messages)) {
                  onMessageReceived({
                    type: 'history',
                    payload: formatHistoryForServer(messages)
                  });
                }
                setIsLoading(false);
              })
              .catch(() => {
                setIsLoading(false);
              });
          }
        } catch (error) {
          console.error("Error parsing SSE message:", error);
          onMessageReceived(createSystemMessage("Error: Failed to parse message from server"));
        }
      });

      // Handle stream_chunk events
      eventSource.current.addEventListener('stream_chunk', (event) => {
        try {
          console.log('SSE stream_chunk received:', event.data);
          onMessageReceived({
            type: 'stream_chunk',
            payload: event.data
          });
        } catch (error) {
          console.error("Error handling stream chunk:", error);
        }
      });

      // Handle message events
      eventSource.current.addEventListener('message', (event) => {
        try {
          console.log('SSE message received:', event.data);
          const data = JSON.parse(event.data);
          onMessageReceived(data);
        } catch (error) {
          console.error("Error parsing message:", error);
        }
      });

      // Handle system events
      eventSource.current.addEventListener('system', (event) => {
        try {
          console.log('SSE system message received:', event.data);
          onMessageReceived(createSystemMessage(event.data));
        } catch (error) {
          console.error("Error handling system message:", error);
        }
      });

      // Handle error events
      eventSource.current.addEventListener('error', (event) => {
        try {
          console.log('SSE error message received:', event.data);
          const errorType = detectErrorType(event.data);

          // For LLM config errors, don't attempt reconnection
          if (errorType === 'llm_config') {
            reconnectAttempts.current = maxReconnectAttempts; // Prevent reconnection
          }

          onMessageReceived(createSystemMessage(event.data, errorType));
        } catch (error) {
          console.error("Error handling error message:", error);
        }
      });

      // Handle any other events
      eventSource.current.onmessage = (event) => {
        try {
          console.log('SSE generic message received:', event.data);
          const data = JSON.parse(event.data);
          onMessageReceived(data);
        } catch (error) {
          console.error("Error parsing generic message:", error);
          onMessageReceived(createSystemMessage("Error: Failed to parse message from server"));
        }
      };

      eventSource.current.onerror = (error) => {
        console.error("SSE error:", error);
        setIsConnected(false);
        isConnectedRef.current = false;
        setIsLoading(false);

        // Check if we have a recent LLM config error
        const hasLLMError = error?.data && detectErrorType(error.data) === 'llm_config';

        if (!hasLLMError && reconnectAttempts.current < maxReconnectAttempts) {
          onMessageReceived({
            id: generateTempId(),
            type: "system",
            payload: `Connection lost. Attempting to reconnect... (Attempt ${reconnectAttempts.current + 1}/${maxReconnectAttempts})`,
            errorType: 'connection'
          });

          const delay = initialReconnectDelay * Math.pow(2, reconnectAttempts.current);
          console.log(`Attempting to reconnect in ${delay / 1000} seconds... (Attempt ${reconnectAttempts.current + 1}/${maxReconnectAttempts})`);

          setTimeout(() => {
            reconnectAttempts.current++;
            console.log("Reconnecting SSE...");

            if (eventSource.current) {
              eventSource.current.close();
            }

            // Only update URL if it's a connection error
            if (!hasLLMError) {
              setupEventSource();
            }
          }, delay);
        } else {
          const message = hasLLMError
            ? "LLM configuration error. Please check your settings."
            : "Maximum reconnection attempts reached. Please refresh the page.";

          onMessageReceived({
            id: generateTempId(),
            type: "system",
            payload: message,
            errorType: hasLLMError ? 'llm_config' : 'connection'
          });
          setError(message);
        }
      };

      // Handle ping events to keep connection alive
      eventSource.current.addEventListener('ping', () => {
        console.log('Received ping');
      });
    };

    setIsLoading(true);
    const timer = setTimeout(() => {
      setupEventSource();

      // Set loading timeout
      loadingTimeoutRef.current = setTimeout(() => {
        if (!isConnectedRef.current) {
          setIsLoading(false);
          onMessageReceived(createSystemMessage("Connection timeout. Please try again."));
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
