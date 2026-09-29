import apiClient from '../utils/apiClient';

// Push operation statuses that are final.
export const PUSH_FINAL_STATUSES = ['succeeded', 'succeeded_with_warnings', 'partially_failed', 'failed', 'expired'];

/** The API's error text: JSON:API `errors[].detail`, or `message`/`error`. */
export const apiErrorMessage = (error, fallback) =>
  error?.response?.data?.errors?.[0]?.detail ||
  error?.response?.data?.message ||
  error?.response?.data?.error ||
  fallback;

/** A push operation ("reload") as the API reports it, in camelCase. */
export const normalisePushOperation = (a) => ({
  operationId: a.operation_id,
  scope: a.scope,
  targetNamespace: a.target_namespace,
  targetEdges: a.target_edges || [],
  initiatedBy: a.initiated_by,
  initiatedAt: a.initiated_at,
  deadlineAt: a.deadline_at || null,
  completedAt: a.completed_at || null,
  status: a.status,
  progress: typeof a.progress === 'number' ? a.progress : 0,
  message: a.message || '',
  counts: a.counts || {},
  warnings: a.warnings || [],
  skipped: (a.skipped || []).map((t) => ({ edgeId: t.edge_id, namespace: t.namespace, reason: t.reason })),
  skippedTotal: a.skipped_total || (a.skipped || []).length,
  targets: (a.targets || []).map((t) => ({ edgeId: t.edge_id, namespace: t.namespace, reachable: !!t.reachable, reason: t.reason || '' })),
  edges: a.edges
    ? a.edges.map((e) => ({
        edgeId: e.edge_id,
        namespace: e.namespace,
        status: e.status,
        phase: e.phase || '',
        message: e.message || '',
        warning: e.warning || '',
        attempts: e.attempts || 0,
        maxAttempts: e.max_attempts || 0,
        reachable: typeof e.reachable === 'boolean' ? e.reachable : null,
        waitingReason: e.waiting_reason || '',
      }))
    : null,
});

class EdgeGatewayService {
  async listEdgeGateways(namespace = null) {
    try {
      const params = namespace ? { namespace } : {};
      const response = await apiClient.get('/edges', { params });
      
      if (response.data?.data) {
        return {
          data: response.data.data.map(edge => ({
            id: edge.id,
            edgeId: edge.attributes.edge_id,
            namespace: edge.attributes.namespace || 'global',
            status: edge.attributes.status,
            version: edge.attributes.version,
            buildHash: edge.attributes.build_hash,
            metadata: edge.attributes.metadata || {},
            lastHeartbeat: edge.attributes.last_heartbeat,
            sessionId: edge.attributes.session_id,
            // Sync status fields
            syncStatus: edge.attributes.sync_status || 'unknown',
            loadedChecksum: edge.attributes.loaded_checksum,
            loadedVersion: edge.attributes.loaded_version,
            lastSyncAck: edge.attributes.last_sync_ack,
            createdAt: edge.attributes.created_at,
            updatedAt: edge.attributes.updated_at,
          })),
          meta: response.data.meta || {},
        };
      }

      return { data: [], meta: {} };
    } catch (error) {
      console.error('Error fetching edge gateways:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch edge gateways');
    }
  }

  async getEdgeGateway(id) {
    try {
      const response = await apiClient.get(`/edges/${id}`);

      if (response.data?.data) {
        const edge = response.data.data;
        return {
          id: edge.id,
          edgeId: edge.attributes.edge_id,
          namespace: edge.attributes.namespace || 'global',
          status: edge.attributes.status,
          version: edge.attributes.version,
          buildHash: edge.attributes.build_hash,
          metadata: edge.attributes.metadata || {},
          lastHeartbeat: edge.attributes.last_heartbeat,
          sessionId: edge.attributes.session_id,
          // Sync status fields
          syncStatus: edge.attributes.sync_status || 'unknown',
          loadedChecksum: edge.attributes.loaded_checksum,
          loadedVersion: edge.attributes.loaded_version,
          lastSyncAck: edge.attributes.last_sync_ack,
          createdAt: edge.attributes.created_at,
          updatedAt: edge.attributes.updated_at,
        };
      }

      return null;
    } catch (error) {
      console.error('Error fetching edge gateway:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch edge gateway');
    }
  }

