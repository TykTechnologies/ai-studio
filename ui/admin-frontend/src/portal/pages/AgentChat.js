import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box,
  Typography,
  CircularProgress,
  Alert,
  IconButton,
  Paper,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import SmartToyIcon from '@mui/icons-material/SmartToy';
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

  const messagesEndRef = useRef(null);
  const eventSourceRef = useRef(null);
  const currentAgentMessageRef = useRef(null);

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
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      {/* Header */}
      <TitleBox top="64px">
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <IconButton onClick={handleBack} size="small">
            <ArrowBackIcon />
          </IconButton>
          <SmartToyIcon fontSize="large" color="primary" />
          <Box>
            <Typography variant="headingXLarge">{agent.name}</Typography>
            {agent.description && (
              <Typography variant="bodySmallDefault" color="text.secondary">
                {agent.description}
              </Typography>
            )}
          </Box>
        </Box>
      </TitleBox>

      {/* Messages */}
      <Box
        sx={{
          flex: 1,
          overflow: 'auto',
          p: 3,
          pt: 10,
        }}
      >
        {messages.length === 0 ? (
          <Box sx={{ textAlign: 'center', mt: 8 }}>
            <SmartToyIcon sx={{ fontSize: 64, color: 'text.secondary', mb: 2 }} />
            <Typography variant="headingLarge" gutterBottom>
              Start a conversation
            </Typography>
            <Typography variant="bodyLargeDefault" color="text.secondary">
              Send a message to begin
            </Typography>
          </Box>
        ) : (
          <Box sx={{ maxWidth: 800, margin: '0 auto' }}>
            {messages.map((message) => (
              <AgentMessage key={message.id} message={message} />
            ))}
            {sending && (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, my: 2 }}>
                <CircularProgress size={20} />
                <Typography variant="bodySmallDefault" color="text.secondary">
                  Agent is thinking...
                </Typography>
              </Box>
            )}
            <div ref={messagesEndRef} />
          </Box>
        )}
      </Box>

      {/* Input */}
      <Paper
        elevation={3}
        sx={{
          p: 2,
          borderTop: (theme) => `1px solid ${theme.palette.divider}`,
        }}
      >
        <Box sx={{ maxWidth: 800, margin: '0 auto' }}>
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
            />
          )}
        </Box>
      </Paper>
    </Box>
  );
};

export default AgentChat;
