import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { Alert, Box, Button, CircularProgress, Grid, Snackbar, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import PrintIcon from '@mui/icons-material/Print';
import { AssistantRuntimeProvider, useAuiState } from '@assistant-ui/react';
import { TitleBox } from '../../admin/styles/sharedStyles';
import pubClient from '../../admin/utils/pubClient';
import ChatSidebar from '../components/chat/ChatSidebar';
import usePrintChat from '../components/chat/hooks/usePrintChat';
import '../components/chat/printChat.css';
import { ChatUiProvider } from '../components/chat-v2/ChatUiContext';
import StudioThread from '../components/chat-v2/StudioThread';
import { useStudioRuntime } from '../components/chat-v2/runtime/useStudioRuntime';
import {
  createSession,
  addTool,
  removeTool,
  addDatasource,
  removeDatasource,
} from '../components/chat-v2/api/chatV2Client';

/** Everything that needs the runtime: toolbar, thread and sidebar. */
const StudioChat = ({ session, chatId, onNewChat, onError, tools, databases, onToggle, showTools }) => {
  const viewportRef = useRef(null);
  const runtime = useStudioRuntime({ sessionId: session.session_id, onRunError: onError });

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <StudioChatLayout
        session={session}
        chatId={chatId}
        onNewChat={onNewChat}
        tools={tools}
        databases={databases}
        onToggle={onToggle}
        showTools={showTools}
        viewportRef={viewportRef}
      />
    </AssistantRuntimeProvider>
  );
};

const StudioChatLayout = ({ session, onNewChat, tools, databases, onToggle, showTools, viewportRef }) => {
  const isEmpty = useAuiState((s) => s.thread.isEmpty);
  // usePrintChat only inspects the array for a user turn.
  const printMessages = useMemo(() => (isEmpty ? [] : [{ role: 'user' }]), [isEmpty]);
  const { handlePrint, canPrint } = usePrintChat({
    chatName: session.chat?.name,
    messages: printMessages,
    messagesContainerRef: viewportRef,
  });

  return (
    <>
      <TitleBox top="64px" data-print-role="toolbar">
        <Typography variant="headingXLarge">{session.chat?.name}</Typography>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            variant="outlined"
            startIcon={<PrintIcon />}
            onClick={handlePrint}
            disabled={!canPrint}
            title="Opens your browser's print dialog, where you can save the conversation as a PDF"
          >
            Print
          </Button>
          <Button variant="outlined" startIcon={<AddIcon />} onClick={onNewChat}>
            New Chat
          </Button>
        </Box>
      </TitleBox>
      <Box sx={{ height: '85vh', display: 'flex', flexDirection: 'column' }} data-print-role="chat-outer">
        <Grid container sx={{ flexGrow: 1, overflow: 'hidden', mb: 4 }}>
          <Grid item xs={9} sx={{ height: '100%' }}>
            <StudioThread ref={viewportRef} />
          </Grid>
          <Grid item xs={3} sx={{ height: '100%', overflowY: 'auto' }} data-print-role="sidebar">
            <ChatSidebar
              currentlyUsing={[]}
              databases={databases}
              tools={tools}
              showTools={showTools}
              removeFromCurrentlyUsing={(item) => onToggle(item, false)}
              addToCurrentlyUsing={(item) => onToggle(item, true)}
              messages={printMessages}
              roomName={session.chat?.name}
            />
          </Grid>
        </Grid>
      </Box>
    </>
  );
};

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
        const sess = await createSession(chatId, resumeId);
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
    [chatId, applySelection],
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
          await (selected ? addTool(session.session_id, item.id) : removeTool(session.session_id, item.id));
          setTools((prev) => prev.map((t) => (t.id === item.id ? { ...t, isSelected: selected } : t)));
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

  return (
    <ChatUiProvider session={session} userName={userName}>
      <StudioChat
        key={session.session_id}
        session={session}
        chatId={chatId}
        onNewChat={handleNewChat}
        onError={handleRunError}
        tools={tools}
        databases={databases}
        onToggle={handleToggle}
        showTools={session.chat?.tool_support !== false}
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
