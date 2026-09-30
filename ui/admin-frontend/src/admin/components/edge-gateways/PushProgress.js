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
 * Whether the push is still waiting for an edge to connect. Until the first
 * progress report, what starting the push said stands; then Enterprise
 * reports each edge (waiting: pending and not reachable) and Community
 * Edition the counts (waiting: anything still pending).
 */
const waitingForConnection = (progress, polled) => {
  if (!polled) return true;
  if (progress.edges) return progress.edges.some((e) => e.status === 'pending' && e.reachable !== true);
  return (progress.counts?.pending || 0) > 0;
};

/**
 * Follows a push until every edge has answered (or the push's deadline has
 * passed) and says, per edge, what happened. `operation` is what starting
 * the push returned.
 */
const PushProgress = ({ operation, pollIntervalMs = 1500, onFinished }) => {
  const [progress, setProgress] = useState(operation);
  const [polled, setPolled] = useState(false);
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
        if (next) {
          setProgress((prev) => ({ ...prev, ...next }));
          setPolled(true);
        }
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
  // The warnings from starting the push are about edges that were not
  // connected ("the push waits up to 5m0s...") and edges left out; they
  // stand only while an edge is still waiting to connect, not once every
  // edge has its push (a reloading edge is not waiting). Later reports carry
  // their own warnings (per edge, on Enterprise). Left-out edges stay listed
  // below either way.
  const waiting = inProgress && waitingForConnection(progress, polled);
  const warnings = mergeWarnings(waiting ? operation.warnings : [], polled ? progress.warnings : []);
  // Only starting the push reports the edges left out.
  const skipped = operation.skipped || [];
  const skippedTotal = operation.skippedTotal || skipped.length;

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
              {waiting && (
                <>
                  Edges that are not connected are waited for
                  {progress.deadlineAt ? ` until ${new Date(progress.deadlineAt).toLocaleTimeString()}` : ''}.{' '}
                </>
              )}
              The push carries on if you close this dialog.
            </Typography>
          </Box>
        )}
      </Alert>

      {warnings.length > 0 && (
        <Alert severity="warning" sx={{ mb: 2 }} data-testid="push-warnings">
          {warnings.map((w) => (
            <div key={w}>{w}</div>
          ))}
        </Alert>
      )}

      {skipped.length > 0 && (
        <Alert severity="info" sx={{ mb: 2 }} data-testid="push-skipped">
          <AlertTitle>Not pushed to</AlertTitle>
          {skipped.map((s) => (
            <div key={s.edgeId}>
              {s.edgeId}: {s.reason}
            </div>
          ))}
          {skippedTotal > skipped.length && (
            <div>and {skippedTotal - skipped.length} more</div>
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
