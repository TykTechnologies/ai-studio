import React, { useState, useEffect, useRef } from 'react';
import { useNavigate, useParams, Link as RouterLink } from 'react-router-dom';
import {
  Box,
  Typography,
  CircularProgress,
  Alert,
  Card,
  CardContent,
  Chip,
  Grid,
  Divider,
  List,
  ListItem,
  ListItemText,
  TextField,
  Paper,
  Collapse,
  IconButton,
} from '@mui/material';
import {
  Edit as EditIcon,
  Delete as DeleteIcon,
  Send as SendIcon,
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material';
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryOutlineButton,
  DangerButton,
} from '../../styles/sharedStyles';
import ConfirmationDialog from '../common/ConfirmationDialog';
import agentService from '../../services/agentService';

const AgentDetail = () => {
  const navigate = useNavigate();
  const { id } = useParams();
  const [agent, setAgent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  // Test interface state
  const [testExpanded, setTestExpanded] = useState(false);
  const [testMessage, setTestMessage] = useState('');
  const [testMessages, setTestMessages] = useState([]);
  const [testLoading, setTestLoading] = useState(false);
  const [testSessionId, setTestSessionId] = useState('');
  const [testConnected, setTestConnected] = useState(false);
  const testMessagesEndRef = useRef(null);
  const eventSourceRef = useRef(null);

  useEffect(() => {
    loadAgent();
  }, [id]);

  useEffect(() => {
    scrollToBottom();
  }, [testMessages]);

  // Establish SSE connection when test interface is expanded
  useEffect(() => {
    if (testExpanded && agent?.isActive && !eventSourceRef.current) {
      establishTestConnection();
    } else if (!testExpanded && eventSourceRef.current) {
      // Clean up connection when collapsed
      eventSourceRef.current.close();
      eventSourceRef.current = null;
      setTestConnected(false);
      setTestSessionId('');
    }
    
    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
    };
  }, [testExpanded, agent]);

  const scrollToBottom = () => {
    testMessagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  const establishTestConnection = async () => {
    try {
      const baseURL = '/api/v1';
      
      // Create SSE connection to establish session
      // Admin uses cookie auth, no need for token in URL
      const eventSource = new EventSource(`${baseURL}/agents/${id}/stream`);
      eventSourceRef.current = eventSource;
      
      eventSource.addEventListener('session', (event) => {
        const data = JSON.parse(event.data);
        setTestSessionId(data.session_id);
        setTestConnected(true);
        console.log('Test session established:', data.session_id);
        
        // Add system message
        setTestMessages(prev => [...prev, {
          type: 'system',
          content: `Connected to agent. Session ID: ${data.session_id}`,
        }]);
      });
      
      // Handle agent responses
      eventSource.addEventListener('content', (event) => {
        const data = JSON.parse(event.data);
        setTestMessages(prev => {
          const newMessages = [...prev];
          const lastMsg = newMessages[newMessages.length - 1];
          
          if (lastMsg && lastMsg.type === 'agent' && !lastMsg.is_final) {
            lastMsg.content += data.content || '';
            lastMsg.is_final = data.is_final;
            return newMessages;
          }
          
          return [...newMessages, {
            type: 'agent',
            content: data.content || '',
            metadata: data.metadata,
            is_final: data.is_final,
          }];
        });
        
        setTestLoading(false);
      });
      
      eventSource.addEventListener('done', (event) => {
        setTestLoading(false);
      });
      
      eventSource.addEventListener('error', (event) => {
        const data = JSON.parse(event.data);
        setTestMessages(prev => [...prev, {
          type: 'error',
          content: data.content || 'Unknown error',
        }]);
        setTestLoading(false);
      });
      
      eventSource.onerror = (error) => {
        console.error('SSE connection error:', error);
        setTestConnected(false);
        setTestSessionId('');
        setTestMessages(prev => [...prev, {
          type: 'error',
          content: 'Connection lost. Please refresh and try again.',
        }]);
      };
      
    } catch (err) {
      console.error('Error establishing test connection:', err);
      setTestMessages(prev => [...prev, {
        type: 'error',
        content: `Failed to connect: ${err.message}`,
      }]);
    }
  };

  const loadAgent = async () => {
    try {
      setLoading(true);
      const data = await agentService.getAgent(id);
      setAgent(data);
    } catch (err) {
      console.error('Error loading agent:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleEdit = () => {
    navigate(`/admin/agents/edit/${id}`);
  };

  const handleDelete = async () => {
    try {
      await agentService.deleteAgent(id);
      navigate('/admin/agents');
    } catch (err) {
      setError(err.message);
    }
  };

  const handleToggleActive = async () => {
    try {
      if (agent.isActive) {
        await agentService.deactivateAgent(id);
      } else {
        await agentService.activateAgent(id);
      }
      loadAgent();
    } catch (err) {
      setError(err.message);
    }
  };

  const handleSendTestMessage = async () => {
    if (!testMessage.trim()) return;
    
    if (!testConnected || !testSessionId) {
      setTestMessages(prev => [...prev, {
        type: 'error',
        content: 'Not connected to agent. Please wait for connection to establish.',
      }]);
      return;
    }

    const userMessage = testMessage;
    setTestMessage('');
    setTestLoading(true);

    // Add user message
    setTestMessages(prev => [...prev, {
      type: 'user',
      content: userMessage,
    }]);

    try {
      const baseURL = '/api/v1';

      // Send message with session ID as query parameter
      // Admin uses cookie auth, no Authorization header needed
      const response = await fetch(`${baseURL}/agents/${id}/message?session_id=${testSessionId}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include', // Include cookies for auth
        body: JSON.stringify({
          message: userMessage,
          history: [],
        }),
      });

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData?.errors?.[0]?.detail || 'Failed to send message');
      }

      // Response will come through the SSE connection, not here
      // The content/done/error events will update the messages
      
    } catch (err) {
      console.error('Error sending test message:', err);
      setTestMessages(prev => [...prev, {
        type: 'error',
        content: err.message,
      }]);
      setTestLoading(false);
    }
  };

  const handleTestChunk = (chunk) => {
    // Handle different chunk types
    setTestMessages(prev => {
      const newMessages = [...prev];

      // If last message is same type and not complete, append to it
      const lastMsg = newMessages[newMessages.length - 1];
      if (lastMsg && lastMsg.type === chunk.type && !lastMsg.is_final) {
        lastMsg.content += chunk.content;
        lastMsg.is_final = chunk.is_final;
        return newMessages;
      }

      // Otherwise add new message
      return [...newMessages, {
        type: chunk.type,
        content: chunk.content,
        metadata: chunk.metadata,
        is_final: chunk.is_final,
      }];
    });
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (!agent) {
    return (
      <ContentBox>
        <Alert severity="error">Agent not found</Alert>
      </ContentBox>
    );
  }

  return (
    <>
      <TitleBox>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <SmartToyIcon fontSize="large" color="primary" />
          <Typography variant="headingXLarge">{agent.name}</Typography>
          <Chip
            label={agent.isActive ? 'Active' : 'Inactive'}
            color={agent.isActive ? 'success' : 'default'}
          />
        </Box>
        <Box sx={{ display: 'flex', gap: 2 }}>
          <SecondaryOutlineButton
            startIcon={<EditIcon />}
            onClick={handleEdit}
          >
            Edit
          </SecondaryOutlineButton>
          <SecondaryOutlineButton onClick={handleToggleActive}>
            {agent.isActive ? 'Deactivate' : 'Activate'}
          </SecondaryOutlineButton>
          <DangerButton
            startIcon={<DeleteIcon />}
            onClick={() => setDeleteDialogOpen(true)}
          >
            Delete
          </DangerButton>
        </Box>
      </TitleBox>

      <ContentBox>
        {error && (
          <Alert severity="error" sx={{ mb: 3 }}>
            {error}
          </Alert>
        )}

        <Grid container spacing={3}>
          {/* Overview */}
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="headingMedium" gutterBottom>
                  Overview
                </Typography>
                <Divider sx={{ my: 2 }} />
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <Box>
                    <Typography variant="bodySmallDefault" color="text.secondary">
                      Description
                    </Typography>
                    <Typography variant="bodyMedium">
                      {agent.description || 'No description'}
                    </Typography>
                  </Box>
                  <Box>
                    <Typography variant="bodySmallDefault" color="text.secondary">
                      Slug
                    </Typography>
                    <Typography variant="bodyMedium">{agent.slug}</Typography>
                  </Box>
                  <Box>
                    <Typography variant="bodySmallDefault" color="text.secondary">
                      Namespace
                    </Typography>
                    <Typography variant="bodyMedium">
                      {agent.namespace || 'Global'}
                    </Typography>
                  </Box>
                  <Box>
                    <Typography variant="bodySmallDefault" color="text.secondary">
                      Created
                    </Typography>
                    <Typography variant="bodyMedium">
                      {new Date(agent.createdAt).toLocaleString()}
                    </Typography>
                  </Box>
                </Box>
              </CardContent>
            </Card>
          </Grid>

          {/* Configuration */}
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="headingMedium" gutterBottom>
                  Configuration
                </Typography>
                <Divider sx={{ my: 2 }} />
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <Box>
                    <Typography variant="bodySmallDefault" color="text.secondary">
                      Plugin
                    </Typography>
                    <Typography
                      variant="bodyMedium"
                      component={RouterLink}
                      to={`/admin/plugins/${agent.plugin?.id}`}
                      sx={{ textDecoration: 'none', color: 'primary.main' }}
                    >
                      {agent.plugin?.name || 'Unknown Plugin'}
                    </Typography>
                  </Box>
                  <Box>
                    <Typography variant="bodySmallDefault" color="text.secondary">
                      App
                    </Typography>
                    <Typography
                      variant="bodyMedium"
                      component={RouterLink}
                      to={`/admin/apps/${agent.app?.id}`}
                      sx={{ textDecoration: 'none', color: 'primary.main' }}
                    >
                      {agent.app?.name || 'Unknown App'}
                    </Typography>
                  </Box>
                  <Box>
                    <Typography variant="bodySmallDefault" color="text.secondary">
                      Plugin Config
                    </Typography>
                    <Paper
                      sx={{
                        p: 1,
                        bgcolor: 'background.default',
                        fontFamily: 'monospace',
                        fontSize: '0.75rem',
                        maxHeight: 200,
                        overflow: 'auto',
                      }}
                    >
                      <pre style={{ margin: 0 }}>
                        {JSON.stringify(agent.config, null, 2)}
                      </pre>
                    </Paper>
                  </Box>
                </Box>
              </CardContent>
            </Card>
          </Grid>

          {/* Resources from App */}
          <Grid item xs={12}>
            <Card>
              <CardContent>
                <Typography variant="headingMedium" gutterBottom>
                  Available Resources (from App)
                </Typography>
                <Divider sx={{ my: 2 }} />
                <Grid container spacing={2}>
                  <Grid item xs={12} md={4}>
                    <Typography variant="bodyMedium" gutterBottom>
                      LLMs ({agent.app?.llms?.length || 0})
                    </Typography>
                    <List dense>
                      {agent.app?.llms?.map((llm) => (
                        <ListItem key={llm.id}>
                          <ListItemText
                            primary={llm.name}
                            secondary={llm.vendor}
                          />
                        </ListItem>
                      ))}
                    </List>
                  </Grid>
                  <Grid item xs={12} md={4}>
                    <Typography variant="bodyMedium" gutterBottom>
                      Tools ({agent.app?.tools?.length || 0})
                    </Typography>
                    <List dense>
                      {agent.app?.tools?.map((tool) => (
                        <ListItem key={tool.id}>
                          <ListItemText primary={tool.name} />
                        </ListItem>
                      ))}
                    </List>
                  </Grid>
                  <Grid item xs={12} md={4}>
                    <Typography variant="bodyMedium" gutterBottom>
                      Datasources ({agent.app?.datasources?.length || 0})
                    </Typography>
                    <List dense>
                      {agent.app?.datasources?.map((ds) => (
                        <ListItem key={ds.id}>
                          <ListItemText primary={ds.name} />
                        </ListItem>
                      ))}
                    </List>
                  </Grid>
                </Grid>
              </CardContent>
            </Card>
          </Grid>

          {/* Access Control */}
          <Grid item xs={12}>
            <Card>
              <CardContent>
                <Typography variant="headingMedium" gutterBottom>
                  Access Control
                </Typography>
                <Divider sx={{ my: 2 }} />
                <Box>
                  <Typography variant="bodySmallDefault" color="text.secondary" gutterBottom>
                    Groups
                  </Typography>
                  {agent.groups.length === 0 ? (
                    <Chip label="Public (All Users)" />
                  ) : (
                    <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                      {agent.groups.map((group) => (
                        <Chip key={group.id} label={group.name} />
                      ))}
                    </Box>
                  )}
                </Box>
              </CardContent>
            </Card>
          </Grid>

          {/* Test Interface */}
          <Grid item xs={12}>
            <Card>
              <CardContent>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <Typography variant="headingMedium">
                    Test Interface
                  </Typography>
                  <IconButton onClick={() => setTestExpanded(!testExpanded)}>
                    {testExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                  </IconButton>
                </Box>

                <Collapse in={testExpanded}>
                  <Divider sx={{ my: 2 }} />

                  {/* Messages */}
                  <Paper
                    sx={{
                      p: 2,
                      mb: 2,
                      maxHeight: 400,
                      overflow: 'auto',
                      bgcolor: 'background.default',
                    }}
                  >
                    {testMessages.length === 0 ? (
                      <Typography variant="bodySmallDefault" color="text.secondary">
                        Send a message to test the agent
                      </Typography>
                    ) : (
                      testMessages.map((msg, idx) => (
                        <Box
                          key={idx}
                          sx={{
                            mb: 2,
                            p: 1,
                            borderRadius: 1,
                            bgcolor: msg.type === 'user' ? 'primary.light' : 'background.paper',
                          }}
                        >
                          <Typography variant="bodySmallDefault" color="text.secondary">
                            {msg.type.toUpperCase()}
                          </Typography>
                          <Typography variant="bodyMedium" sx={{ whiteSpace: 'pre-wrap' }}>
                            {msg.content}
                          </Typography>
                        </Box>
                      ))
                    )}
                    <div ref={testMessagesEndRef} />
                  </Paper>

                  {/* Input */}
                  <Box sx={{ display: 'flex', gap: 2 }}>
                    <TextField
                      fullWidth
                      placeholder="Type a message..."
                      value={testMessage}
                      onChange={(e) => setTestMessage(e.target.value)}
                      onKeyPress={(e) => {
                        if (e.key === 'Enter' && !e.shiftKey) {
                          e.preventDefault();
                          handleSendTestMessage();
                        }
                      }}
                      disabled={testLoading || !agent.isActive}
                    />
                    <PrimaryButton
                      startIcon={<SendIcon />}
                      onClick={handleSendTestMessage}
                      disabled={testLoading || !agent.isActive || !testMessage.trim()}
                    >
                      Send
                    </PrimaryButton>
                  </Box>
                </Collapse>
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      </ContentBox>

      <ConfirmationDialog
        open={deleteDialogOpen}
        title="Delete Agent"
        message={`Are you sure you want to delete the agent "${agent.name}"? This action cannot be undone.`}
        onConfirm={handleDelete}
        onCancel={() => setDeleteDialogOpen(false)}
      />
    </>
  );
};

export default AgentDetail;