  async getEdgesInNamespace(namespace) {
    try {
      const response = await apiClient.get(`/namespaces/${namespace}/edges`);

      if (response.data?.data) {
        return {
          data: response.data.data.map(edge => ({
            id: edge.id,
            edgeId: edge.attributes.edge_id,
            namespace: edge.attributes.namespace || 'global',
            status: edge.attributes.status,
            version: edge.attributes.version,
            buildHash: edge.attributes.build_hash,
            metadata: edge.attributes.metadata || {},
            lastHeartbeat: edge.attributes.last_heartbeat,
            sessionId: edge.attributes.session_id,
            // Sync status fields
            syncStatus: edge.attributes.sync_status || 'unknown',
            loadedChecksum: edge.attributes.loaded_checksum,
            loadedVersion: edge.attributes.loaded_version,
            lastSyncAck: edge.attributes.last_sync_ack,
            createdAt: edge.attributes.created_at,
            updatedAt: edge.attributes.updated_at,
          })),
          meta: response.data.meta || {},
        };
      }

      return { data: [], meta: {} };
    } catch (error) {
      console.error('Error fetching edges in namespace:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch edges in namespace');
    }
  }

  /**
   * Starts a configuration push to one namespace (targetType 'namespace') or
   * one edge ('edge'). The push is only recorded here; follow it with
   * getPushProgress until it is no longer in progress.
   */
  async triggerConfigurationReload(namespace, targetType = 'namespace') {
    try {
      const endpoint = targetType === 'namespace'
        ? `/namespaces/${namespace}/reload`
        : `/edges/${namespace}/reload`; // namespace is actually edge ID in this case

      const response = await apiClient.post(endpoint);
      const attributes = response.data?.data?.attributes;
      return attributes ? normalisePushOperation(attributes) : null;
    } catch (error) {
      console.error('Error triggering configuration reload:', error);
      throw new Error(apiErrorMessage(error, 'Failed to trigger configuration reload'));
    }
  }

  /** Starts a push to every edge gateway, as one operation. */
  async reloadAllEdges() {
    try {
      const response = await apiClient.post('/edges/reload-all');
      const attributes = response.data?.data?.attributes;
      return attributes ? normalisePushOperation(attributes) : null;
    } catch (error) {
      console.error('Error triggering global reload:', error);
      throw new Error(apiErrorMessage(error, 'Failed to trigger global reload'));
    }
  }

  /**
   * What has changed in a namespace since its configuration was last pushed
   * to the edge gateways, so the push dialog can say what it is about to do.
   * Returns { namespace, since, lastPushAt, baseline, total, changes } where
   * `changes` is capped by the server (see `total` for the real count) and
   * `baseline` says what `since` is: 'push' (a recorded push), 'edge_ack'
   * (an in-sync edge's ack, no push recorded) or 'none'.
   */
  async getPendingChanges(namespace) {
    try {
      const params = namespace ? { namespace } : {};
      const response = await apiClient.get('/sync/pending-changes', { params });
      const data = response.data?.data || {};
      return {
        namespace: data.namespace ?? namespace ?? '',
        since: data.since || null,
        lastPushAt: data.last_push_at || null,
        baseline: data.baseline || (data.last_push_at ? 'push' : 'none'),
        total: typeof data.total === 'number' ? data.total : (data.changes || []).length,
        changes: (data.changes || []).map(change => ({
          type: change.type,
          id: change.id,
          name: change.name,
          change: change.change,
          at: change.at,
        })),
      };
    } catch (error) {
      console.error('Error fetching pending changes:', error);
      throw new Error(error.response?.data?.error || error.response?.data?.message || 'Failed to fetch pending changes');
    }
  }

