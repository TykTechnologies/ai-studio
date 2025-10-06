import React, { useState } from 'react';
import { Box, Typography, Paper, Collapse, IconButton, Chip } from '@mui/material';
import {
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  Build as BuildIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
  Psychology as PsychologyIcon,
} from '@mui/icons-material';
import MarkdownMessage from '../chat/MarkdownMessage';

const AgentMessage = ({ message }) => {
  const [expanded, setExpanded] = useState(false);

  if (message.role === 'user') {
    return (
      <Box sx={{ mb: 3, display: 'flex', justifyContent: 'flex-end' }}>
        <Paper
          sx={{
            p: 2,
            maxWidth: '70%',
            bgcolor: 'primary.light',
            color: 'primary.contrastText',
          }}
        >
          <Typography variant="bodyMedium" sx={{ whiteSpace: 'pre-wrap' }}>
            {message.content}
          </Typography>
        </Paper>
      </Box>
    );
  }

  // Agent messages - different types
  const messageType = message.type?.toLowerCase() || 'content';

  // CONTENT - Regular AI response
  if (messageType === 'content') {
    return (
      <Box sx={{ mb: 3 }}>
        <Paper sx={{ p: 2 }}>
          <MarkdownMessage content={message.content} />
        </Paper>
      </Box>
    );
  }

  // TOOL_CALL - Agent is calling a tool
  if (messageType === 'tool_call') {
    const toolName = message.metadata?.tool_name || 'Tool';
    const toolParams = message.metadata?.parameters;

    return (
      <Box sx={{ mb: 2 }}>
        <Paper
          sx={{
            p: 2,
            bgcolor: 'info.light',
            borderLeft: '4px solid',
            borderLeftColor: 'info.main',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
            <BuildIcon fontSize="small" color="info" />
            <Typography variant="bodyMedium" fontWeight="medium">
              Calling Tool: {toolName}
            </Typography>
            <IconButton
              size="small"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
            </IconButton>
          </Box>

          <Collapse in={expanded}>
            {message.content && (
              <Typography variant="bodySmallDefault" sx={{ mt: 1 }}>
                {message.content}
              </Typography>
            )}
            {toolParams && (
              <Paper
                sx={{
                  mt: 1,
                  p: 1,
                  bgcolor: 'background.default',
                  fontFamily: 'monospace',
                  fontSize: '0.75rem',
                }}
              >
                <pre style={{ margin: 0, overflow: 'auto' }}>
                  {JSON.stringify(toolParams, null, 2)}
                </pre>
              </Paper>
            )}
          </Collapse>
        </Paper>
      </Box>
    );
  }

  // TOOL_RESULT - Result from tool execution
  if (messageType === 'tool_result') {
    const toolName = message.metadata?.tool_name || 'Tool';

    return (
      <Box sx={{ mb: 2 }}>
        <Paper
          sx={{
            p: 2,
            bgcolor: 'success.light',
            borderLeft: '4px solid',
            borderLeftColor: 'success.main',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
            <CheckCircleIcon fontSize="small" color="success" />
            <Typography variant="bodyMedium" fontWeight="medium">
              Tool Result: {toolName}
            </Typography>
            <IconButton
              size="small"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
            </IconButton>
          </Box>

          <Collapse in={expanded}>
            <Paper
              sx={{
                mt: 1,
                p: 1,
                bgcolor: 'background.default',
                maxHeight: 300,
                overflow: 'auto',
              }}
            >
              <Typography
                variant="bodySmallDefault"
                component="pre"
                sx={{ whiteSpace: 'pre-wrap', margin: 0 }}
              >
                {message.content}
              </Typography>
            </Paper>
          </Collapse>
        </Paper>
      </Box>
    );
  }

  // THINKING - Agent reasoning/planning
  if (messageType === 'thinking') {
    return (
      <Box sx={{ mb: 2 }}>
        <Paper
          sx={{
            p: 2,
            bgcolor: 'background.default',
            borderLeft: '4px solid',
            borderLeftColor: 'text.secondary',
            opacity: 0.8,
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
            <PsychologyIcon fontSize="small" color="disabled" />
            <Typography variant="bodySmallDefault" color="text.secondary" fontWeight="medium">
              Thinking
            </Typography>
            <Chip label="Internal" size="small" variant="outlined" />
            <IconButton
              size="small"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
            </IconButton>
          </Box>

          <Collapse in={expanded}>
            <Typography
              variant="bodySmallDefault"
              color="text.secondary"
              sx={{ fontStyle: 'italic', whiteSpace: 'pre-wrap' }}
            >
              {message.content}
            </Typography>
          </Collapse>
        </Paper>
      </Box>
    );
  }

  // ERROR - Error occurred
  if (messageType === 'error') {
    return (
      <Box sx={{ mb: 2 }}>
        <Paper
          sx={{
            p: 2,
            bgcolor: 'error.light',
            borderLeft: '4px solid',
            borderLeftColor: 'error.main',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
            <ErrorIcon fontSize="small" color="error" />
            <Typography variant="bodyMedium" fontWeight="medium" color="error.main">
              Error
            </Typography>
          </Box>
          <Typography variant="bodySmallDefault">
            {message.content}
          </Typography>
        </Paper>
      </Box>
    );
  }

  // DONE - Session complete
  if (messageType === 'done') {
    return (
      <Box sx={{ mb: 2, textAlign: 'center' }}>
        <Chip
          icon={<CheckCircleIcon />}
          label="Agent session completed"
          color="success"
          variant="outlined"
        />
      </Box>
    );
  }

  // Default fallback - treat as content
  return (
    <Box sx={{ mb: 3 }}>
      <Paper sx={{ p: 2 }}>
        <Typography variant="bodySmallDefault" color="text.secondary" gutterBottom>
          {messageType.toUpperCase()}
        </Typography>
        <Typography variant="bodyMedium" sx={{ whiteSpace: 'pre-wrap' }}>
          {message.content}
        </Typography>
      </Paper>
    </Box>
  );
};

export default AgentMessage;
