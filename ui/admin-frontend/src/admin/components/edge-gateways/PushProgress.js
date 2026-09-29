import React, { useEffect, useRef, useState } from 'react';
import {
  Alert,
  AlertTitle,
  Box,
  Chip,
  LinearProgress,
  List,
  ListItem,
  ListItemText,
  Typography,
} from '@mui/material';
import edgeGatewayService, { PUSH_FINAL_STATUSES } from '../../services/edgeGatewayService';

// How an edge's push reads to the user.
const EDGE_STATUS = {
  pending: { label: 'Waiting for connection', color: 'default' },
  claimed: { label: 'Sending', color: 'info' },
  sent: { label: 'Reloading', color: 'info' },
  succeeded: { label: 'Updated', color: 'success' },
  succeeded_with_warning: { label: 'Updated, with a warning', color: 'warning' },
  failed: { label: 'Failed', color: 'error' },
  expired: { label: 'Timed out', color: 'error' },
};

const OUTCOME = {
  in_progress: { severity: 'info', title: 'Pushing configuration' },
  succeeded: { severity: 'success', title: 'Configuration pushed' },
  succeeded_with_warnings: { severity: 'warning', title: 'Configuration pushed, with warnings' },
  partially_failed: { severity: 'error', title: 'Some edge gateways did not update' },
  failed: { severity: 'error', title: 'The push failed' },
  expired: { severity: 'error', title: 'The push timed out' },
};

const edgeDetail = (edge) => {
  if (edge.warning) return edge.warning;
  if (edge.status === 'pending' && edge.waitingReason) return `Waiting: ${edge.waitingReason}`;
  if (edge.status === 'sent' && edge.phase) return `Edge reports ${edge.phase.toLowerCase().replace('_', ' ')}`;
  return edge.message;
};

/**
 * Follows a push until every edge has answered (or the push's deadline has
 * passed) and says, per edge, what happened. `operation` is what starting
 * the push returned.
 */
const PushProgress = ({ operation, pollIntervalMs = 1500, onFinished }) => {
  const [progress, setProgress] = useState(operation);
  const [pollError, setPollError] = useState(null);
  const finishedRef = useRef(false);
  const onFinishedRef = useRef(onFinished);
  onFinishedRef.current = onFinished;

  useEffect(() => {
    let cancelled = false;
    let timer;
    const poll = async () => {
      try {
        const next = await edgeGatewayService.getPushProgress(operation.operationId);
        if (cancelled) return;
        if (next) setProgress((prev) => ({ ...prev, ...next, warnings: mergeWarnings(prev.warnings, next.warnings) }));
        setPollError(null);
        if (next && PUSH_FINAL_STATUSES.includes(next.status)) {
          if (!finishedRef.current) {
            finishedRef.current = true;
            if (onFinishedRef.current) onFinishedRef.current(next);
          }
          return;
        }
      } catch (err) {
        if (cancelled) return;
        setPollError(err.message);
      }
      timer = setTimeout(poll, pollIntervalMs);
    };
    timer = setTimeout(poll, pollIntervalMs);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [operation.operationId, pollIntervalMs]);

  const outcome = OUTCOME[progress.status] || OUTCOME.in_progress;
  const inProgress = !PUSH_FINAL_STATUSES.includes(progress.status);
  const total = progress.targetEdges.length || progress.edges?.length || 0;

  return (
    <Box data-testid="push-progress">
      <Alert severity={outcome.severity} sx={{ mb: 2 }} data-testid="push-outcome">
        <AlertTitle>{outcome.title}</AlertTitle>
        {progress.message || `Pushing to ${total} edge gateway(s).`}
        {inProgress && (
          <Box sx={{ mt: 1 }}>
            <LinearProgress
              variant="determinate"
              value={progress.progress}
              aria-label="Push progress"
            />
            <Typography variant="caption" color="text.secondary">
              Edges that are not connected are waited for
              {progress.deadlineAt ? ` until ${new Date(progress.deadlineAt).toLocaleTimeString()}` : ''}.
              The push carries on if you close this dialog.
            </Typography>
          </Box>
        )}
      </Alert>

      {progress.warnings.length > 0 && (
        <Alert severity="warning" sx={{ mb: 2 }} data-testid="push-warnings">
          {progress.warnings.map((w) => (
            <div key={w}>{w}</div>
          ))}
        </Alert>
      )}

      {progress.skipped.length > 0 && (
        <Alert severity="info" sx={{ mb: 2 }} data-testid="push-skipped">
          <AlertTitle>Not pushed to</AlertTitle>
          {progress.skipped.map((s) => (
            <div key={s.edgeId}>
              {s.edgeId}: {s.reason}
            </div>
          ))}
          {progress.skippedTotal > progress.skipped.length && (
            <div>and {progress.skippedTotal - progress.skipped.length} more</div>
          )}
        </Alert>
      )}

      {pollError && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          Could not fetch the push's progress ({pollError}); retrying.
        </Alert>
      )}

      {progress.edges ? (
        <List dense data-testid="push-edges" sx={{ maxHeight: 280, overflow: 'auto' }}>
          {progress.edges.map((edge) => {
            const s = EDGE_STATUS[edge.status] || { label: edge.status, color: 'default' };
            return (
              <ListItem key={edge.edgeId} data-testid={`push-edge-${edge.edgeId}`} disableGutters>
                <ListItemText
                  primary={
                    <Box display="flex" alignItems="center" gap={1}>
                      <Typography variant="body2" component="span">{edge.edgeId}</Typography>
                      <Chip size="small" label={s.label} color={s.color} />
                      {edge.attempts > 1 && (
                        <Typography variant="caption" color="text.secondary" component="span">
                          attempt {edge.attempts} of {edge.maxAttempts}
                        </Typography>
                      )}
                    </Box>
                  }
                  secondary={edgeDetail(edge)}
                />
              </ListItem>
            );
          })}
        </List>
      ) : (
        progress.edges === null && !inProgress && (
          <Typography variant="body2" color="text.secondary">
            Per-edge results are available in Enterprise Edition; each edge gateway&apos;s sync status is on its page.
          </Typography>
        )
      )}
    </Box>
  );
};

function mergeWarnings(a = [], b = []) {
  return Array.from(new Set([...a, ...b]));
}

export default PushProgress;