  /** The full report of a push, per edge (Enterprise Edition). */
  async getReloadStatus(operationId) {
    try {
      const response = await apiClient.get(`/reload-operations/${operationId}/status`);
      const attributes = response.data?.data?.attributes;
      return attributes ? normalisePushOperation(attributes) : null;
    } catch (error) {
      console.error('Error fetching reload status:', error);
      const err = new Error(apiErrorMessage(error, 'Failed to fetch reload status'));
      err.status = error.response?.status;
      throw err;
    }
  }

  /** Recent pushes (the last day), newest first, with their outcome counts. */
  async listReloadOperations() {
    try {
      const response = await apiClient.get('/edges/reload-operations');
      return (response.data?.data || []).map((op) => normalisePushOperation(op.attributes || {}));
    } catch (error) {
      console.error('Error listing reload operations:', error);
      throw new Error(apiErrorMessage(error, 'Failed to list reload operations'));
    }
  }

  /**
   * How a push is going. Enterprise Edition reports each edge; Community
   * Edition (where the per-edge report is not available) falls back to the
   * push's outcome counts from the listing, with `edges` null.
   */
  async getPushProgress(operationId) {
    try {
      return await this.getReloadStatus(operationId);
    } catch (error) {
      if (error.status !== 402) {
        throw error;
      }
    }
    const operations = await this.listReloadOperations();
    const found = operations.find((op) => op.operationId === operationId);
    if (!found) {
      throw new Error('The push is no longer listed');
    }
    return { ...found, edges: null };
  }

  async deleteEdgeGateway(edgeId) {
    try {
      await apiClient.delete(`/edges/${edgeId}`);
      return { success: true };
    } catch (error) {
      console.error('Error deleting edge gateway:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to delete edge gateway');
    }
  }

  // Utility function to determine connection status based on last heartbeat
  getConnectionStatus(lastHeartbeat) {
    if (!lastHeartbeat) {
      return { status: 'disconnected', color: 'error', label: 'Disconnected' };
    }

    const heartbeatTime = new Date(lastHeartbeat);
    const now = new Date();
    const ageInMinutes = (now - heartbeatTime) / (1000 * 60);

    if (ageInMinutes < 5) {
      return { status: 'connected', color: 'success', label: 'Connected' };
    } else if (ageInMinutes < 15) {
      return { status: 'stale', color: 'warning', label: 'Stale' };
    } else {
      return { status: 'disconnected', color: 'error', label: 'Disconnected' };
    }
  }

  // Utility function to format last heartbeat time
  formatLastHeartbeat(lastHeartbeat) {
    if (!lastHeartbeat) {
      return 'Never';
    }

    const heartbeatTime = new Date(lastHeartbeat);
    const now = new Date();
    const ageInMinutes = (now - heartbeatTime) / (1000 * 60);

    if (ageInMinutes < 1) {
      return 'Just now';
    } else if (ageInMinutes < 60) {
      return `${Math.floor(ageInMinutes)} minutes ago`;
    } else if (ageInMinutes < 1440) { // 24 hours
      return `${Math.floor(ageInMinutes / 60)} hours ago`;
    } else {
      return heartbeatTime.toLocaleDateString();
    }
  }

  // Utility function to get sync status display info
  getSyncStatusDisplay(syncStatus) {
    const statusConfig = {
      in_sync: { status: 'in_sync', color: 'success', label: 'Synced' },
      pending: { status: 'pending', color: 'warning', label: 'Pending' },
      stale: { status: 'stale', color: 'error', label: 'Stale' },
      unknown: { status: 'unknown', color: 'default', label: 'Unknown' },
    };
    return statusConfig[syncStatus] || statusConfig.unknown;
  }
}

export default new EdgeGatewayService();