import React, { useEffect, useMemo, useRef } from 'react';
import { Box, Button, Grid, IconButton, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import PrintIcon from '@mui/icons-material/Print';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { AssistantRuntimeProvider, useAuiState } from '@assistant-ui/react';
import { TitleBox } from '../../../admin/styles/sharedStyles';
import usePrintChat from '../chat/hooks/usePrintChat';
import '../chat/printChat.css';
import StudioThread from './StudioThread';
import { useStudioRuntime } from './runtime/useStudioRuntime';
import { loadPluginToolRenderers } from './pluginToolRenderers';

const Layout = ({ title, subtitle, icon, onBack, onNewChat, sidebar, hideFileUpload, viewportRef }) => {
  const isEmpty = useAuiState((s) => s.thread.isEmpty);
  // usePrintChat only inspects the array for a user turn.
  const printMessages = useMemo(() => (isEmpty ? [] : [{ role: 'user' }]), [isEmpty]);
  const { handlePrint, canPrint } = usePrintChat({
    chatName: title,
    messages: printMessages,
    messagesContainerRef: viewportRef,
  });

  return (
    <>
      <TitleBox top="64px" data-print-role="toolbar">
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1, minWidth: 0 }}>
          {onBack && (
            <IconButton onClick={onBack} size="small" aria-label="Back">
              <ArrowBackIcon />
            </IconButton>
          )}
          {icon}
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5, minWidth: 0 }}>
            <Typography variant="headingXLarge" noWrap>{title}</Typography>
            {subtitle && (
              <Typography variant="bodySmallDefault" color="text.secondary" noWrap>{subtitle}</Typography>
            )}
          </Box>
        </Box>
        <Box sx={{ display: 'flex', gap: 1, flexShrink: 0 }}>
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
          <Grid item xs={sidebar ? 9 : 12} sx={{ height: '100%' }}>
            <StudioThread ref={viewportRef} hideFileUpload={hideFileUpload} />
          </Grid>
          {sidebar && (
            <Grid item xs={3} sx={{ height: '100%', overflowY: 'auto' }} data-print-role="sidebar">
              {typeof sidebar === 'function' ? sidebar({ printMessages }) : sidebar}
            </Grid>
          )}
        </Grid>
      </Box>
    </>
  );
};

/**
 * Everything below the runtime provider for one session: creates the
 * runtime for `endpoints` + `session`, renders the toolbar, the thread and an
 * optional sidebar. Remount it (key by session id) to switch sessions.
 */
const StudioChatShell = ({ session, endpoints, onRunError, ...layoutProps }) => {
  const viewportRef = useRef(null);
  const clientToolNames = useMemo(() => (session.client_tools || []).map((t) => t.name), [session.client_tools]);
  const runtime = useStudioRuntime({ sessionId: session.session_id, endpoints, clientToolNames, onRunError });
  // Plugin-provided tool renderers load once per page; tools render with the
  // default card until they arrive.
  useEffect(() => {
    loadPluginToolRenderers();
  }, []);
  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <Layout viewportRef={viewportRef} {...layoutProps} />
    </AssistantRuntimeProvider>
  );
};

export default StudioChatShell;
