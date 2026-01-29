import React, { createContext, useContext, useState, useCallback, useEffect } from 'react';
import syncStatusService from '../services/syncStatusService';

const SyncStatusContext = createContext();

/**
 * SyncStatusProvider manages the global sync status state and provides
 * a way to trigger immediate refreshes from anywhere in the application.
 *
 * This allows components like PushConfigurationModal to trigger an
 * immediate refresh of the sync status after a config push completes,
 * rather than waiting for the next polling interval.
 */
export const SyncStatusProvider = ({ children }) => {
  const [syncStatus, setSyncStatus] = useState(null);
  const [loading, setLoading] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(null);

  const fetchSyncStatus = useCallback(async () => {
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

  // Initial fetch on mount
  useEffect(() => {
    fetchSyncStatus();
  }, [fetchSyncStatus]);

  // Auto-refresh every 30 seconds
  useEffect(() => {
    const interval = setInterval(fetchSyncStatus, 30000);
    return () => clearInterval(interval);
  }, [fetchSyncStatus]);

  /**
   * Trigger an immediate refresh of the sync status.
   * Returns a promise that resolves when the refresh is complete.
   */
  const refreshSyncStatus = useCallback(async () => {
    return await fetchSyncStatus();
  }, [fetchSyncStatus]);

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
