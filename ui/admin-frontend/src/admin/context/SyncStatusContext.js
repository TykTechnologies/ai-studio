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
 *
 * Note: Only fetches for admin users since sync status is only relevant
 * to administrators managing edge gateways.
 */
export const SyncStatusProvider = ({ children }) => {
  const [syncStatus, setSyncStatus] = useState(null);
  const [loading, setLoading] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(null);

  // Check if user is an admin (set by App.js after auth check)
  const isAdmin = () => window.adminEntitlements?.is_admin === true;

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

  // Initial fetch on mount (only for admins)
  useEffect(() => {
    // Small delay to allow App.js to set adminEntitlements after auth check
    const timer = setTimeout(() => {
      if (isAdmin()) {
        fetchSyncStatus();
      }
    }, 100);
    return () => clearTimeout(timer);
  }, [fetchSyncStatus]);

  // Auto-refresh every 30 seconds (only for admins)
  useEffect(() => {
    if (!isAdmin()) {
      return; // No polling for non-admins
    }
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
