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

  const messagesEndRef = useRef(null);
  const readerRef = useRef(null);

  useEffect(() => {
    loadAgent();
  }, [agentId]);

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  // Cleanup reader on unmount
  useEffect(() => {
    return () => {
      if (readerRef.current) {
        readerRef.current.cancel();
      }
    };
  }, []);

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

  const handleSendMessage = async () => {
    if (!inputMessage.trim() || sending) return;

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
      // Send message and get SSE stream
      const stream = await agentService.sendMessage(
        agentId,
        userMessage,
        [], // history - could be built from messages
        sessionId
      );

      const reader = stream.getReader();
      readerRef.current = reader;
      const decoder = new TextDecoder();

      let buffer = '';
      let currentAgentMessage = null;

      while (true) {
        const { done, value } = await reader.read();

        if (done) {
          break;
        }

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop(); // Keep incomplete line in buffer

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6));

              // Handle session message
              if (data.session_id) {
                setSessionId(data.session_id);
                continue;
              }

              // Process chunk
              const chunkType = data.type?.toLowerCase() || 'content';

              if (currentAgentMessage && currentAgentMessage.type === chunkType && !data.is_final) {
                // Append to existing message
                setMessages(prev => {
                  const updated = [...prev];
                  const lastMsg = updated[updated.length - 1];
                  if (lastMsg.id === currentAgentMessage.id) {
                    lastMsg.content += data.content || '';
                    lastMsg.metadata = data.metadata || lastMsg.metadata;
                    lastMsg.is_final = data.is_final;
                  }
                  return updated;
                });
              } else {
                // Create new message
                const newMessage = {
                  id: Date.now() + Math.random(),
                  role: 'agent',
                  type: chunkType,
                  content: data.content || '',
                  metadata: data.metadata,
                  is_final: data.is_final,
                };

                currentAgentMessage = newMessage;
                setMessages(prev => [...prev, newMessage]);
              }

            } catch (err) {
              console.error('Error parsing SSE data:', err, line);
            }
          }
        }
      }

      readerRef.current = null;
    } catch (err) {
      console.error('Error sending message:', err);
      setMessages(prev => [...prev, {
        id: Date.now(),
        role: 'agent',
        type: 'error',
        content: `Error: ${err.message}`,
      }]);
    } finally {
      setSending(false);
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
          <ChatInput
            value={inputMessage}
            onChange={setInputMessage}
            onSend={handleSendMessage}
            disabled={sending}
            placeholder="Type your message..."
          />
        </Box>
      </Paper>
    </Box>
  );
};

export default AgentChat;
