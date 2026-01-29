import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import WarningBanner from './WarningBanner';
import { useSyncStatus } from '../../context/SyncStatusContext';

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
  const { syncStatus, hasPendingSync, pendingCount } = useSyncStatus();
  const [dismissed, setDismissed] = useState(false);
  const [lastPendingCount, setLastPendingCount] = useState(0);

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

  // Calculate totals
  const pendingNamespaces = syncStatus?.data?.filter(ns =>
    (ns.pending_count > 0 || ns.stale_count > 0)) || [];
  const totalPending = pendingNamespaces.reduce((sum, ns) =>
    sum + (ns.pending_count || 0), 0);
  const totalStale = pendingNamespaces.reduce((sum, ns) =>
    sum + (ns.stale_count || 0), 0);

  // Build message
  let message = `${totalPending + totalStale} edge gateway(s) have configuration updates pending`;
  if (pendingNamespaces.length > 1) {
    message += ` across ${pendingNamespaces.length} namespace(s)`;
  } else if (pendingNamespaces.length === 1 && pendingNamespaces[0].namespace) {
    message += ` in namespace "${pendingNamespaces[0].namespace || 'default'}"`;
  }
  message += '. Push configuration to sync.';

  return (
    <WarningBanner
      title="Configuration Sync Pending"
      message={message}
      onClose={() => setDismissed(true)}
      showCloseButton={true}
      buttonText="View Edge Gateways"
      onButtonClick={() => navigate('/admin/edge-gateways')}
      sx={{ marginBottom: 2 }}
    />
  );
};

export default SyncStatusBanner;
