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
        content: ":::system Error: Failed to parse message from server:::",
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
        content: messageContent,
        isComplete: true
      });
    } catch (error) {
      console.error("Error handling system message:", error);
    }
  });

  // Handle error events from the server (SSE events with type 'error')
  // Note: This is different from connection errors handled by onerror
  eventSourceRef.current.addEventListener('error', (event) => {
    try {
      // Only process if there's actual data (server-sent error event)
      // Connection errors trigger onerror, not this handler, but some browsers
      // may trigger both - we only want to handle events with data here
      if (!event.data) return;

      const errorType = detectErrorType(event.data);

      // For LLM config errors, don't attempt reconnection
      if (errorType === 'llm_config') {
        reconnectAttempts.current = maxReconnectAttempts; // Prevent reconnection
      }

      onMessageReceived({
        id: generateTempId(),
        type: 'system',
        content: `:::system Error: ${event.data}:::`,
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
        content: ":::system Error: Failed to parse message from server:::",
        isComplete: true
      });
    }
  };

  // Handle connection errors (network failures, disconnects)
  // Note: This is different from server-sent error events handled by addEventListener('error')
  const handleConnectionError = (error) => {
    console.error("SSE connection error:", error);

    setIsConnected(false);
    isConnectedRef.current = false;
    setIsLoading(false);

    // If error has data, it was a server-sent error event that was already
    // handled by addEventListener('error'). This handler is just being called
    // because the connection dropped after the error was sent.
    if (error?.data) {
      // Already handled by the 'error' event listener - don't duplicate the message
      return;
    }

    // Check if we've already hit max reconnect attempts (set by LLM config error handler)
    const isMaxedOut = reconnectAttempts.current >= maxReconnectAttempts;

    if (!isMaxedOut) {
      onMessageReceived({
        id: generateTempId(),
        type: "system",
        content: `Connection lost. Attempting to reconnect... (Attempt ${reconnectAttempts.current + 1}/${maxReconnectAttempts})`,
        errorType: 'connection',
        isComplete: true
      });

      const delay = initialReconnectDelay * Math.pow(2, reconnectAttempts.current);

      setTimeout(() => {
        reconnectAttempts.current++;

        if (eventSourceRef.current) {
          eventSourceRef.current.close();
        }

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
      }, delay);
    } else {
      // Only show "max attempts" message for connection errors, not LLM errors
      // LLM errors already have their own message from the server
      const message = "Maximum reconnection attempts reached. Please refresh the page.";

      onMessageReceived({
        id: generateTempId(),
        type: "system",
        content: message,
        errorType: 'connection',
        isComplete: true
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
