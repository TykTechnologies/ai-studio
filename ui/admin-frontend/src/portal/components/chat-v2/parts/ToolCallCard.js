import React, { useState } from 'react';
import {
  Box,
  Chip,
  CircularProgress,
  Collapse,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import BuildIcon from '@mui/icons-material/Build';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ErrorIcon from '@mui/icons-material/Error';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import { useToolCallElapsed } from '@assistant-ui/react';

const isPlainObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

/** Array of flat objects -> table; anything else -> pretty JSON / text. */
export const ResultViewer = ({ result }) => {
  if (result === undefined || result === null) return null;

  if (Array.isArray(result) && result.length > 0 && result.length <= 200 && result.every(isPlainObject)) {
    const columns = Array.from(new Set(result.flatMap((row) => Object.keys(row)))).slice(0, 12);
    return (
      <Box sx={{ overflowX: 'auto' }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              {columns.map((c) => (
                <TableCell key={c} sx={{ fontWeight: 600 }}>{c}</TableCell>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {result.map((row, i) => (
              <TableRow key={i}>
                {columns.map((c) => (
                  <TableCell key={c} sx={{ whiteSpace: 'nowrap', maxWidth: 320, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {isPlainObject(row[c]) || Array.isArray(row[c]) ? JSON.stringify(row[c]) : String(row[c] ?? '')}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Box>
    );
  }

  const text = typeof result === 'string' ? result : JSON.stringify(result, null, 2);
  return (
    <Box
      component="pre"
      sx={{
        m: 0,
        p: 1,
        maxHeight: 320,
        overflow: 'auto',
        fontFamily: 'monospace',
        fontSize: '0.8rem',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        bgcolor: 'background.default',
        borderRadius: 1,
      }}
    >
      {text}
    </Box>
  );
};

const ElapsedLabel = () => {
  const elapsed = useToolCallElapsed();
  if (elapsed === undefined || elapsed === null) return null;
  return (
    <Typography variant="caption" color="text.secondary">
      {(elapsed / 1000).toFixed(1)}s
    </Typography>
  );
};

/**
 * Default renderer for a tool call: header with name and state, collapsible
 * arguments (streamed as they arrive) and the result. Used as the fallback
 * for every tool without a dedicated renderer.
 */
const ToolCallCard = ({ toolName, args, argsText, result, isError, status }) => {
  const running = status?.type === 'running';
  const requiresAction = status?.type === 'requires-action';
  const [expanded, setExpanded] = useState(false);

  const color = isError ? 'error' : running ? 'info' : 'success';
  const Icon = isError ? ErrorIcon : running ? BuildIcon : CheckCircleIcon;
  const label = isError
    ? `Tool failed: ${toolName}`
    : running
      ? `Calling ${toolName}`
      : requiresAction
        ? `Waiting for your input: ${toolName}`
        : `Tool result: ${toolName}`;

  const argsPretty = (() => {
    if (args && Object.keys(args).length > 0) return JSON.stringify(args, null, 2);
    return argsText || '';
  })();

  return (
    <Paper
      variant="outlined"
      sx={{
        my: 1,
        p: 1.5,
        borderLeft: '4px solid',
        borderLeftColor: `${color}.main`,
        bgcolor: isError ? 'background.surfaceCriticalDefault' : running ? 'background.surfaceInformativeDefault' : 'background.surfaceSuccessDefault',
      }}
      data-testid="tool-call-card"
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        {running ? <CircularProgress size={16} /> : <Icon fontSize="small" color={color} />}
        <Typography variant="bodyMedium" fontWeight={500} sx={{ flex: 1 }}>
          {label}
        </Typography>
        {running && <ElapsedLabel />}
        {isError && <Chip label="error" size="small" color="error" variant="outlined" />}
        <IconButton size="small" onClick={() => setExpanded((v) => !v)} aria-label="Toggle tool details">
          {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
        </IconButton>
      </Box>

      <Collapse in={expanded}>
        {argsPretty && (
          <Box sx={{ mt: 1 }}>
            <Typography variant="caption" color="text.secondary">Arguments</Typography>
            <Box
              component="pre"
              sx={{ m: 0, p: 1, fontFamily: 'monospace', fontSize: '0.8rem', whiteSpace: 'pre-wrap', bgcolor: 'background.default', borderRadius: 1 }}
            >
              {argsPretty}
            </Box>
          </Box>
        )}
        {result !== undefined && (
          <Box sx={{ mt: 1 }}>
            <Typography variant="caption" color="text.secondary">Result</Typography>
            <ResultViewer result={result} />
          </Box>
        )}
      </Collapse>
    </Paper>
  );
};

export default ToolCallCard;
