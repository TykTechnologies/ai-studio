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
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import pubClient from '../../admin/utils/pubClient';

import { useChatWebSocket } from './chat/hooks/useChatWebSocket';
import MessageContent from './chat/MessageContent';
import ChatInput from './chat/ChatInput';
import ChatSidebar from './chat/ChatSidebar';

/**
 * Modified ChatView to remove the extra REST fetch of chat messages.
 * Now it only listens for a 'history' message from the WebSocket
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
  const [chatName, setChatName] = useState('');
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

  const { chatId } = useParams();
  const location = useLocation();
  const messageContainerRef = useRef(null);
  const navigate = useNavigate();

  /**
   * We no longer fetch messages from REST. Instead, once our
   * WebSocket is open, the server will send a "history" type
   * message that we use to populate 'messages'.
   */

  const updateChatName = useCallback(async (name, currentSessionId) => {
    try {
      let truncatedName = name.trim().slice(0, 60);
      if (name.length > 60) {
        truncatedName += '...';
      }
      await pubClient.put(`/common/chat-history-records/${currentSessionId}/name`, {
        name: truncatedName,
      });
      setChatName(truncatedName);
    } catch (error) {
      console.error('Error updating chat name:', error);
      setSnackbar({
        open: true,
        message: 'Failed to update chat name',
        severity: 'error',
      });
    }
  }, []);

  /**
   * handleRealtimeChunks merges repeated system/error lines into the last system message
   * if that last system message is also "system".
   */
  const lastTypeRef = useRef(null);

  // const reorderToolMessages = (messages) => {
  //   console.log("Reordering tool messages", messages);
  //   const result = [...messages];

  //   for (let i = 0; i < result.length; i++) {
  //     // Look for an AI message that contains "tool_use"
  //     if (result[i]?.type === 'ai' && result[i]?.content.includes('tool_use')) {
  //       // Check if the next two messages exist:
  //       // - The second message should be a tool_result message.
  //       // - The third message is the explanation (should not include "tool_use" or "tool_result").
  //       if (
  //         i + 2 < result.length &&
  //         result[i + 1]?.type === 'ai' && result[i + 1]?.content.includes('tool_result') &&
  //         result[i + 2]?.type === 'ai' &&
  //         !result[i + 2]?.content.includes('tool_use') &&
  //         !result[i + 2]?.content.includes('tool_result')
  //       ) {
  //         // Remove the explanation message from its current position.
  //         const explanation = result.splice(i + 2, 1)[0];
  //         // Insert the explanation message before the tool_use message.
  //         result.splice(i, 0, explanation);
  //         // Skip ahead by two positions since we've just processed this group.
  //         i += 2;
  //       }
  //     }
  //   }

  //   return result;
  // };


  const handleRealtimeChunks = useCallback((data) => {
    const currentType = data.type;
    const lastType = lastTypeRef.current;

    // If the server sends the entire chat history
    if (data.type === 'history') {
      try {
        const parsed = JSON.parse(data.payload);
        if (Array.isArray(parsed)) {
          const processedMessages = parsed.map(msg => {
            try {
              const content = msg.attributes?.content || msg.content;
              const parsedContent = JSON.parse(content);

              // Keep the original text which may include context tags
              const messageText = parsedContent.text || parsedContent.content;
              const context = parsedContent.context;

              const messageContent = context ? `[CONTEXT]${context}[/CONTEXT]${messageText}` : messageText;

              switch (parsedContent.role) {
                case 'human':
                  return {
                    id: Math.floor(Math.random() * 1_000_000_000),
                    type: 'user',
                    content: messageContent,
                    isComplete: true
                  };
                case 'ai':
                  return {
                    id: Math.floor(Math.random() * 1_000_000_000),
                    type: 'ai',
                    content: messageContent,
                    isComplete: true
                  };
                case 'system':
                  // Ensure system messages have the :::system::: wrapper
                  const systemText = messageText.includes(':::system')
                    ? messageContent
                    : `:::system ${messageContent}:::`;
                  return {
                    id: Math.floor(Math.random() * 1_000_000_000),
                    type: 'system',
                    content: systemText,
                    isComplete: true
                  };
                case 'tool':
                  return {
                    id: Math.floor(Math.random() * 1_000_000_000),
                    type: 'ai',
                    content: messageContent,
                    isComplete: true
                  };
                default:
                  console.log('Unknown role:', parsedContent.role);
                  return null;
              }
            } catch (e) {
              console.error('Failed to parse message:', e);
              return null;
            }
          }).filter(msg => msg !== null);

          // const reorderedMessages = reorderToolMessages(processedMessages);
          setMessages(processedMessages);
        }
      } catch (err) {
        console.error('Failed to parse incoming history:', err);
      }
      return;
    }

    // Update last message type
    lastTypeRef.current = currentType;

    // Process AI messages and stream chunks
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

        if (lastMessage && lastMessage.type === 'ai' && !lastMessage.isComplete) {
          newMessages[newMessages.length - 1] = {
            ...lastMessage,
            content: lastMessage.content + content,
            isComplete: isComplete
          };
        } else {
          newMessages.push({
            id: Math.floor(Math.random() * 1_000_000_000),
            type: 'ai',
            content: content,
            isComplete: isComplete
          });
        }
        return newMessages;
      });
    } else if (data.type === 'historical_user_message' || data.type === 'user_message') {
      // Handle both historical and real-time user messages
      // For historical messages or non-edited real-time messages, add to the list
      if (data.type === 'historical_user_message' || !data.isEdited) {
        console.log('Adding user message:', data);
        setMessages((prevMessages) => [
          ...prevMessages,
          {
            id: Math.floor(Math.random() * 1_000_000_000),
            type: 'user',
            content: data.payload,
            isComplete: true,
          },
        ]);
      }
    } else if (data.type === 'error' || data.type === 'system' || data.type === 'tool') {
      let messageContent = data.payload;

      // Skip "Currently using" tool status messages
      if (messageContent.includes('Currently using')) {
        console.log("Skipping 'Currently using' system message");
        return;
      }

      if (data.type === 'error') {
        messageContent = `:::system Error: ${data.payload}:::`;
      } else if (data.type === 'tool') {
        if (!data.payload.includes(':::system')) {
          messageContent = `:::system ${data.payload}:::`;
        }
      }

      setMessages((prevMessages) => [
        ...prevMessages,
        {
          id: Math.floor(Math.random() * 1_000_000_000),
          type: 'system',
          content: messageContent,
          isComplete: true,
        },
      ]);
    } else if (data.type === 'session_id') {
      // This is the initial "session_id" from server
      // We might store it or set tools/datasources
      if (data.tools) {
        setCurrentlyUsing(data.tools);
      }
      if (data.datasources) {
        setCurrentlyUsing(prev => [...prev, ...data.datasources]);
      }
      console.log('Session ID received:', data.payload);
    }
  }, []);

  const {
    isConnected,
    isLoading,
    sessionId,
    isNewChat,
    hasUpdatedChatName,
    setHasUpdatedChatName,
    setIsNewChat,
    sendMessage,
    closeWebSocket,
    error: wsError,
    setError: setWsError,
  } = useChatWebSocket({
    chatId,
    onMessageReceived: handleRealtimeChunks,
    updateChatName,
  });

  useEffect(() => {
    // Clear error states when connection is restored
    if (isConnected) {
      setError(null);
      setWsError(null);
    }
  }, [isConnected, setError, setWsError]);

  useEffect(() => {
    if ((error || wsError) && !isNewChat) {
      console.warn('Encountered error in ChatView:', error || wsError);
    }
  }, [error, wsError, isNewChat]);

  // Automatic scrolling
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
        const cachedEntitlements = localStorage.getItem('userEntitlements');
        let userEntitlements;

        if (cachedEntitlements) {
          const parsedData = JSON.parse(cachedEntitlements);
          userEntitlements = parsedData.data;
        } else {
          const response = await pubClient.get('/me');
          userEntitlements = response.data.attributes.entitlements;
          localStorage.setItem(
            'userEntitlements',
            JSON.stringify({ data: userEntitlements, timestamp: Date.now() })
          );
        }

        const currentChat = userEntitlements.chats.find((chat) => chat.id === chatId);
        if (currentChat) {
          setShowTools(currentChat.attributes.tool_support);
          setChatName(currentChat.attributes.name);
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
        }));

        const newTools = toolsResponse.data.map((tool) => ({
          id: tool.id.toString(),
          name: tool.attributes.name,
          type: 'tool',
          description: tool.attributes.description,
          toolType: tool.attributes.tool_type,
        }));

        setDatabases(newDatabases);
        setTools(newTools);
      } catch (error) {
        console.error('Error fetching data:', error);
        setMessages((prevMessages) => [
          ...prevMessages,
          {
            id: Math.floor(Math.random() * 1_000_000_000),
            type: 'system',
            content: ':::system Error: Failed to load databases and tools:::',
            isComplete: true,
          },
        ]);
      }
    };

    fetchData();
  }, [chatId]);

  // Clean up on unmount
  useEffect(() => {
    return () => {
      if (closeWebSocket) {
        closeWebSocket();
      }
      setMessages([]);
      setCurrentlyUsing([]);
      setDatabases([]);
      setTools([]);
      setInputMessage('');
      setError(null);
      setUploadedFiles([]);
      setIsUploading(false);
      setChatName('');
      setExpandedGroups({});
      setAutoScroll(true);
    };
  }, [chatId, closeWebSocket]);

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

  const handleSendMessage = (e) => {
    e.preventDefault();
    if ((inputMessage.trim() || uploadedFiles.length > 0) && isConnected) {
      const messageContent = inputMessage.trim();
      const message = {
        type: 'user_message',
        payload: messageContent,
        file_refs: uploadedFiles.map((file) => file.name),
      };
      sendMessage(message);
      setMessages((prevMessages) => [
        ...prevMessages,
        {
          // assign a local ID
          id: Math.floor(Math.random() * 1_000_000_000),
          type: 'user',
          content: messageContent,
          isComplete: true,
        },
      ]);

      if (isNewChat && !hasUpdatedChatName && sessionId) {
        updateChatName(inputMessage.trim(), sessionId);
        setHasUpdatedChatName(true);
        setIsNewChat(false);
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
        setCurrentlyUsing((prevItems) =>
          prevItems.filter((i) => i.uniqueId !== item.uniqueId)
        );
        if (item.type === 'database') {
          setDatabases((prevDatabases) => [...prevDatabases, item]);
        } else if (item.type === 'tool') {
          setTools((prevTools) => [...prevTools, item]);
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
        const uniqueId = `${item.type}-${item.id}`;
        setCurrentlyUsing((prevItems) => [...prevItems, { ...item, uniqueId }]);
        if (item.type === 'database') {
          setDatabases((prevDatabases) =>
            prevDatabases.filter((db) => db.id !== item.id)
          );
        } else if (item.type === 'tool') {
          setTools((prevTools) => prevTools.filter((tool) => tool.id !== item.id));
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

  // Only show error if we have an error AND we're not connected AND it's not a new chat
  if ((error || wsError) && !isConnected && !isNewChat) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" height="100vh">
        <Alert severity="error">{error || wsError}</Alert>
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
    <Box
      sx={{
        height: '85vh',
        display: 'flex',
        flexDirection: 'column',
        '& .inline-code': {
          display: 'inline-block',
          padding: '2px 4px',
          color: '#232629',
          backgroundColor: 'rgb(240, 240, 240)',
          borderRadius: '3px',
          fontFamily: 'monospace',
          fontSize: '0.9em',
        },
      }}
    >
      {chatName && (
        <Box sx={{ p: 2, borderBottom: '1px solid #e0e0e0' }}>
          <Typography variant="h6" component="h1">
            {chatName}
          </Typography>
        </Box>
      )}
      <Grid container sx={{ flexGrow: 1, overflow: 'hidden' }}>
        <Grid item xs={9} sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
          <Paper
            elevation={0}
            sx={{
              flexGrow: 1,
              display: 'flex',
              flexDirection: 'column',
              overflow: 'hidden',
              height: '100%',
            }}
          >
            <Box
              ref={messageContainerRef}
              sx={{
                flexGrow: 1,
                overflowY: 'auto',
                display: 'flex',
                flexDirection: 'column',
                scrollBehavior: 'smooth',
                '&::-webkit-scrollbar': {
                  width: '0.4em',
                },
                '&::-webkit-scrollbar-track': {
                  boxShadow: 'inset 0 0 6px rgba(0,0,0,0.00)',
                },
                '&::-webkit-scrollbar-thumb': {
                  backgroundColor: 'rgba(0,0,0,.1)',
                  outline: '1px solid slategrey',
                },
              }}
            >
              {messages.length > 1 && (
                <Box sx={{ p: 1, textAlign: 'right' }}>
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
                // Only skip pure system messages when hidden
                if (!showSystemMessages && message.type === 'system') {
                  return null;
                }
                return (
                  <Box
                    key={message.id || index}
                    sx={{
                      width: '100%',
                      p: 2,
                      borderTop: index > 0 ? '1px solid #e0e0e0' : 'none',
                      borderBottom:
                        index === messages.length - 1 ? '1px solid #e0e0e0' : 'none',
                      opacity: message.isComplete === false ? 0.9 : 1,
                    }}
                  >
                    {message.type !== 'system' && (
                      <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 1 }}>
                        {message.type === 'user'
                          ? 'You:'
                          : message.type === 'ai'
                            ? 'Assistant:'
                            : 'Tool:'}
                      </Typography>
                    )}
                    <MessageContent
                      content={message.content}
                      messageIndex={index}
                      expandedGroups={expandedGroups}
                      toggleGroup={toggleGroup}
                      messageId={message.id}
                      messageType={message.type}
                      sessionId={sessionId}
                      showSystemMessages={showSystemMessages}
                      onEditSuccess={(editedText) => {
                        // If the user edits, we re-broadcast or re-fetch
                        const newMsg = {
                          type: 'user_message',
                          payload: editedText,
                          file_refs: [],
                          isEdited: true
                        };
                        sendMessage(newMsg);
                        // We won't do an extra fetch. We'll rely on the server's WS broadcast.
                      }}
                    />
                  </Box>
                );
              })}
            </Box>

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
          </Paper>

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
          />
        </Grid>

        <Grid item xs={3} sx={{ height: '100%', overflowY: 'auto' }}>
          <ChatSidebar
            currentlyUsing={currentlyUsing}
            databases={databases}
            tools={tools}
            showTools={showTools}
            removeFromCurrentlyUsing={(item) => removeFromCurrentlyUsing(item, sessionId)}
            addToCurrentlyUsing={(item) => addToCurrentlyUsing(item, sessionId)}
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
  );
};

export default ChatView;
