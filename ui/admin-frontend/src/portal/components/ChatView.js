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
 * This ChatView ensures:
 * 1) Consecutive system/error messages merge into the last system message.
 * 2) "[CONTEXT]" blocks or ":::system" lines are handled in the single unified MessageContent logic with toggling.
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
  const [isFetchingHistory, setIsFetchingHistory] = useState(false);
  const [chatName, setChatName] = useState('');
  const [showTools, setShowTools] = useState(true);
  const [expandedGroups, setExpandedGroups] = useState({});
  const [autoScroll, setAutoScroll] = useState(true);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'error',
  });

  const { chatId } = useParams();
  const location = useLocation();
  const messageContainerRef = useRef(null);
  const navigate = useNavigate();

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
  const handleRealtimeChunks = useCallback((data) => {
    if (data.type === 'stream_chunk' || data.type === 'ai_message') {
      setMessages((prevMessages) => {
        const newMessages = [...prevMessages];
        const lastMessage = newMessages[newMessages.length - 1];
        let content = data.payload;

        if (data.type === 'ai_message') {
          try {
            const parsed = JSON.parse(data.payload);
            content = parsed.text;
          } catch (e) {
            content = data.payload;
          }
        }

        if (
          lastMessage &&
          lastMessage.type === 'ai' &&
          !lastMessage.isComplete &&
          data.type === 'stream_chunk'
        ) {
          newMessages[newMessages.length - 1] = {
            ...lastMessage,
            content: lastMessage.content + content,
          };
        } else {
          // Assign a local random ID that doesn't have "temp_" so it can be edited if needed
          newMessages.push({
            id: Math.floor(Math.random() * 1_000_000_000),
            type: 'ai',
            content: content,
            isComplete: data.type === 'ai_message',
          });
        }
        return newMessages;
      });
    } else if (data.type === 'user_message') {
      setMessages((prevMessages) => [
        ...prevMessages,
        {
          // remove "temp_" prefix approach
          id: Math.floor(Math.random() * 1_000_000_000),
          type: 'user',
          content: data.payload,
          isComplete: true,
        },
      ]);
    } else if (data.type === 'error' || data.type === 'system' || data.type === 'tool') {
      let messageContent = data.payload;

      // Skip "Currently using" tool status messages
      if (messageContent.includes('Currently using')) {
        console.log("REMOVING!")
        return;
      }

      // The user specifically asked that 'system' lines or 'tool' lines should remain so we can see them as system messages
      // We'll unify them in the front-end rendering
      if (data.type === 'system') {
        // for the example, let's do nothing special here if we want them merged with system
        // or we can do the same approach as below
        return;
      }

      if (data.type === 'error') {
        messageContent = `:::system Error: ${data.payload}:::`;
      } else if (data.type === 'tool') {
        // For tool messages, don't wrap in system markers if they already are
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
      // no-op, we handle it in the hook
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

  // We unify message parsing so if the server returns either
  // { data: [...] } or just [ ... ],
  // we handle it gracefully and do not overwrite messages with []:
  const parseFetchedMessages = (respData) => {
    let messages = [];
    if (Array.isArray(respData)) {
      messages = respData;
    } else if (respData && Array.isArray(respData.data)) {
      messages = respData.data;
    }
    // Filter out "Currently using" messages
    return messages.filter(msg => {
      if (msg?.attributes?.content) {
        try {
          const parsed = JSON.parse(msg.attributes.content);
          if (parsed.role === 'system' && parsed.text?.includes('Currently using')) {
            return false;
          }
        } catch (e) {
          // If parsing fails, keep the message
        }
      }
      return true;
    });
  };

  const fetchMessagesForSession = useCallback(async (sessionID) => {
    if (!sessionID) return;
    setIsFetchingHistory(true);
    setError(null);
    try {
      const response = await pubClient.get(`/common/sessions/${sessionID}/messages?page_size=500`);
      const messagesData = parseFetchedMessages(response.data);
      if (!messagesData.length) {
        setMessages([]);
        setIsFetchingHistory(false);
        return;
      }
      const newMessages = messagesData
        .filter(msg => msg && msg.attributes)
        .map((msg) => {
          let content = msg.attributes.content;
          let role = 'assistant';
          try {
            const parsed = JSON.parse(content);
            role = parsed.role || 'assistant';

            // Skip "Currently using" messages
            if (parsed.role === 'system' && parsed.text?.includes('Currently using')) {
              return null;
            }

            if (parsed.text) {
              content = parsed.text;
            } else if (parsed.content) {
              content = parsed.content;
            }
          } catch (e) {
            // fallback: not valid JSON, interpret as text
          }

          let displayType;
          switch (role.toLowerCase()) {
            case 'user':
            case 'human':
              displayType = 'user';
              break;
            case 'assistant':
              displayType = 'ai';
              break;
            case 'system':
              displayType = 'system';
              // Always wrap system messages in :::system markers if they don't already have them
              if (!content.startsWith(':::system')) {
                content = `:::system ${content}:::`;
              }
              break;
            case 'tool':
              displayType = 'tool';
              break;
            default:
              displayType = 'ai';
              break;
          }

          return {
            // keep the actual ID from DB so it can be edited
            id: msg.id,
            type: displayType,
            content: content,
            // For existing messages, mark them as complete so new lines won't merge
            isComplete: true,
          };
        });

      setMessages(newMessages);
      setError(null);
    } catch (err) {
      console.error('Error fetching messages:', err);
      if (sessionID) {
        setError('Error fetching messages from DB');
      }
    }
    setIsFetchingHistory(false);
  }, []);

  const scrollToBottom = () => {
    if (messageContainerRef.current) {
      const scrollHeight = messageContainerRef.current.scrollHeight;
      const height = messageContainerRef.current.clientHeight;
      const maxScrollTop = scrollHeight - height;
      messageContainerRef.current.scrollTo({
        top: maxScrollTop > 0 ? maxScrollTop : 0,
        behavior: 'smooth',
      });
    }
  };

  useEffect(() => {
    if (autoScroll) {
      scrollToBottom();
    }
  }, [messages, autoScroll]);

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
          // Check if already scrolled to bottom before calling scrollToBottom
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

  useEffect(() => {
    if (sessionId && isConnected) {
      fetchMessagesForSession(sessionId);
    }
  }, [sessionId, isConnected, fetchMessagesForSession]);

  // Handle chat navigation - clean up state when unmounting
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
      setIsFetchingHistory(false);
      setChatName('');
      setExpandedGroups({});
      setAutoScroll(true);
    };
  }, [chatId, closeWebSocket]);

  useEffect(() => {
    if ((error || wsError) && !isNewChat) {
      setIsFetchingHistory(false);
    }
  }, [error, wsError, isNewChat]);

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
          // assign a local ID that is not prefixed with "temp_"
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

  if ((error || wsError) && !isNewChat && !isFetchingHistory) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" height="100vh">
        <Alert severity="error">{error || wsError}</Alert>
      </Box>
    );
  }

  if (isLoading || isFetchingHistory) {
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
              {messages.map((message, index) => (
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
                    onEditSuccess={() => {
                      fetchMessagesForSession(sessionId);
                    }}
                  />
                </Box>
              ))}
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