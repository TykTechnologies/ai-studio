import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Typography,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Button,
  Alert,
  AlertTitle,
  CircularProgress,
  Box,
  RadioGroup,
  FormControlLabel,
  Radio,
  FormLabel,
} from '@mui/material';
import { CloudSync as PushIcon } from '@mui/icons-material';
import edgeGatewayService from '../../services/edgeGatewayService';
import useNamespaces from '../../hooks/useNamespaces';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import { useSyncStatus } from '../../context/SyncStatusContext';
import usePendingChanges from './usePendingChanges';
import PendingChangesPreview from './PendingChangesPreview';
import PushProgress from './PushProgress';

const PushConfigurationModal = ({ open, onClose, onSuccess, pollIntervalMs }) => {
  const { getAvailableNamespaces } = useNamespaces();
  const { features } = useSystemFeatures();
  const { refreshSyncStatus, notifyConfigPushed, syncStatus } = useSyncStatus();

  // Default to 'all' on every edition. On Enterprise this used to open on
  // 'namespace' with nothing selected, so the modal appeared with its submit
  // button already disabled and no indication of why -- the user had to work
  // out that a namespace still had to be chosen. Narrowing to one namespace is
  // the deliberate act; pushing everything is the ordinary one.
  const [targetType, setTargetType] = useState('all');
  const [selectedNamespace, setSelectedNamespace] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  // The target a push was refused for because no edge gateway could receive
  // it (409: every edge offline too long, or none registered). Pushing the
  // same target again would fail the same way, so Push stays disabled until
  // the target changes.
  const [noTargetsFor, setNoTargetsFor] = useState(null);
  const errorRef = useRef(null);
  // The push once it is recorded; PushProgress follows it from there.
  const [operation, setOperation] = useState(null);
  const success = operation !== null;

  const availableNamespaces = getAvailableNamespaces();

  // Which namespaces the preview describes: the chosen one, or -- for "all" --
  // every namespace the sync status knows about, the ones with something
  // pending first so they open at the top.
  const previewNamespaces = useMemo(() => {
    if (targetType === 'namespace') {
      return selectedNamespace ? [selectedNamespace] : [];
    }
    const summaries = syncStatus?.data || [];
    const pending = summaries.filter((ns) => (ns.pending_count || 0) > 0 || (ns.stale_count || 0) > 0);
    const rest = summaries.filter((ns) => !pending.includes(ns));
    return [...pending, ...rest].map((ns) => ns.namespace || 'default');
  }, [targetType, selectedNamespace, syncStatus]);

  const pending = usePendingChanges(previewNamespaces, open && !success);

  const targetKey = `${targetType}:${targetType === 'namespace' ? selectedNamespace : ''}`;
  const noTargets = noTargetsFor !== null && noTargetsFor === targetKey;

  // An error is shown at the top of the dialog; bring it into view, since the
  // preview above the buttons can be long.
  useEffect(() => {
    if (error && errorRef.current && errorRef.current.scrollIntoView) {
      errorRef.current.scrollIntoView({ block: 'nearest' });
    }
  }, [error]);

  const changeTarget = (type, namespace) => {
    setTargetType(type);
    setSelectedNamespace(namespace);
    setError(null);
  };

  const handleClose = () => {
    if (!loading) {
      setTargetType('all');
      setSelectedNamespace('');
      setError(null);
      setNoTargetsFor(null);
      setOperation(null);
      onClose();
    }
  };

  const handleSubmit = async () => {
    setLoading(true);
    setError(null);
    setOperation(null);

    try {
      let result;

      if (targetType === 'all') {
        // CE/ENT: every edge gateway, as one push
        result = await edgeGatewayService.reloadAllEdges();
      } else {
        // ENT only: Push to specific namespace
        if (!selectedNamespace) {
          setError('Please select a namespace');
          return;
        }

        result = await edgeGatewayService.triggerConfigurationReload(
          selectedNamespace === 'global' ? 'global' : selectedNamespace,
          'namespace'
        );
      }
      if (!result) {
        throw new Error('The server did not return the push it started');
      }
      // Recorded, not yet delivered: PushProgress reports what the edges do.
      setOperation(result);

      if (onSuccess) {
        onSuccess();
      }

      // Refresh sync status after the push starts. The edges acknowledge
      // over their next heartbeat, so the provider keeps polling for a short
      // while until nothing is pending any more (or gives up after 30 s).
      if (notifyConfigPushed) {
        notifyConfigPushed();
      } else {
        refreshSyncStatus();
      }
    } catch (err) {
      console.error('Error pushing configuration:', err);
      setError(err.message);
      if (err.status === 409) {
        setNoTargetsFor(targetKey);
      }
    } finally {
      setLoading(false);
    }
  };

  const isValid = targetType === 'all' || Boolean(selectedNamespace);
  // A disabled control must say why it is disabled.
  let disabledReason = '';
  if (!isValid && targetType === 'namespace') {
    disabledReason = 'Select a namespace to push to, or choose All Namespaces.';
  } else if (noTargets) {
    disabledReason = 'No edge gateway can receive this push; see the message above.';
  }

  // When the preview says nothing has changed the push is still allowed
  // (a gateway may have been re-registered, or someone wants a fresh load),
  // but the button says so.
  const nothingToPush = previewNamespaces.length > 0 && pending.total === 0;
  const submitLabel = loading ? 'Pushing...' : nothingToPush ? 'Push anyway' : 'Push Configuration';

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        <Box display="flex" alignItems="center" gap={1}>
          <PushIcon />
          Push Configuration
        </Box>
      </DialogTitle>

      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} ref={errorRef} data-testid="push-error">
            {noTargets && <AlertTitle>Nothing to push to</AlertTitle>}
            {error}
          </Alert>
        )}

        <Typography variant="body2" color="textSecondary" paragraph>
          Push the latest configuration to edge gateways. This will reload all affected edge instances
          with the current configuration from the control server.
        </Typography>

        {/* CE: Hide namespace selection, ENT: Show radio buttons */}
        {features.hub_spoke_multi_tenant ? (
          <>
            <FormControl component="fieldset" fullWidth sx={{ mb: 3 }}>
              <FormLabel component="legend">Target</FormLabel>
              <RadioGroup
                value={targetType}
                onChange={(e) => changeTarget(e.target.value, selectedNamespace)}
              >
                <FormControlLabel
                  value="namespace"
                  control={<Radio />}
                  label="Specific Namespace"
                />
                <FormControlLabel
                  value="all"
                  control={<Radio />}
                  label="All Namespaces"
                />
              </RadioGroup>
            </FormControl>

            {targetType === 'namespace' && (
              <FormControl fullWidth required sx={{ mb: 3 }}>
                <InputLabel id="pushconfigurationmodal-select-namespace-label">Select Namespace</InputLabel>
                <Select
                  labelId="pushconfigurationmodal-select-namespace-label"
                  value={selectedNamespace}
                  label="Select Namespace"
                  onChange={(e) => changeTarget(targetType, e.target.value)}
                >
                  {availableNamespaces.map((namespace) => (
                    <MenuItem key={namespace.name} value={namespace.name}>
                      {namespace.name === 'global' ? 'Global' : namespace.name} ({namespace.edgeCount} edges)
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}

            {targetType === 'all' && !noTargets && (
              <Alert severity="info" sx={{ mb: 2 }}>
                This will push configuration to all {availableNamespaces.length} namespaces with active edges.
              </Alert>
            )}
          </>
        ) : (
          !noTargets && (
            <Alert severity="info" sx={{ mb: 2 }}>
              This will push configuration to all edge gateways.
            </Alert>
          )
        )}

        {/* What the push will actually change, so the user is not confirming blind. */}
        {!success && (
          <PendingChangesPreview
            namespaces={previewNamespaces}
            byNamespace={pending.byNamespace}
            onNavigate={handleClose}
          />
        )}

        {operation && (
          <PushProgress
            operation={operation}
            pollIntervalMs={pollIntervalMs}
            onFinished={() => refreshSyncStatus && refreshSyncStatus()}
          />
        )}

        {loading && (
          <Box display="flex" alignItems="center" gap={2} sx={{ mb: 2 }}>
            <CircularProgress size={20} />
            <Typography variant="body2">
              Initiating configuration push...
            </Typography>
          </Box>
        )}
      </DialogContent>

      <DialogActions>
        {disabledReason && (
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ mr: 'auto', ml: 1 }}
          >
            {disabledReason}
          </Typography>
        )}
        <Button onClick={handleClose} disabled={loading}>
          {success ? 'Close' : 'Cancel'}
        </Button>
        {!success && (
          <Button
            onClick={handleSubmit}
            variant="contained"
            disabled={loading || !isValid || noTargets}
            startIcon={loading ? <CircularProgress size={16} /> : <PushIcon />}
          >
            {submitLabel}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default PushConfigurationModal;
