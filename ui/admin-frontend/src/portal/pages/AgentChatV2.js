import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Alert, Box, Button, CircularProgress, IconButton, Snackbar } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import agentService from '../services/agentService';
import { ChatUiProvider } from '../components/chat-v2/ChatUiContext';
import StudioChatShell from '../components/chat-v2/StudioChatShell';
import { agentEndpoints, createSession } from '../components/chat-v2/api/chatV2Client';

/**
 * Plugin agent chat on the v2 API + assistant-ui. Same shell as the chat
 * room page without the tools/datasources sidebar or file uploads; agent
 * sessions keep an in-memory transcript for their lifetime.
 */
const AgentChatV2 = () => {
  const { agentId } = useParams();
  const navigate = useNavigate();
  const endpoints = useMemo(() => agentEndpoints(agentId), [agentId]);

  const [agent, setAgent] = useState(null);
  const [session, setSession] = useState(null);
  const [loading, setLoading] = useState(true);
  const [fatal, setFatal] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'error' });
  const [userName, setUserName] = useState('');

  const notify = useCallback((message, severity = 'error') => setSnackbar({ open: true, message, severity }), []);
  const inFlightRef = useRef(false);

  const startSession = useCallback(async () => {
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    setLoading(true);
    setFatal(null);
    try {
      const sess = await createSession(endpoints);
      setSession(sess);
    } catch (err) {
      const detail = err.response?.data?.errors?.[0]?.detail || err.message;
      setFatal(`Failed to connect to agent: ${detail}`);
    } finally {
      inFlightRef.current = false;
      setLoading(false);
    }
  }, [endpoints]);

  useEffect(() => {
    let cancelled = false;
    try {
      const cached = localStorage.getItem('userData');
      if (cached) setUserName(JSON.parse(cached).name || '');
    } catch (e) {
      // ignore
    }
    const load = async () => {
      try {
        const data = await agentService.getAgent(agentId);
        if (cancelled) return;
        if (!data) {
          setFatal('Agent not found');
          setLoading(false);
          return;
        }
        if (!data.isActive) {
          setFatal('This agent is currently inactive');
          setLoading(false);
          return;
        }
        setAgent(data);
        await startSession();
      } catch (err) {
        if (!cancelled) {
          setFatal(err.message);
          setLoading(false);
        }
      }
    };
    load();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId]);

  const handleNewChat = useCallback(() => {
    setSession(null);
    startSession();
  }, [startSession]);

  const handleRunError = useCallback((err) => notify(err?.message || 'The request failed'), [notify]);
  const handleBack = useCallback(() => navigate('/chat/agents'), [navigate]);

  if (fatal) {
    return (
      <Box sx={{ p: 4 }}>
        <Alert severity="error">{fatal}</Alert>
        <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
          <IconButton onClick={handleBack} aria-label="Back"><ArrowBackIcon /></IconButton>
          <Button variant="outlined" startIcon={<AddIcon />} onClick={handleNewChat}>Try again</Button>
        </Box>
      </Box>
    );
  }

  if (loading || !session || !agent) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <ChatUiProvider session={session} userName={userName}>
      <StudioChatShell
        key={session.session_id}
        session={session}
        endpoints={endpoints}
        onRunError={handleRunError}
        title={agent.name}
        subtitle={agent.description}
        icon={<SmartToyIcon fontSize="large" color="primary" />}
        onBack={handleBack}
        onNewChat={handleNewChat}
        hideFileUpload
      />
      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar((s) => ({ ...s, open: false }))}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert onClose={() => setSnackbar((s) => ({ ...s, open: false }))} severity={snackbar.severity} sx={{ width: '100%' }}>
          {snackbar.message}
        </Alert>
      </Snackbar>
    </ChatUiProvider>
  );
};

export default AgentChatV2;
