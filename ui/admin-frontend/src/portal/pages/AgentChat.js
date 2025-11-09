import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box,
  Typography,
  CircularProgress,
  Alert,
  IconButton,
  Grid,
  Button,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import AddIcon from '@mui/icons-material/Add';
import { TitleBox } from '../../admin/styles/sharedStyles';
import agentService from '../services/agentService';
import ChatInput from '../components/chat/ChatInput';
import AgentMessage from '../components/agents/AgentMessage';

const AgentChat = () => {
  const { agentId } = useParams();
  const navigate = useNavigate();

  const [agent, setAgent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [messages, setMessages] = useState([]);
  const [inputMessage, setInputMessage] = useState('');
  const [sending, setSending] = useState(false);
  const [sessionId, setSessionId] = useState('');
  const [isConnected, setIsConnected] = useState(false);
  const [showSystemMessages, setShowSystemMessages] = useState(() => {
    const saved = localStorage.getItem('showSystemMessages');
    return saved !== null ? JSON.parse(saved) : true;
  });

  const messagesEndRef = useRef(null);
  const eventSourceRef = useRef(null);
  const currentAgentMessageRef = useRef(null);

  const hasUserMessages = useMemo(() =>
    messages.some(message => message.role === 'user'),
    [messages]
  );

  useEffect(() => {
    loadAgent();
  }, [agentId]);

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  // Establish SSE connection when agent is loaded
  useEffect(() => {
    if (agent && agent.isActive && !eventSourceRef.current) {
      connectToAgent();
    }

    // Cleanup on unmount or agent change
    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
    };
  }, [agent]);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  const loadAgent = async () => {
    try {
      setLoading(true);
      const data = await agentService.getAgent(agentId);

      if (!data) {
        setError('Agent not found');
        return;
      }

      if (!data.isActive) {
        setError('This agent is currently inactive');
        return;
      }

      setAgent(data);
    } catch (err) {
      console.error('Error loading agent:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const connectToAgent = async () => {
    try {
      const eventSource = await agentService.connectToAgent(agentId, sessionId);
      eventSourceRef.current = eventSource;

      // Handle session establishment
      eventSource.addEventListener('session', (event) => {
        const data = JSON.parse(event.data);
        setSessionId(data.session_id);
        setIsConnected(true);
        console.log('Agent session established:', data.session_id);
      });

      // Handle content messages - aligned with admin test interface pattern
      eventSource.addEventListener('content', (event) => {
        const data = JSON.parse(event.data);

        setMessages(prev => {
          const newMessages = [...prev];
          const lastMsg = newMessages[newMessages.length - 1];

          // If last message is an agent content message and not final, append to it
          if (lastMsg && lastMsg.type === 'content' && lastMsg.role === 'agent' && !lastMsg.is_final) {
            lastMsg.content += data.content || '';
            lastMsg.is_final = data.is_final;
            lastMsg.metadata = data.metadata || lastMsg.metadata;
            return newMessages;
          }

          // Otherwise create a new message
          return [...newMessages, {
            id: Date.now() + Math.random(),
            role: 'agent',
            type: 'content',
            content: data.content || '',
            metadata: data.metadata,
            is_final: data.is_final,
          }];
        });

        setSending(false);
      });

      // Handle other event types (excluding 'done' which is just a completion signal)
      ['tool_call', 'tool_result', 'thinking', 'error'].forEach(eventType => {
        eventSource.addEventListener(eventType, (event) => {
          const data = JSON.parse(event.data);
          const newMessage = {
            id: Date.now() + Math.random(),
            role: 'agent',
            type: eventType,
            content: data.content || '',
            metadata: data.metadata,
            is_final: data.is_final,
          };
          setMessages(prev => [...prev, newMessage]);
        });
      });

      // Handle 'done' event separately - just update state, don't show message
      eventSource.addEventListener('done', (event) => {
        setSending(false);
      });

      eventSource.onerror = (error) => {
        console.error('SSE connection error:', error);
        setIsConnected(false);
        if (eventSource.readyState === EventSource.CLOSED) {
          eventSourceRef.current = null;
        }
      };

    } catch (err) {
      console.error('Error connecting to agent:', err);
      setError(`Failed to connect to agent: ${err.message}`);
    }
  };

  const handleSendMessage = async () => {
    console.log('handleSendMessage - sessionId:', sessionId);
    if (!inputMessage.trim() || sending || !sessionId) {
      console.log('Cannot send - inputMessage:', inputMessage.trim(), 'sending:', sending, 'sessionId:', sessionId);
      return;
    }

    const userMessage = inputMessage.trim();
    setInputMessage('');
    setSending(true);

    // Add user message to display
    setMessages(prev => [...prev, {
      id: Date.now(),
      role: 'user',
      content: userMessage,
    }]);

    try {
      console.log('Sending message with sessionId:', sessionId);
      // Just send the message - response will come through the SSE connection
      await agentService.sendMessage(
        agentId,
        userMessage,
        [], // history - could be built from messages
        sessionId
      );

      // Response will be handled by the SSE event listeners
      // setSending(false) will be called when 'done' event is received
      
    } catch (err) {
      console.error('Error sending message:', err);
      setSending(false);
      setMessages(prev => [...prev, {
        id: Date.now(),
        role: 'agent',
        type: 'error',
        content: `Error: ${err.message}`,
      }]);
    }
  };

  const handleBack = () => {
    navigate('/agents');
  };

  const handleNewChat = () => {
    // Close SSE connection
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }

    // Clear state
    setMessages([]);
    setInputMessage('');
    setSessionId('');
    setIsConnected(false);
    setSending(false);

    // Reconnect to agent
    connectToAgent();
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error || !agent) {
    return (
      <Box sx={{ p: 4 }}>
        <Alert severity="error">{error || 'Agent not found'}</Alert>
        <Box sx={{ mt: 2 }}>
          <IconButton onClick={handleBack}>
            <ArrowBackIcon />
          </IconButton>
        </Box>
      </Box>
    );
  }

  return (
    <>
      {/* Header */}
      <TitleBox top="64px">
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
          <IconButton onClick={handleBack} size="small">
            <ArrowBackIcon />
          </IconButton>
          <SmartToyIcon fontSize="large" color="primary" />
          <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 0.5 }}>
            <Typography variant="headingXLarge">
              {agent.name}
            </Typography>
            {agent.description && (
              <Typography variant="bodySmallDefault" color="text.secondary">
                {agent.description}
              </Typography>
            )}
          </Box>
          <Button
            variant="outlined"
            startIcon={<AddIcon />}
            onClick={handleNewChat}
          >
            New Chat
          </Button>
        </Box>
      </TitleBox>

      <Box sx={{ height: '85vh', display: 'flex', flexDirection: 'column' }}>
        <Grid container sx={{ flexGrow: 1, overflow: 'hidden', mb: 4 }}>
          <Grid item xs={12} sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
            {/* Messages container */}
            <Box
              sx={{
                flexGrow: 1,
                overflowY: 'auto',
                display: 'flex',
                flexDirection: 'column',
                width: '100%',
              }}
              ref={messagesEndRef}
            >
              <Box sx={{
                maxWidth: '740px',
                width: '100%',
                mx: 'auto',
                display: 'flex',
                flexDirection: 'column',
                flexGrow: 1,
              }}>
                {!hasUserMessages ? (
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
                        Welcome to {agent.name} chat
                      </Typography>
                      <Typography variant="headingXLargSub" mb={3}>
                        How can I help you today?
                      </Typography>
                      {agent.description && (
                        <Typography variant="bodyLargeDefault" color="text.defaultSubdued" mb={4} maxWidth="600px">
                          {agent.description}
                        </Typography>
                      )}
                    </Box>
                    <Box sx={{ width: '100%', mt: 2 }}>
                      {!isConnected ? (
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                          <CircularProgress size={20} />
                          <Typography variant="bodySmallDefault" color="text.secondary">
                            Connecting to agent...
                          </Typography>
                        </Box>
                      ) : (
                        <ChatInput
                          inputMessage={inputMessage}
                          setInputMessage={setInputMessage}
                          handleSendMessage={handleSendMessage}
                          isConnected={!sending && !!sessionId}
                          uploadedFiles={[]}
                          setUploadedFiles={() => {}}
                          onDrop={() => {}}
                          isUploading={false}
                          renderUploadIndicator={() => null}
                          chatId={agentId}
                          messages={messages}
                          hideFileUpload={true}
                        />
                      )}
                    </Box>
                  </Box>
                ) : (
                  <>
                    {/* System message toggle - reserve space even when not shown to prevent layout shift */}
                    <Box sx={{ mt: 2, textAlign: 'right', minHeight: '24px' }}>
                      {messages.length > 1 && (
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
                          {showSystemMessages ? 'Hide' : 'Show'} System Messages
                        </Typography>
                      )}
                    </Box>
                    {messages
                      .filter(msg => showSystemMessages || msg.type !== 'system')
                      .map((message) => (
                        <AgentMessage key={message.id} message={message} />
                      ))
                    }
                    {sending && (
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, my: 2 }}>
                        <CircularProgress size={20} />
                        <Typography variant="bodySmallDefault" color="text.secondary">
                          Agent is thinking...
                        </Typography>
                      </Box>
                    )}
                    <div ref={messagesEndRef} />
                  </>
                )}
              </Box>
            </Box>

            {hasUserMessages && (
              <Box sx={{
                width: '100%',
                padding: 2,
                paddingTop: 0,
              }}>
                <Box sx={{
                  maxWidth: '740px',
                  width: '100%',
                  mx: 'auto',
                }}>
                  {!isConnected ? (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                      <CircularProgress size={20} />
                      <Typography variant="bodySmallDefault" color="text.secondary">
                        Connecting to agent...
                      </Typography>
                    </Box>
                  ) : (
                    <ChatInput
                      inputMessage={inputMessage}
                      setInputMessage={setInputMessage}
                      handleSendMessage={handleSendMessage}
                      isConnected={!sending && !!sessionId}
                      uploadedFiles={[]}
                      setUploadedFiles={() => {}}
                      onDrop={() => {}}
                      isUploading={false}
                      renderUploadIndicator={() => null}
                      chatId={agentId}
                      messages={messages}
                      hideFileUpload={true}
                    />
                  )}
                </Box>
              </Box>
            )}
          </Grid>
        </Grid>
      </Box>
    </>
  );
};

export default AgentChat;
