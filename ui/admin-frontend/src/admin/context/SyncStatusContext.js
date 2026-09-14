import React, { createContext, useContext, useState, useCallback, useEffect, useRef } from 'react';
import syncStatusService from '../services/syncStatusService';
import edgeGatewayService from '../services/edgeGatewayService';
import { registerSyncStatusRefresh } from '../utils/configSyncNotifier';
import { hasPermissionNow, subscribe } from '../utils/identityStore';
import { P } from '../rbac/permissions';

const SyncStatusContext = createContext();

// After a push the edges acknowledge over their next heartbeat; poll this
// often, for at most this long, until nothing is pending any more.
export const POST_PUSH_POLL_INTERVAL_MS = 3000;
export const POST_PUSH_POLL_TIMEOUT_MS = 30000;

/**
 * SyncStatusProvider manages the global sync status state and provides
 * a way to trigger immediate refreshes from anywhere in the application.
 *
 * This allows components like PushConfigurationModal to trigger an
 * immediate refresh of the sync status after a config push completes,
 * rather than waiting for the next polling interval.
 *
 * Note: Only fetches for users who may read edge gateways, since sync
 * status is only relevant to people managing them.
 */
export const SyncStatusProvider = ({ children }) => {
  const [syncStatus, setSyncStatus] = useState(null);
  const [loading, setLoading] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(null);
  // namespace -> { total, lastPushAt } from /sync/pending-changes, for the
  // namespaces that currently have something to push. Null when the endpoint
  // is unavailable so consumers can fall back to edge counts.
  const [pendingChanges, setPendingChanges] = useState(null);
  const pollRef = useRef(null);

  // The identity store is populated by App.js before any provider mounts.
  const isAdmin = () => hasPermissionNow(P.EDGES_READ);

  // What has changed, per pending namespace. Failures (an older Studio
  // without the endpoint, a transient error) leave the counts unknown rather
  // than hiding the banner.
  const fetchPendingChanges = useCallback(async (status) => {
    const pendingNamespaces = (status?.data || [])
      .filter((ns) => (ns.pending_count || 0) > 0 || (ns.stale_count || 0) > 0)
      .map((ns) => ns.namespace || 'default');
    if (pendingNamespaces.length === 0) {
      setPendingChanges({});
      return {};
    }
    try {
      const results = await Promise.all(
        pendingNamespaces.map((ns) => edgeGatewayService.getPendingChanges(ns))
      );
      const next = {};
      results.forEach((result, i) => {
        next[pendingNamespaces[i]] = { total: result.total, lastPushAt: result.lastPushAt };
      });
      setPendingChanges(next);
      return next;
    } catch (error) {
      console.debug('Failed to fetch pending changes:', error);
      setPendingChanges(null);
      return null;
    }
  }, []);

  const fetchSyncStatus = useCallback(async () => {
    // Only fetch sync status for admin users
    if (!isAdmin()) {
      return null;
    }

    setLoading(true);
    try {
      const response = await syncStatusService.getSyncStatus();
      setSyncStatus(response);
      setLastRefresh(new Date());
      fetchPendingChanges(response);
      return response;
    } catch (error) {
      console.debug('Failed to fetch sync status:', error);
      return null;
    } finally {
      setLoading(false);
    }
  }, [fetchPendingChanges]);

  // Initial fetch on mount, and again whenever the identity changes (login,
  // role change), for users who may read edge gateways.
  useEffect(() => {
    if (isAdmin()) {
      fetchSyncStatus();
    }
    return subscribe(() => {
      if (isAdmin()) {
        fetchSyncStatus();
      }
    });
  }, [fetchSyncStatus]);

  // Auto-refresh every 30 seconds (only for users who may read edge gateways)
  useEffect(() => {
    const interval = setInterval(() => {
      if (isAdmin()) {
        fetchSyncStatus();
      }
    }, 30000);
    return () => clearInterval(interval);
  }, [fetchSyncStatus]);

  /**
   * Trigger an immediate refresh of the sync status.
   * Returns a promise that resolves when the refresh is complete.
   */
  const refreshSyncStatus = useCallback(async () => {
    return await fetchSyncStatus();
  }, [fetchSyncStatus]);

  const stopPostPushPolling = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current.interval);
      clearTimeout(pollRef.current.timeout);
      pollRef.current = null;
    }
  }, []);

  /**
   * Call right after a configuration push: refreshes at once, then keeps
   * polling every few seconds until no namespace is pending any more (or the
   * timeout passes), so the banner clears as soon as the edges acknowledge
   * rather than up to 30 s later.
   */
  const notifyConfigPushed = useCallback(() => {
    stopPostPushPolling();
    const poll = async () => {
      const status = await fetchSyncStatus();
      if (status && !status.has_pending) {
        stopPostPushPolling();
      }
    };
    pollRef.current = {
      interval: setInterval(poll, POST_PUSH_POLL_INTERVAL_MS),
      timeout: setTimeout(stopPostPushPolling, POST_PUSH_POLL_TIMEOUT_MS),
    };
    poll();
  }, [fetchSyncStatus, stopPostPushPolling]);

  useEffect(() => stopPostPushPolling, [stopPostPushPolling]);

  // Let the API client refresh sync status right after a save that the edge
  // gateways care about, so the pending banner appears while the user is still
  // looking at the thing they just saved.
  useEffect(() => registerSyncStatusRefresh(refreshSyncStatus), [refreshSyncStatus]);

  // Calculate derived values
  const hasPendingSync = syncStatus?.has_pending || false;
  const pendingCount = syncStatus?.data?.reduce(
    (sum, ns) => sum + (ns.pending_count || 0) + (ns.stale_count || 0),
    0
  ) || 0;

  /**
   * When a namespace's configuration was last pushed, from the sync status
   * summary (or the pending-changes lookup). Undefined when the running
   * Studio does not report it, null when it has never been pushed.
   */
  const getLastPushAt = useCallback(
    (namespace) => {
      const ns = namespace || 'default';
      const summary = syncStatus?.data?.find((s) => (s.namespace || 'default') === ns);
      if (summary && 'last_push_at' in summary) {
        return summary.last_push_at || null;
      }
      const pending = pendingChanges?.[ns];
      return pending ? pending.lastPushAt || null : undefined;
    },
    [syncStatus, pendingChanges]
  );

  return (
    <SyncStatusContext.Provider
      value={{
        syncStatus,
        loading,
        lastRefresh,
        hasPendingSync,
        pendingCount,
        pendingChanges,
        getLastPushAt,
        refreshSyncStatus,
        notifyConfigPushed,
      }}
    >
      {children}
    </SyncStatusContext.Provider>
  );
};

export const useSyncStatus = () => {
  const context = useContext(SyncStatusContext);
  if (!context) {
    throw new Error('useSyncStatus must be used within a SyncStatusProvider');
  }
  return context;
};
