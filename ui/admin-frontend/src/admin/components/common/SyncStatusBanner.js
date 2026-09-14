import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import WarningBanner from './WarningBanner';
import { useSyncStatus } from '../../context/SyncStatusContext';
import PushConfigurationModal from '../edge-gateways/PushConfigurationModal';

const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`;

/**
 * The banner's headline. With the pending-changes lookup available it says
 * what is waiting ("3 changes not yet pushed to 1 edge gateway in namespace
 * default"); without it (older Studio, transient error) it falls back to the
 * edge counts alone.
 */
export const buildSyncMessage = (pendingNamespaces, pendingChanges) => {
  const edges = pendingNamespaces.reduce(
    (sum, ns) => sum + (ns.pending_count || 0) + (ns.stale_count || 0),
    0
  );
  const names = pendingNamespaces.map((ns) => ns.namespace || 'default');
  const known = pendingChanges && names.every((name) => pendingChanges[name]);
  const changes = known ? names.reduce((sum, name) => sum + (pendingChanges[name].total || 0), 0) : null;

  const where =
    names.length > 1
      ? ` across ${plural(names.length, 'namespace')}`
      : names.length === 1
        ? ` in namespace "${names[0]}"`
        : '';

  let message;
  if (changes !== null && changes > 0) {
    message = `${plural(changes, 'change')} not yet pushed to ${plural(edges, 'edge gateway')}${where}`;
  } else {
    message = `${plural(edges, 'edge gateway')} ${edges === 1 ? 'has' : 'have'} configuration updates pending${where}`;
  }
  return `${message}. Changes you have saved are not live on those gateways until you push.`;
};

/**
 * SyncStatusBanner displays a warning banner when edge gateways are out of sync
 * with the control plane configuration.
 *
 * Features:
 * - Uses shared SyncStatusContext for state management
 * - Responds immediately when refreshSyncStatus is called (e.g., after config push)
 * - Shows warning when edges have pending configuration updates
 * - Dismissible but reappears if new pending syncs are detected
 * - Provides quick navigation to edge gateways page
 */
const SyncStatusBanner = () => {
  const navigate = useNavigate();
  const { syncStatus, hasPendingSync, pendingCount, pendingChanges } = useSyncStatus();
  const [dismissed, setDismissed] = useState(false);
  const [lastPendingCount, setLastPendingCount] = useState(0);
  const [pushOpen, setPushOpen] = useState(false);

  // If there are new pending syncs, undismiss the banner
  useEffect(() => {
    if (pendingCount > lastPendingCount && pendingCount > 0) {
      setDismissed(false);
    }
    setLastPendingCount(pendingCount);
  }, [pendingCount, lastPendingCount]);

  // Don't show banner if dismissed or no pending syncs
  if (!hasPendingSync || dismissed) {
    return null;
  }

  const pendingNamespaces = syncStatus?.data?.filter(ns =>
    (ns.pending_count > 0 || ns.stale_count > 0)) || [];
  const message = buildSyncMessage(pendingNamespaces, pendingChanges);

  // The banner named the problem and then sent the user two nav sections away
  // to do something about it, which is most of the distance between "I saved
  // it" and "why did nothing happen". Push from here.
  return (
    <>
      <WarningBanner
        title="Configuration Sync Pending"
        message={message}
        onClose={() => setDismissed(true)}
        showCloseButton={true}
        primaryButtonText="Push Configuration"
        onPrimaryButtonClick={() => setPushOpen(true)}
        buttonText="View Edge Gateways"
        onButtonClick={() => navigate('/admin/edge-gateways')}
        sx={{ marginBottom: 2 }}
      />
      <PushConfigurationModal
        open={pushOpen}
        onClose={() => setPushOpen(false)}
      />
    </>
  );
};

export default SyncStatusBanner;
