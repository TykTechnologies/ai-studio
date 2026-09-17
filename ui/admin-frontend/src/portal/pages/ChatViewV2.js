import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { Alert, Box, Button, CircularProgress, Snackbar } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import pubClient from '../../admin/utils/pubClient';
import ChatSidebar from '../components/chat/ChatSidebar';
import { ChatUiProvider } from '../components/chat-v2/ChatUiContext';
import StudioChatShell from '../components/chat-v2/StudioChatShell';
import {
  chatEndpoints,
  createSession,
  addTool,
  removeTool,
  addDatasource,
  removeDatasource,
} from '../components/chat-v2/api/chatV2Client';

/**
 * Chat room page on the v2 API + assistant-ui. Owns the session (create /
 * continue / new), the tool & datasource catalogue for the sidebar and the
 * user identity; the runtime owns the conversation.
 */
const ChatViewV2 = () => {
  const { chatId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const continueId = new URLSearchParams(location.search).get('continue_id');
  const endpoints = useMemo(() => chatEndpoints(chatId), [chatId]);

  const [session, setSession] = useState(null);
  const [loading, setLoading] = useState(true);
  const [fatal, setFatal] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'error' });
  const [tools, setTools] = useState([]);
  const [databases, setDatabases] = useState([]);
  const [userName, setUserName] = useState('');

  const notify = useCallback((message, severity = 'error') => setSnackbar({ open: true, message, severity }), []);

  const applySelection = useCallback((sess) => {
    const toolIds = new Set((sess?.tools || []).map((t) => String(t.id)));
    const dsIds = new Set((sess?.datasources || []).map((d) => String(d.id)));
    setTools((prev) => prev.map((t) => ({ ...t, isSelected: toolIds.has(t.id) })));
    setDatabases((prev) => prev.map((d) => ({ ...d, isSelected: dsIds.has(d.id) })));
  }, []);

  // React StrictMode runs effects twice in development; without this guard
  // two sessions would be created and one abandoned in the hub.
  const inFlightRef = useRef(null);

  const startSession = useCallback(
    async (resumeId) => {
      const key = `${chatId}:${resumeId || ''}`;
      if (inFlightRef.current === key) return;
      inFlightRef.current = key;
      setLoading(true);
      setFatal(null);
      try {
        const sess = await createSession(endpoints, resumeId);
        setSession(sess);
        applySelection(sess);
        try {
          window.history.replaceState({}, '', `/chat/${chatId}?continue_id=${sess.session_id}`);
        } catch (e) {
          // ignore
        }
      } catch (err) {
        const detail = err.response?.data?.errors?.[0]?.detail || err.message;
        setFatal(resumeId ? `Could not continue this conversation: ${detail}` : `Could not start a chat session: ${detail}`);
      } finally {
        if (inFlightRef.current === key) inFlightRef.current = null;
        setLoading(false);
      }
    },
    [chatId, endpoints, applySelection],
  );

  // Catalogue + identity (same sources as the v1 page).
  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const cached = localStorage.getItem('userData');
        if (cached) {
          setUserName(JSON.parse(cached).name || '');
        } else {
          const me = await pubClient.get('/common/me');
          setUserName(me.data.attributes?.name || '');
          localStorage.setItem('userData', JSON.stringify(me.data.attributes));
        }
        const [dsRes, toolRes] = await Promise.all([
          pubClient.get('/common/accessible-datasources'),
          pubClient.get('/common/accessible-tools'),
        ]);
        if (cancelled) return;
        setDatabases(
          dsRes.data.map((db) => ({
            id: db.id.toString(),
            name: db.attributes.name,
            type: 'database',
            description: db.attributes.short_description,
            icon: db.attributes.icon,
            isSelected: false,
          })),
        );
        setTools(
          toolRes.data.map((tool) => ({
            id: tool.id.toString(),
            name: tool.attributes.name,
            type: 'tool',
            description: tool.attributes.description,
            toolType: tool.attributes.tool_type,
            isSelected: false,
          })),
        );
      } catch (err) {
        if (!cancelled) notify('Failed to load databases and tools');
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, [chatId, notify]);

  useEffect(() => {
    startSession(continueId || undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chatId, continueId]);

  // Re-apply the selection once the catalogue arrives after the session.
  useEffect(() => {
    if (session) applySelection(session);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.session_id, tools.length, databases.length]);

  const handleToggle = useCallback(
    async (item, selected) => {
      if (!session) return;
      try {
        if (item.type === 'tool') {
          const res = await (selected ? addTool(session.session_id, item.id) : removeTool(session.session_id, item.id));
          setTools((prev) => prev.map((t) => (t.id === item.id ? { ...t, isSelected: selected } : t)));
          // A client tool (generative UI, form, approval) needs its renderer
          // and its human-tool registration as soon as it joins the session.
          const clientTools = res?.data?.client_tools;
          if (Array.isArray(clientTools)) {
            setSession((prev) => (prev && prev.session_id === session.session_id ? { ...prev, client_tools: clientTools } : prev));
          }
        } else {
          await (selected ? addDatasource(session.session_id, item.id) : removeDatasource(session.session_id, item.id));
          setDatabases((prev) => prev.map((d) => (d.id === item.id ? { ...d, isSelected: selected } : d)));
        }
      } catch (err) {
        notify(err.response?.data?.errors?.[0]?.detail || `Failed to ${selected ? 'add' : 'remove'} ${item.name}`);
      }
    },
    [session, notify],
  );

  const handleNewChat = useCallback(() => {
    setSession(null);
    navigate(`/chat/${chatId}`, { replace: true });
    startSession(undefined);
  }, [chatId, navigate, startSession]);

  const handleRunError = useCallback((err) => notify(err?.message || 'The request failed'), [notify]);

  if (fatal) {
    return (
      <Box display="flex" flexDirection="column" alignItems="center" justifyContent="center" height="100vh" gap={2}>
        <Alert severity="error" sx={{ maxWidth: 600 }}>{fatal}</Alert>
        <Button variant="outlined" startIcon={<AddIcon />} onClick={handleNewChat}>Start a new chat</Button>
      </Box>
    );
  }

  if (loading || !session) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" height="100vh">
        <CircularProgress />
      </Box>
    );
  }

  const showTools = session.chat?.tool_support !== false;

  return (
    <ChatUiProvider session={session} userName={userName}>
      <StudioChatShell
        key={session.session_id}
        session={session}
        endpoints={endpoints}
        onRunError={handleRunError}
        title={session.chat?.name}
        onNewChat={handleNewChat}
        sidebar={({ printMessages }) => (
          <ChatSidebar
            currentlyUsing={[]}
            databases={databases}
            tools={tools}
            showTools={showTools}
            removeFromCurrentlyUsing={(item) => handleToggle(item, false)}
            addToCurrentlyUsing={(item) => handleToggle(item, true)}
            messages={printMessages}
            roomName={session.chat?.name}
          />
        )}
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

export default ChatViewV2;
