import React, { forwardRef, useMemo } from 'react';
import { Box, Chip, CircularProgress, IconButton, Stack, Typography } from '@mui/material';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import { ThreadPrimitive, useAuiState } from '@assistant-ui/react';
import AssistantMessage from './AssistantMessage';
import UserMessage from './UserMessage';
import EditComposer from './EditComposer';
import Composer from './Composer';
import { useChatUi } from './ChatUiContext';

const messageComponents = { UserMessage, AssistantMessage, EditComposer };

const SystemMessagesToggle = () => {
  const { showSystemMessages, setShowSystemMessages } = useChatUi();
  return (
    <Box sx={{ mt: 2, textAlign: 'right' }} data-print-role="system-toggle">
      <Typography
        variant="caption"
        component="div"
        onClick={() => setShowSystemMessages(!showSystemMessages)}
        sx={{
          cursor: 'pointer',
          display: 'inline-flex',
          alignItems: 'center',
          color: showSystemMessages ? 'primary.main' : 'text.secondary',
          '&:hover': { color: 'primary.main' },
        }}
      >
        {showSystemMessages ? 'Hide' : 'Show'} System and Context Messages
      </Typography>
    </Box>
  );
};

const Welcome = ({ title, description, suggestions }) => (
  <Box sx={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', flex: 1, py: 4 }} data-print-role="welcome">
    <Typography variant="headingXLarge" mb={2}>Welcome to {title} chat</Typography>
    <Typography variant="headingXLargSub" mb={3}>How can I help you today?</Typography>
    {description && (
      <Typography variant="bodyLargeDefault" color="text.defaultSubdued" mb={4} maxWidth="600px">
        {description}
      </Typography>
    )}
    {suggestions?.length > 0 && (
      <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 1 }}>
        {suggestions.map((s) => (
          <ThreadPrimitive.Suggestion key={s.id || s.name} prompt={s.prompt} send={false} asChild>
            <Chip label={s.name} variant="outlined" clickable sx={{ borderRadius: '25px' }} />
          </ThreadPrimitive.Suggestion>
        ))}
      </Stack>
    )}
  </Box>
);

const LoadingIndicator = () => {
  const isLoading = useAuiState((s) => s.thread.isLoading);
  if (!isLoading) return null;
  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
      <CircularProgress />
    </Box>
  );
};

/**
 * The conversation: viewport with welcome screen, messages, scroll-to-bottom
 * and the composer. Keeps the data-print-role hooks the print stylesheet uses.
 */
const StudioThread = forwardRef(({ hideFileUpload = false }, viewportRef) => {
  const { session } = useChatUi();
  const suggestions = useMemo(() => session?.chat?.prompt_templates || [], [session]);

  return (
    <ThreadPrimitive.Root asChild>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', position: 'relative' }} data-print-role="messages-grid">
        <ThreadPrimitive.Viewport asChild autoScroll>
          <Box ref={viewportRef} sx={{ flexGrow: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column' }} data-print-role="messages">
            <Box sx={{ maxWidth: '740px', width: '100%', mx: 'auto', display: 'flex', flexDirection: 'column', flexGrow: 1, px: 2 }} data-print-role="messages-inner">
              <LoadingIndicator />
              <ThreadPrimitive.Empty>
                <Welcome title={session?.chat?.name} description={session?.chat?.description} suggestions={suggestions} />
              </ThreadPrimitive.Empty>
              <ThreadPrimitive.If empty={false}>
                <SystemMessagesToggle />
              </ThreadPrimitive.If>
              <ThreadPrimitive.Messages components={messageComponents} />
              <ThreadPrimitive.ViewportFooter asChild>
                <Box sx={{ height: 8 }} />
              </ThreadPrimitive.ViewportFooter>
            </Box>
          </Box>
        </ThreadPrimitive.Viewport>

        <ThreadPrimitive.ScrollToBottom asChild>
          <IconButton
            aria-label="Scroll to bottom"
            sx={{
              position: 'absolute',
              right: 24,
              bottom: 120,
              bgcolor: 'background.paper',
              boxShadow: 2,
              '&:disabled': { display: 'none' },
            }}
          >
            <KeyboardArrowDownIcon />
          </IconButton>
        </ThreadPrimitive.ScrollToBottom>

        <Box sx={{ width: '100%', px: 2, pt: 1 }} data-print-role="input">
          <Box sx={{ maxWidth: '740px', width: '100%', mx: 'auto' }}>
            <Composer hideFileUpload={hideFileUpload} />
          </Box>
        </Box>
      </Box>
    </ThreadPrimitive.Root>
  );
});

StudioThread.displayName = 'StudioThread';

export default StudioThread;
