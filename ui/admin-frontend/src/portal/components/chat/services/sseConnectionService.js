import pubClient from '../../../../admin/utils/pubClient';
import { detectErrorType, generateTempId } from '../utils/chatMessageUtils';

export const setupSSEConnection = ({
  eventSourceRef,
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
  maxReconnectAttempts = 5,
  initialReconnectDelay = 500
}) => {
  let currentSessionId = continueId;

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

  if (!eventSourceRef.current || eventSourceRef.current.readyState === EventSource.CLOSED) {
    eventSourceRef.current = new EventSource(url, {
      withCredentials: true
    });
  }

  eventSourceRef.current.onopen = () => {
    setIsConnected(true);
    isConnectedRef.current = true;
    reconnectAttempts.current = 0;
    setError(null);

    if (loadingTimeoutRef.current) {
      clearTimeout(loadingTimeoutRef.current);
      loadingTimeoutRef.current = null;
    }
  };

  // Listen specifically for session_id events
  eventSourceRef.current.addEventListener('session_id', (event) => {
    try {
      const data = JSON.parse(event.data);
      const newSessionId = data.payload;
      
      currentSessionId = newSessionId;
      
      setSessionId(newSessionId);
      // Update URL with new session ID
      const newUrl = `/chat/${chatId}?continue_id=${newSessionId}`;
      try {
        window.history.replaceState({}, "", newUrl);
      } catch (err) {
        console.error('Failed to update URL:', err);
      }

      // Handle tools and datasources
      if (Array.isArray(data.tools)) {
        data.tools.forEach(tool => {
          const uniqueId = `tool-${tool.id}`;
          tool.type = 'tool';
          tool.uniqueId = uniqueId;
        });
      }
      if (Array.isArray(data.datasources)) {
        data.datasources.forEach(ds => {
          const uniqueId = `database-${ds.id}`;
          ds.type = 'database';
          ds.uniqueId = uniqueId;
        });
      }
      // Forward the entire session_id message
      onMessageReceived(data);

      if (continueId) {
        fetchChatHistory(continueId).then((messages) => {
          if (Array.isArray(messages)) {
            onMessageReceived({
              type: 'history',
              payload: JSON.stringify(messages.map(msg => ({
                id: msg.id,
                attributes: {
                  content: JSON.stringify({
                    role: msg.type === 'user' ? 'human' : msg.type === 'ai' ? 'ai' : 'system',
                    text: msg.content
                  })
                }
              })))
            });
          }
        }).catch((error) => {
          console.error("Error fetching chat history:", error);
        }).finally(() => {
          setIsLoading(false);
        });
      } else {
        setIsLoading(false);
      }
    } catch (error) {
      console.error("Error parsing SSE message:", error);
      onMessageReceived({
        id: generateTempId(),
        type: "system",
        payload: "Error: Failed to parse message from server",
        isComplete: true
      });
    }
  });

  // Handle stream_chunk events
  eventSourceRef.current.addEventListener('stream_chunk', (event) => {
    try {
      onMessageReceived({
        type: 'stream_chunk',
        payload: event.data
      });
    } catch (error) {
      console.error("Error handling stream chunk:", error);
    }
  });

  // Handle message events
  eventSourceRef.current.addEventListener('message', (event) => {
    try {
      const data = JSON.parse(event.data);
      onMessageReceived(data);
    } catch (error) {
      console.error("Error parsing message:", error);
    }
  });

  // Handle system events
  eventSourceRef.current.addEventListener('system', (event) => {
    try {
      const messageContent = event.data.includes(':::system')
        ? event.data
        : `:::system ${event.data}:::`;
      onMessageReceived({
        id: generateTempId(),
        type: 'system',
        payload: messageContent,  // Fixed: Use 'payload' instead of 'content' for consistency
        isComplete: true
      });
    } catch (error) {
      console.error("Error handling system message:", error);
    }
  });

  // Handle error events
  eventSourceRef.current.addEventListener('error', (event) => {
    try {
      const errorType = detectErrorType(event.data);

      // For LLM config errors, don't attempt reconnection
      if (errorType === 'llm_config') {
        reconnectAttempts.current = maxReconnectAttempts; // Prevent reconnection
      }

      onMessageReceived({
        id: generateTempId(),
        type: 'system',
        payload: `:::system Error: ${event.data}:::`,  // Fixed: Use 'payload' instead of 'content' for consistency
        errorType: errorType,
        isComplete: true
      });
    } catch (error) {
      console.error("Error handling error message:", error);
    }
  });

  // Handle any other events
  eventSourceRef.current.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      onMessageReceived(data);
    } catch (error) {
      console.error("Error parsing generic message:", error);
      onMessageReceived({
        id: generateTempId(),
        type: "system",
        payload: "Error: Failed to parse message from server",
        isComplete: true
      });
    }
  };

  const handleConnectionError = (error) => {
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

      setTimeout(() => {
        reconnectAttempts.current++;

        if (eventSourceRef.current) {
          eventSourceRef.current.close();
        }

        // Only update URL if it's a connection error
        if (!hasLLMError) {
          setupSSEConnection({
            eventSourceRef,
            chatId,
            continueId: currentSessionId || continueId,
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

  eventSourceRef.current.onerror = handleConnectionError;

  // Handle ping events to keep connection alive
  eventSourceRef.current.addEventListener('ping', () => {
    console.log('Received ping');
  });
};
