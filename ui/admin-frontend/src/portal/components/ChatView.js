import React, { useState, useRef, useCallback } from 'react';
import { useParams, useLocation } from 'react-router-dom';
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

  const handleMessageReceived = useCallback((data) => {
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
            // If parsing fails, use the payload directly
            content = data.payload;
          }
        }

        if (lastMessage && lastMessage.type === 'ai' && !lastMessage.isComplete && data.type === 'stream_chunk') {
          newMessages[newMessages.length - 1] = {
            ...lastMessage,
            content: lastMessage.content + content,
          };
        } else {
          newMessages.push({
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
          type: 'user',
          content: data.payload,
          isComplete: true,
        },
      ]);
    } else if (data.type === 'error') {
      setMessages((prevMessages) => [
        ...prevMessages,
        {
          type: 'system',
          content: `:::system Error: ${data.payload}:::`,
          isComplete: true,
        },
      ]);
    }
  }, []);

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

  const {
    isConnected,
    isLoading,
    sessionId,
    isNewChat,
    hasUpdatedChatName,
    setHasUpdatedChatName,
    setIsNewChat,
    sendMessage,
  } = useChatWebSocket({
    chatId,
    onMessageReceived: handleMessageReceived,
    updateChatName,
  });

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

  React.useEffect(() => {
    if (autoScroll) {
      scrollToBottom();
    }
  }, [messages, autoScroll]);

  React.useEffect(() => {
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

  React.useEffect(() => {
    const messageContainer = messageContainerRef.current;

    if (!messageContainer) return;

    const resizeObserver = new ResizeObserver(() => {
      if (autoScroll) {
        scrollToBottom();
      }
    });

    resizeObserver.observe(messageContainer);

    return () => {
      resizeObserver.unobserve(messageContainer);
    };
  }, [autoScroll]);

  React.useEffect(() => {
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
            type: 'system',
            content: ':::system Error: Failed to load databases and tools:::',
            isComplete: true,
          },
        ]);
      }
    };

    fetchData();
  }, [chatId]);

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
    [sessionId, setSnackbar]
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
                  key={index}
                  sx={{
                    width: '100%',
                    p: 2,
                    borderTop: index > 0 ? '1px solid #e0e0e0' : 'none',
                    borderBottom:
                      index === messages.length - 1 ? '1px solid #e0e0e0' : 'none',
                    opacity: message.isComplete ? 1 : 0.7,
                  }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 1 }}>
                    {message.type === 'user' ? 'You:' : 'Assistant:'}
                  </Typography>
                  <MessageContent
                    content={message.content}
                    messageIndex={index}
                    expandedGroups={expandedGroups}
                    toggleGroup={toggleGroup}
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
