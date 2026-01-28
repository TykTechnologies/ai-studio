import apiClient from '../utils/apiClient';

class SyncStatusService {
  /**
   * Get sync status summary for all namespaces
   * @returns {Promise<{data: Array, has_pending: boolean, pending_namespaces: Array}>}
   */
  async getSyncStatus() {
    try {
      const response = await apiClient.get('/sync/status');
      return response.data;
    } catch (error) {
      console.error('Error fetching sync status:', error);
      throw new Error(error.response?.data?.error || 'Failed to fetch sync status');
    }
  }

  /**
   * Get detailed sync status for a specific namespace
   * @param {string} namespace - The namespace to get status for
   * @returns {Promise<{namespace: Object, edges: Array}>}
   */
  async getNamespaceSyncStatus(namespace) {
    try {
      const response = await apiClient.get(`/sync/status/${namespace}`);
      return response.data;
    } catch (error) {
      console.error('Error fetching namespace sync status:', error);
      throw new Error(error.response?.data?.error || 'Failed to fetch namespace sync status');
    }
  }

  /**
   * Get sync audit log entries
   * @param {Object} params - Query parameters
   * @param {string} [params.namespace] - Filter by namespace
   * @param {string} [params.edge_id] - Filter by edge ID
   * @param {string} [params.event_type] - Filter by event type
   * @param {number} [params.limit=50] - Maximum number of entries to return
   * @returns {Promise<{data: Array}>}
   */
  async getSyncAuditLog(params = {}) {
    try {
      const response = await apiClient.get('/sync/audit', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching sync audit log:', error);
      throw new Error(error.response?.data?.error || 'Failed to fetch sync audit log');
    }
  }

  /**
   * Check if there are any pending syncs
   * @returns {Promise<boolean>}
   */
  async hasPendingSyncs() {
    try {
      const response = await this.getSyncStatus();
      return response.has_pending || false;
    } catch (error) {
      console.error('Error checking pending syncs:', error);
      return false;
    }
  }

  /**
   * Get sync status color for UI display
   * @param {string} syncStatus - The sync status value
   * @returns {{color: string, label: string}}
   */
  getSyncStatusDisplay(syncStatus) {
    const statusConfig = {
      in_sync: { color: 'success', label: 'Synced' },
      pending: { color: 'warning', label: 'Pending' },
      stale: { color: 'error', label: 'Stale' },
      unknown: { color: 'default', label: 'Unknown' },
    };
    return statusConfig[syncStatus] || statusConfig.unknown;
  }
}

export default new SyncStatusService();
