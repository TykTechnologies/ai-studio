import React, { createContext, useContext, useState, useCallback, useEffect } from 'react';
import syncStatusService from '../services/syncStatusService';
import { registerSyncStatusRefresh } from '../utils/configSyncNotifier';
import { hasPermissionNow, subscribe } from '../utils/identityStore';
import { P } from '../rbac/permissions';

const SyncStatusContext = createContext();

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

  // The identity store is populated by App.js before any provider mounts.
  const isAdmin = () => hasPermissionNow(P.EDGES_READ);

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
      return response;
    } catch (error) {
      console.debug('Failed to fetch sync status:', error);
      return null;
    } finally {
      setLoading(false);
    }
  }, []);

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

  return (
    <SyncStatusContext.Provider
      value={{
        syncStatus,
        loading,
        lastRefresh,
        hasPendingSync,
        pendingCount,
        refreshSyncStatus,
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
