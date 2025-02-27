import React, { useState, useRef, useCallback, useEffect } from 'react';
import { debounce } from 'lodash';
import { useParams, useLocation, useNavigate } from 'react-router-dom';
import {
  Box,
  Typography,
  Paper,
  CircularProgress,
  Grid,
  Snackbar,
  Alert,
  IconButton,
} from '@mui/material';
import { TitleBox } from '../../admin/styles/sharedStyles';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import pubClient from '../../admin/utils/pubClient';

import { useChatSSE } from './chat/hooks/useChatSSE';
import MessageContent from './chat/MessageContent';
import ChatInput from './chat/ChatInput';
import ChatSidebar from './chat/ChatSidebar';
import simulateAgenticMode from './chat/AgenticModeMockData';

/**
 * Modified ChatView to use Server-Sent Events (SSE) instead of WebSocket.
 * It listens for a 'history' message from the SSE connection
 * to load old messages once, ensuring only a single request is made.
 */
const ChatView = () => {
  const [currentlyUsing, setCurrentlyUsing] = useState([]);
  const [databases, setDatabases] = useState([]);
  const [tools, setTools] = useState([]);
  const [messages, setMessages] = useState([]);
  const [inputMessage, setInputMessage] = useState('');
  const [error, setError] = useState(null);
  const [uploadedFiles, setUploadedFiles] = useState([]);
  const [isUploading, setIsUploading] = useState(false);
  const [showTools, setShowTools] = useState(true);
  const [expandedGroups, setExpandedGroups] = useState({});
  const [autoScroll, setAutoScroll] = useState(true);
  const [showSystemMessages, setShowSystemMessages] = useState(() => {
    const saved = localStorage.getItem('showSystemMessages');
    return saved !== null ? JSON.parse(saved) : false;
  });
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'error',
  });
  const [chatName, setChatName] = useState('');
  const [chatDescription, setChatDescription] = useState('');
  const [userName, setUserName] = useState('');
  const [isAgenticMode, setIsAgenticMode] = useState(false);

  const { chatId } = useParams();
  const location = useLocation();
  const messageContainerRef = useRef(null);
  const navigate = useNavigate();
  const lastTypeRef = useRef(null);

  // Process error messages to extract meaningful information
  const processErrorMessage = (error) => {
    // LLM Configuration errors
    if (error.includes('failed to create message') ||
      error.includes('anthropic') ||
      error.includes('openai')) {
      return {
        type: 'llm_config',
        title: 'LLM Configuration Error',
        message: 'Unable to generate response due to LLM configuration',
        details: error
      };
    }

    // API errors (e.g., "API returned unexpected status code: 404: Not Found")
    if (error.includes('API returned unexpected status code')) {
      const statusMatch = error.match(/status code: (\d+)/);
      const status = statusMatch ? statusMatch[1] : '';
      return {
        type: 'api',
        title: 'API Error',
        message: `Unable to generate response`,
        details: status === '404' ? 'Service not found' :
          status === '401' ? 'Authentication failed' :
            status === '429' ? 'Rate limit exceeded' :
              `API returned ${status}`
      };
    }

    // Connection errors
    if (error.includes('Connection lost') || error.includes('Failed to connect')) {
      return {
        type: 'connection',
        title: 'Connection Error',
        message: 'Lost connection to the server',
        details: error
      };
    }

    // Authentication errors
    if (error.includes('Unauthorized') || error.includes('Authentication failed')) {
      return {
        type: 'auth',
        title: 'Authentication Error',
        message: 'Please sign in again',
        details: error
      };
    }

    // Parse error message into parts
    const errorParts = error.split(':');
    return {
      type: 'system',
      title: errorParts.length > 1 ? errorParts[0].trim() : 'Error',
      message: errorParts.length > 1 ? errorParts[1].trim() : error,
      details: errorParts.length > 2 ? errorParts.slice(2).join(':').trim() : undefined
    };
  };

  const handleRealtimeChunks = useCallback((data) => {
    const currentType = data.type;
    const lastType = lastTypeRef.current;

    if (data.type === 'history') {
      try {
        const parsed = JSON.parse(data.payload);
        if (Array.isArray(parsed)) {
          const processedMessages = parsed.map(msg => {
            try {
              const content = msg.attributes?.content || msg.content;
              const messageId = msg.id || msg.attributes?.id;
              if (!messageId) {
                console.error('Message ID missing from server response:', msg);
                return null;
              }
              const parsedContent = JSON.parse(content);

              const messageText = parsedContent.text || parsedContent.content;
              const context = parsedContent.context;

              const messageContent = context ? `[CONTEXT]${context}[/CONTEXT]${messageText}` : messageText;

              switch (parsedContent.role) {
                case 'human':
                  return {
                    id: messageId,
                    type: 'user',
                    content: messageContent,
                    isComplete: true
                  };
                case 'ai':
                  return {
                    id: messageId,
                    type: 'ai',
                    content: messageContent,
                    isComplete: true
                  };
                case 'system':
                  const systemText = messageText.includes(':::system')
                    ? messageContent
                    : `:::system ${messageContent}:::`;
                  return {
                    id: messageId,
                    type: 'system',
                    content: systemText,
                    isComplete: true
                  };
                case 'tool':
                  return {
                    id: messageId,
                    type: 'ai',
                    content: messageContent,
                    isComplete: true
                  };
                default:
                  return null;
              }
            } catch (e) {
              console.error('Failed to parse message:', e);
              return null;
            }
          }).filter(msg => msg !== null);

          setMessages(processedMessages);
        }
      } catch (err) {
        console.error('Failed to parse incoming history:', err);
      }
      return;
    }

    lastTypeRef.current = currentType;

    if (currentType === 'stream_chunk' || currentType === 'message' || currentType === 'ai_message') {
      setMessages((prevMessages) => {
        const newMessages = [...prevMessages];
        const lastMessage = newMessages[newMessages.length - 1];
        let content;
        let isComplete = false;

        try {
          if (currentType === 'message' || currentType === 'ai_message') {
            const parsedPayload = JSON.parse(data.payload);
            if (parsedPayload.role === 'ai') {
              content = parsedPayload.text;
              isComplete = true;
            } else {
              content = data.payload;
            }
          } else {
            content = data.payload;
          }
        } catch (e) {
          content = data.payload;
        }

        if (currentType === 'stream_chunk') {
          const decodedContent = content.replace(/\\n/g, '\n');

          if (lastMessage && lastMessage.type === 'ai' && !lastMessage.isComplete) {
            newMessages[newMessages.length - 1] = {
              ...lastMessage,
              content: lastMessage.content + decodedContent,
              isComplete: false
            };
          } else {
            newMessages.push({
              id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
              type: 'ai',
              content: decodedContent,
              isComplete: false
            });
          }
        } else {
          if (lastMessage && lastMessage.type === 'ai' && !lastMessage.isComplete) {
            newMessages[newMessages.length - 1] = {
              ...lastMessage,
              content: content,
              isComplete: true
            };
          } else {
            newMessages.push({
              id: data.id || `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
              type: 'ai',
              content: content,
              isComplete: true
            });
          }
        }
        return newMessages;
      });
    } else if (data.type === 'historical_user_message' || data.type === 'user_message') {
      if (data.type === 'historical_user_message' || !data.isEdited) {
        setMessages((prevMessages) => {
          const lastMessage = prevMessages[prevMessages.length - 1];
          if (lastMessage && lastMessage.type === 'user' && lastMessage.id.startsWith('temp_')) {
            const updatedMessages = [...prevMessages];
            updatedMessages[updatedMessages.length - 1] = {
              ...lastMessage,
              id: data.id || lastMessage.id
            };
            return updatedMessages;
          }
          return [
            ...prevMessages,
            {
              id: data.id || `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
              type: 'user',
              content: data.payload,
              isComplete: true,
            },
          ];
        });
      }
    } else if (data.type === 'error' || data.type === 'system' || data.type === 'tool') {
      let messageContent = data.payload;

      if (messageContent.includes('Currently using')) {
        return;
      }

      if (data.type === 'error') {
        const errorInfo = processErrorMessage(data.payload);
        messageContent = `:::system ${errorInfo.title}\n${errorInfo.message}${errorInfo.details ? `\n[Details: ${errorInfo.details}]` : ''}:::`;
      } else if (data.type === 'tool') {
        if (!data.payload.includes(':::system')) {
          messageContent = `:::system ${data.payload}:::`;
        }
      }

      setMessages((prevMessages) => [
        ...prevMessages,
        {
          id: data.id || `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
          type: 'system',
          content: messageContent,
          isComplete: true,
          errorType: data.type === 'error' ? processErrorMessage(data.payload).type : undefined
        },
      ]);
    } else if (data.type === 'session_id') {
      if (data.tools) {
        setTools(prev => prev.map(tool => ({
          ...tool,
          isSelected: data.tools.some(t => t.id === tool.id)
        })));
      }
      if (data.datasources) {
        setDatabases(prev => prev.map(db => ({
          ...db,
          isSelected: data.datasources.some(d => d.id === db.id)
        })));
      }
    }
  }, []);

  const {
    isConnected,
    isLoading,
    sessionId,
    sendMessage,
    closeConnection,
    error: sseError,
    setError: setSSEError,
  } = useChatSSE({
    chatId,
    onMessageReceived: handleRealtimeChunks
  });

  useEffect(() => {
    if (isConnected) {
      setError(null);
      setSSEError(null);
    }
  }, [isConnected, setError, setSSEError]);

  useEffect(() => {
    if (error || sseError) {
      console.warn('Encountered error in ChatView:', error || sseError);
    }
  }, [error, sseError]);

  const scrollToBottom = useCallback(() => {
    if (messageContainerRef.current) {
      const scrollHeight = messageContainerRef.current.scrollHeight;
      const height = messageContainerRef.current.clientHeight;
      const maxScrollTop = scrollHeight - height;
      messageContainerRef.current.scrollTo({
        top: maxScrollTop > 0 ? maxScrollTop : 0,
        behavior: 'smooth',
      });
    }
  }, []);

  useEffect(() => {
    if (autoScroll) {
      scrollToBottom();
    }
  }, [messages, autoScroll, scrollToBottom]);

  useEffect(() => {
    const handleScroll = () => {
      if (messageContainerRef.current) {
        const { scrollHeight, clientHeight, scrollTop } = messageContainerRef.current;
        const isScrolledToBottom = scrollHeight - clientHeight <= scrollTop + 1;
        setAutoScroll(isScrolledToBottom);
      }
    };

    const messageContainer = messageContainerRef.current;
    if (messageContainer) {
      messageContainer.addEventListener('scroll', handleScroll);
    }

    return () => {
      if (messageContainer) {
        messageContainer.removeEventListener('scroll', handleScroll);
      }
    };
  }, []);

  const debouncedScrollToBottom = useCallback(debounce(scrollToBottom, 100), [scrollToBottom]);

  useEffect(() => {
    const messageContainer = messageContainerRef.current;
    if (!messageContainer) return;

    const resizeObserver = new ResizeObserver(() => {
      try {
        if (autoScroll) {
          if (messageContainer.scrollHeight - messageContainer.clientHeight - messageContainer.scrollTop > 1) {
            debouncedScrollToBottom();
          }
        }
      } catch (error) {
        console.error("ResizeObserver error:", error);
      }
    });
    resizeObserver.observe(messageContainer);

    return () => {
      resizeObserver.unobserve(messageContainer);
      debouncedScrollToBottom.cancel();
    };
  }, [autoScroll, debouncedScrollToBottom]);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const cachedUserData = localStorage.getItem('userData');
        let userEntitlements;

        if (cachedUserData) {
          const parsedData = JSON.parse(cachedUserData);
          userEntitlements = parsedData.entitlements;
          setUserName(parsedData.name);
        } else {
          const response = await pubClient.get('/common/me');
          const userData = response.data.attributes;
          userEntitlements = userData.entitlements;
          setUserName(userData.name);
          localStorage.setItem(
            'userData',
            JSON.stringify(userData)
          );
        }

        const currentChat = userEntitlements.chats.find((chat) => chat.id === chatId);
        if (currentChat) {
          setShowTools(currentChat.attributes.tool_support);
          setChatName(currentChat.attributes.name);
          setChatDescription(currentChat.attributes.description);
        }

        const [databasesResponse, toolsResponse] = await Promise.all([
          pubClient.get('/common/accessible-datasources'),
          pubClient.get('/common/accessible-tools'),
        ]);

        const newDatabases = databasesResponse.data.map((db) => ({
          id: db.id.toString(),
          name: db.attributes.name,
          type: 'database',
          description: db.attributes.short_description,
          icon: db.attributes.icon,
          isSelected: false
        }));

        const newTools = toolsResponse.data.map((tool) => ({
          id: tool.id.toString(),
          name: tool.attributes.name,
          type: 'tool',
          description: tool.attributes.description,
          toolType: tool.attributes.tool_type,
          isSelected: false
        }));

        setDatabases(newDatabases);
        setTools(newTools);
      } catch (error) {
        console.error('Error fetching data:', error);
        setMessages((prevMessages) => [
          ...prevMessages,
          {
            id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
            type: 'system',
            content: ':::system Error: Failed to load databases and tools:::',
            isComplete: true,
          },
        ]);
      }
    };

    fetchData();
  }, [chatId]);

  useEffect(() => {
    return () => {
      if (closeConnection) {
        closeConnection();
      }
      setMessages([]);
      setCurrentlyUsing([]);
      setDatabases([]);
      setTools([]);
      setInputMessage('');
      setError(null);
      setUploadedFiles([]);
      setIsUploading(false);
      setExpandedGroups({});
      setAutoScroll(true);
    };
  }, [chatId, closeConnection]);

  const onDrop = useCallback(
    (acceptedFiles) => {
      if (!sessionId) {
        setSnackbar({
          open: true,
          message: 'Cannot upload files: No active session',
          severity: 'error',
        });
        return;
      }

      setIsUploading(true);
      const uploadPromises = acceptedFiles.map((file) => {
        const formData = new FormData();
        formData.append('file', file);
        return pubClient
          .post(`/common/chat-sessions/${sessionId}/upload`, formData, {
            headers: { 'Content-Type': 'multipart/form-data' },
          })
          .then(() => ({ name: file.name, size: file.size }))
          .catch((error) => {
            setSnackbar({
              open: true,
              message: `Failed to upload ${file.name}: ${error.response?.data?.errors?.[0]?.detail || error.message}`,
              severity: 'error',
            });
            return null;
          });
      });

      Promise.all(uploadPromises).then((fileInfos) => {
        const successfulUploads = fileInfos.filter((info) => info !== null);
        setUploadedFiles((prev) => [...prev, ...successfulUploads]);
        setIsUploading(false);
        if (successfulUploads.length > 0) {
          setSnackbar({
            open: true,
            message: `Successfully uploaded ${successfulUploads.length} file(s)`,
            severity: 'success',
          });
        }
      });
    },
    [sessionId]
  );

  const toggleAgenticMode = () => {
    setIsAgenticMode(prev => !prev);
  };

  const handleSendMessage = (e) => {
    e.preventDefault();
    if ((inputMessage.trim() || uploadedFiles.length > 0) && isConnected) {
      const messageContent = inputMessage.trim();
      const tempId = `temp_${Math.floor(Math.random() * 1_000_000_000)}`;

      // Add user message to the chat
      setMessages((prevMessages) => [
        ...prevMessages,
        {
          id: tempId,
          type: 'user',
          content: messageContent,
          isComplete: true,
        },
      ]);

      if (isAgenticMode) {
        // Use the complex agentic mode simulation
        simulateAgenticMode(setMessages);
      } else {
        // Normal mode - send to server
        const message = {
          type: 'user_message',
          payload: messageContent,
          file_refs: uploadedFiles.map((file) => file.name),
        };
        sendMessage(message);
      }

      setInputMessage('');
      setUploadedFiles([]);
    }
  };

  const renderUploadIndicator = () => {
    if (isUploading) {
      return <CircularProgress size={20} />;
    }
    if (uploadedFiles.length > 0) {
      return <CheckCircleOutlineIcon color="success" />;
    }
    return null;
  };

  const toggleGroup = (groupId) => {
    setExpandedGroups((prev) => ({
      ...prev,
      [groupId]: !prev[groupId],
    }));
  };

  const removeFromCurrentlyUsing = async (item, currentSessionId) => {
    if (!currentSessionId) return;
    try {
      let response;
      if (item.type === 'database') {
        response = await pubClient.delete(
          `/common/chat-sessions/${currentSessionId}/datasources/${item.id}`
        );
      } else if (item.type === 'tool') {
        response = await pubClient.delete(
          `/common/chat-sessions/${currentSessionId}/tools/${item.id}`
        );
      }

      if (response.status === 200 || response.status === 204) {
        if (item.type === 'database') {
          setDatabases(prev => prev.map(db =>
            db.id === item.id ? { ...db, isSelected: false } : db
          ));
        } else if (item.type === 'tool') {
          setTools(prev => prev.map(tool =>
            tool.id === item.id ? { ...tool, isSelected: false } : tool
          ));
        }
      }
    } catch (error) {
      console.error('Error removing item from chat session:', error);
    }
  };

  const addToCurrentlyUsing = async (item, currentSessionId) => {
    if (!currentSessionId) return;
    try {
      let response;
      if (item.type === 'database') {
        response = await pubClient.post(
          `/common/chat-sessions/${currentSessionId}/datasources`,
          { datasource_id: parseInt(item.id) }
        );
      } else if (item.type === 'tool') {
        response = await pubClient.post(`/common/chat-sessions/${currentSessionId}/tools`, {
          tool_id: item.id,
        });
      }

      if (response.status === 200 || response.status === 201) {
        if (item.type === 'database') {
          setDatabases(prev => prev.map(db =>
            db.id === item.id ? { ...db, isSelected: true } : db
          ));
        } else if (item.type === 'tool') {
          setTools(prev => prev.map(tool =>
            tool.id === item.id ? { ...tool, isSelected: true } : tool
          ));
        }
      }
    } catch (error) {
      console.error('Error adding item to chat session:', error);
      let errorMessage = 'Failed to add item to chat session';
      if (error.response?.data?.errors) {
        errorMessage = error.response.data.errors[0].detail || errorMessage;
      }
      setSnackbar({
        open: true,
        message: errorMessage,
        severity: 'error',
      });
    }
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === 'clickaway') {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  // Show error if we have an error, but don't block UI for LLM config errors
  if ((error || sseError) && !isConnected) {
    const errorInfo = processErrorMessage(error || sseError);
    const isLLMError = errorInfo.type === 'llm_config';

    return (
      <Box display="flex" justifyContent="center" alignItems="center" height="100vh">
        <Alert
          severity={isLLMError ? "warning" : "error"}
          sx={{
            maxWidth: '600px',
            '& .MuiAlert-message': {
              width: '100%'
            }
          }}
        >
          <Typography variant="subtitle1" sx={{ fontWeight: 'bold', mb: 1 }}>
            {errorInfo.title}
          </Typography>
          <Typography variant="body1" sx={{ mb: errorInfo.details ? 1 : 0 }}>
            {errorInfo.message}
          </Typography>
          {errorInfo.details && (
            <Typography variant="body2" color="text.secondary" sx={{ fontSize: '0.85em' }}>
              Details: {errorInfo.details}
            </Typography>
          )}
        </Alert>
      </Box>
    );
  }

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" height="100vh">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">{chatName}</Typography>
      </TitleBox>
      <Box
        sx={{
          height: '85vh',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Grid container sx={{ flexGrow: 1, overflow: 'hidden', mb: 4 }}>
          <Grid item xs={9} sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
            {/* Messages container */}
            <Box
              sx={{
                flexGrow: 1,
                overflowY: 'auto',
                display: 'flex',
                flexDirection: 'column',
                width: '100%',
              }}
              ref={messageContainerRef}
            >
              <Box sx={{
                maxWidth: '740px',
                width: '100%',
                mx: 'auto', // Center with auto margins
                display: 'flex',
                flexDirection: 'column',
                flexGrow: 1,
              }}>
                {messages.length === 0 ? (
                  <Box sx={{
                    width: '100%',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flex: 1,
                  }}>
                    <Box sx={{
                      width: '100%',
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'flex-start',
                      justifyContent: 'start',
                      textAlign: 'start',
                    }}>
                      <Typography variant="headingXLarge" mb={2}>
                        Welcome to {chatName} chat
                      </Typography>
                      <Typography variant="headingXLargSub" mb={3}>
                        How can I help you today?
                      </Typography>
                      {chatDescription && (
                        <Typography variant="bodyLargeDefault" color="text.defaultSubdued" mb={4} maxWidth="600px">
                          {chatDescription}
                        </Typography>
                      )}
                    </Box>
                    <Box sx={{ width: '100%', mt: 2 }}>
                      <ChatInput
                        inputMessage={inputMessage}
                        setInputMessage={setInputMessage}
                        handleSendMessage={handleSendMessage}
                        isConnected={isConnected}
                        uploadedFiles={uploadedFiles}
                        setUploadedFiles={setUploadedFiles}
                        onDrop={(files) => onDrop(files, sessionId)}
                        isUploading={isUploading}
                        renderUploadIndicator={renderUploadIndicator}
                        isAgenticMode={isAgenticMode}
                        toggleAgenticMode={toggleAgenticMode}
                      />
                    </Box>
                  </Box>
                ) : (
                  <>
                    {messages.length > 1 && (
                      <Box sx={{ mt: 2, textAlign: 'right' }}>
                        <Typography
                          variant="caption"
                          component="div"
                          onClick={() => {
                            const newValue = !showSystemMessages;
                            setShowSystemMessages(newValue);
                            localStorage.setItem('showSystemMessages', JSON.stringify(newValue));
                          }}
                          sx={{
                            cursor: 'pointer',
                            display: 'inline-flex',
                            alignItems: 'center',
                            color: showSystemMessages ? 'primary.main' : 'text.secondary',
                            '&:hover': {
                              color: 'primary.main',
                            },
                          }}
                        >
                          {showSystemMessages ? 'Hide' : 'Show'} System and Context Messages
                        </Typography>
                      </Box>
                    )}
                    {messages.map((message, index) => {
                      if (!showSystemMessages && message.type === 'system') {
                        return null;
                      }

                      return (
                        <MessageContent
                          key={message.id || index}
                          content={message.content}
                          messageIndex={index}
                          expandedGroups={expandedGroups}
                          toggleGroup={toggleGroup}
                          messageId={message.id}
                          messageType={message.type}
                          sessionId={sessionId}
                          showSystemMessages={showSystemMessages}
                          chatId={chatId}
                          userName={userName}
                          onEditSuccess={(editedText, messageId) => {
                            setMessages(prevMessages => {
                              const messageIndex = prevMessages.findIndex(msg => msg.id === messageId);
                              if (messageIndex === -1) return prevMessages;

                              const updatedMessages = prevMessages.slice(0, messageIndex + 1);
                              updatedMessages[messageIndex] = {
                                ...updatedMessages[messageIndex],
                                content: editedText
                              };
                              return updatedMessages;
                            });

                            sendMessage({
                              type: 'user_message',
                              payload: editedText,
                              file_refs: []
                            });
                          }}
                        />
                      );
                    })}

                    {!autoScroll && (
                      <IconButton
                        onClick={scrollToBottom}
                        sx={{
                          position: 'absolute',
                          bottom: 70,
                          right: 20,
                          backgroundColor: 'background.paper',
                          '&:hover': { backgroundColor: 'action.hover' },
                        }}
                      >
                        <KeyboardArrowDownIcon />
                      </IconButton>
                    )}
                  </>
                )}
              </Box>
            </Box>

            {/* Fixed input at bottom - only show when there are messages */}
            {messages.length > 0 && (
              <Box sx={{
                width: '100%',
                padding: 2,
                paddingTop: 0,
              }}>
                <Box sx={{
                  maxWidth: '740px',
                  width: '100%',
                  mx: 'auto', // Center with auto margins
                }}>
                  <ChatInput
                    inputMessage={inputMessage}
                    setInputMessage={setInputMessage}
                    handleSendMessage={handleSendMessage}
                    isConnected={isConnected}
                    uploadedFiles={uploadedFiles}
                    setUploadedFiles={setUploadedFiles}
                    onDrop={(files) => onDrop(files, sessionId)}
                    isUploading={isUploading}
                    renderUploadIndicator={renderUploadIndicator}
                    isAgenticMode={isAgenticMode}
                    toggleAgenticMode={toggleAgenticMode}
                  />
                </Box>
              </Box>
            )}
          </Grid>

          <Grid item xs={3} sx={{ height: '100%', overflowY: 'auto' }}>
            <ChatSidebar
              currentlyUsing={currentlyUsing}
              databases={databases}
              tools={tools}
              showTools={showTools}
              removeFromCurrentlyUsing={(item) => removeFromCurrentlyUsing(item, sessionId)}
              addToCurrentlyUsing={(item) => addToCurrentlyUsing(item, sessionId)}
              messages={messages}
            />
          </Grid>
        </Grid>

        <Snackbar
          open={snackbar.open}
          autoHideDuration={6000}
          onClose={handleCloseSnackbar}
          anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
        >
          <Alert
            onClose={handleCloseSnackbar}
            severity={snackbar.severity}
            sx={{ width: '100%' }}
          >
            {snackbar.message}
          </Alert>
        </Snackbar>
      </Box>
    </>
  );
};

export default ChatView;
