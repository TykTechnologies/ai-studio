import edgeGatewayService from './edgeGatewayService';
import apiClient from '../utils/apiClient';

// Mock apiClient
jest.mock('../utils/apiClient');

describe('EdgeGatewayService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore?.();
  });

  describe('listEdgeGateways', () => {
    const mockEdgeResponse = {
      data: {
        data: [
          {
            id: 1,
            attributes: {
              edge_id: 'edge-001',
              namespace: 'production',
              status: 'active',
              version: '1.0.0',
              build_hash: 'abc123',
              metadata: { region: 'us-east' },
              last_heartbeat: '2024-01-01T12:00:00Z',
              session_id: 'session-001',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T12:00:00Z',
            },
          },
        ],
        meta: { total: 1 },
      },
    };

    test('should fetch edge gateways without namespace filter', async () => {
      apiClient.get.mockResolvedValueOnce(mockEdgeResponse);

      const result = await edgeGatewayService.listEdgeGateways();

      expect(apiClient.get).toHaveBeenCalledWith('/edges', { params: {} });
      expect(result.data).toHaveLength(1);
      expect(result.data[0]).toEqual({
        id: 1,
        edgeId: 'edge-001',
        namespace: 'production',
        status: 'active',
        version: '1.0.0',
        buildHash: 'abc123',
        metadata: { region: 'us-east' },
        lastHeartbeat: '2024-01-01T12:00:00Z',
        sessionId: 'session-001',
        syncStatus: 'unknown',
        loadedChecksum: undefined,
        loadedVersion: undefined,
        lastSyncAck: undefined,
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T12:00:00Z',
      });
    });

    test('should fetch edge gateways with namespace filter', async () => {
      apiClient.get.mockResolvedValueOnce(mockEdgeResponse);

      await edgeGatewayService.listEdgeGateways('production');

      expect(apiClient.get).toHaveBeenCalledWith('/edges', { params: { namespace: 'production' } });
    });

    test('should return empty data when response has no data', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await edgeGatewayService.listEdgeGateways();

      expect(result).toEqual({ data: [], meta: {} });
    });

    test('should handle missing namespace attribute', async () => {
      const responseWithMissingNamespace = {
        data: {
          data: [
            {
              id: 1,
              attributes: {
                edge_id: 'edge-001',
                status: 'active',
                version: '1.0.0',
                build_hash: 'abc123',
                last_heartbeat: '2024-01-01T12:00:00Z',
                session_id: 'session-001',
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T12:00:00Z',
              },
            },
          ],
        },
      };
      apiClient.get.mockResolvedValueOnce(responseWithMissingNamespace);

      const result = await edgeGatewayService.listEdgeGateways();

      expect(result.data[0].namespace).toBe('global');
      expect(result.data[0].metadata).toEqual({});
    });

    test('should throw error on API failure', async () => {
      apiClient.get.mockRejectedValueOnce({
        response: { data: { message: 'Server error' } },
      });

      await expect(edgeGatewayService.listEdgeGateways()).rejects.toThrow('Server error');
    });

    test('should use default error message when none provided', async () => {
      apiClient.get.mockRejectedValueOnce(new Error('Network error'));

      await expect(edgeGatewayService.listEdgeGateways()).rejects.toThrow('Failed to fetch edge gateways');
    });
  });

  describe('getEdgeGateway', () => {
    const mockEdgeResponse = {
      data: {
        data: {
          id: 1,
          attributes: {
            edge_id: 'edge-001',
            namespace: 'production',
            status: 'active',
            version: '1.0.0',
            build_hash: 'abc123',
            metadata: { region: 'us-east' },
            last_heartbeat: '2024-01-01T12:00:00Z',
            session_id: 'session-001',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T12:00:00Z',
          },
        },
      },
    };

    test('should fetch a single edge gateway by ID', async () => {
      apiClient.get.mockResolvedValueOnce(mockEdgeResponse);

      const result = await edgeGatewayService.getEdgeGateway(1);

      expect(apiClient.get).toHaveBeenCalledWith('/edges/1');
      expect(result.id).toBe(1);
      expect(result.edgeId).toBe('edge-001');
      expect(result.status).toBe('active');
    });

    test('should return null when edge gateway not found', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await edgeGatewayService.getEdgeGateway('nonexistent');

      expect(result).toBeNull();
    });

    test('should handle missing optional attributes', async () => {
      const responseWithMissingAttrs = {
        data: {
          data: {
            id: 1,
            attributes: {
              edge_id: 'edge-001',
              status: 'active',
              version: '1.0.0',
              build_hash: 'abc123',
              last_heartbeat: '2024-01-01T12:00:00Z',
              session_id: 'session-001',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T12:00:00Z',
            },
          },
        },
      };
      apiClient.get.mockResolvedValueOnce(responseWithMissingAttrs);

      const result = await edgeGatewayService.getEdgeGateway(1);

      expect(result.namespace).toBe('global');
      expect(result.metadata).toEqual({});
    });

    test('should throw error on API failure', async () => {
      apiClient.get.mockRejectedValueOnce({
        response: { data: { message: 'Edge not found' } },
      });

      await expect(edgeGatewayService.getEdgeGateway(1)).rejects.toThrow('Edge not found');
    });
  });

  describe('getEdgesInNamespace', () => {
    const mockEdgeResponse = {
      data: {
        data: [
          {
            id: 1,
            attributes: {
              edge_id: 'edge-001',
              namespace: 'production',
              status: 'active',
              version: '1.0.0',
              build_hash: 'abc123',
              metadata: {},
              last_heartbeat: '2024-01-01T12:00:00Z',
              session_id: 'session-001',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T12:00:00Z',
            },
          },
        ],
        meta: { total: 1 },
      },
    };

    test('should fetch edges in a specific namespace', async () => {
      apiClient.get.mockResolvedValueOnce(mockEdgeResponse);

      const result = await edgeGatewayService.getEdgesInNamespace('production');

      expect(apiClient.get).toHaveBeenCalledWith('/namespaces/production/edges');
      expect(result.data).toHaveLength(1);
    });

    test('should return empty data when response has no data', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await edgeGatewayService.getEdgesInNamespace('empty-ns');

      expect(result).toEqual({ data: [], meta: {} });
    });

    test('should throw error on API failure', async () => {
      apiClient.get.mockRejectedValueOnce({
        response: { data: { message: 'Namespace not found' } },
      });

      await expect(edgeGatewayService.getEdgesInNamespace('invalid')).rejects.toThrow('Namespace not found');
    });

    test('should use default error message when none provided', async () => {
      apiClient.get.mockRejectedValueOnce(new Error('Network error'));

      await expect(edgeGatewayService.getEdgesInNamespace('production')).rejects.toThrow(
        'Failed to fetch edges in namespace'
      );
    });
  });

  describe('triggerConfigurationReload', () => {
    const mockReloadResponse = {
      data: {
        data: {
          type: 'reload-operations',
          id: 'push-001',
          attributes: {
            operation_id: 'push-001',
            scope: 'namespace',
            target_namespace: 'production',
            target_edges: ['edge-001', 'edge-002'],
            initiated_by: 'admin@example.com',
            initiated_at: '2026-09-30T01:00:00Z',
            deadline_at: '2026-09-30T01:05:00Z',
            status: 'in_progress',
            progress: 0,
            message: 'Push recorded for 2 edge(s); 1 connected now.',
            targets: [
              { edge_id: 'edge-001', namespace: 'production', reachable: true },
              { edge_id: 'edge-002', namespace: 'production', reachable: false, reason: 'no recent heartbeat' },
            ],
            skipped: [],
            warnings: ['1 of 2 edge(s) are not connected to the control plane; the push waits up to 5m0s for them to reconnect.'],
          },
        },
      },
    };

    test('should trigger reload for a namespace', async () => {
      apiClient.post.mockResolvedValueOnce(mockReloadResponse);

      const result = await edgeGatewayService.triggerConfigurationReload('production', 'namespace');

      expect(apiClient.post).toHaveBeenCalledWith('/namespaces/production/reload');
      expect(result).toMatchObject({
        operationId: 'push-001',
        scope: 'namespace',
        targetNamespace: 'production',
        targetEdges: ['edge-001', 'edge-002'],
        status: 'in_progress',
        deadlineAt: '2026-09-30T01:05:00Z',
        edges: null,
      });
      expect(result.targets).toEqual([
        { edgeId: 'edge-001', namespace: 'production', reachable: true, reason: '' },
        { edgeId: 'edge-002', namespace: 'production', reachable: false, reason: 'no recent heartbeat' },
      ]);
      expect(result.warnings).toHaveLength(1);
    });

    test('should trigger reload for a specific edge', async () => {
      apiClient.post.mockResolvedValueOnce(mockReloadResponse);

      await edgeGatewayService.triggerConfigurationReload('edge-001', 'edge');

      expect(apiClient.post).toHaveBeenCalledWith('/edges/edge-001/reload');
    });

    test('should default to namespace target type', async () => {
      apiClient.post.mockResolvedValueOnce(mockReloadResponse);

      await edgeGatewayService.triggerConfigurationReload('production');

      expect(apiClient.post).toHaveBeenCalledWith('/namespaces/production/reload');
    });

    test('should return null when response has no data', async () => {
      apiClient.post.mockResolvedValueOnce({ data: {} });

      const result = await edgeGatewayService.triggerConfigurationReload('production');

      expect(result).toBeNull();
    });

    test('reports the API error detail (JSON:API errors)', async () => {
      apiClient.post.mockRejectedValueOnce({
        response: { status: 409, data: { errors: [{ title: 'Conflict', detail: 'no edges to push to: no edges are registered in namespace "production"' }] } },
      });

      await expect(edgeGatewayService.triggerConfigurationReload('production')).rejects.toThrow(
        'no edges to push to: no edges are registered in namespace "production"'
      );
    });

    test('should throw error on API failure', async () => {
      apiClient.post.mockRejectedValueOnce({
        response: { data: { message: 'Reload failed' } },
      });

      await expect(edgeGatewayService.triggerConfigurationReload('production')).rejects.toThrow('Reload failed');
    });
  });

  describe('reloadAllEdges', () => {
    const mockReloadResponse = {
      data: {
        data: {
          type: 'reload-operations',
          id: 'push-all',
          attributes: {
            operation_id: 'push-all',
            scope: 'all',
            target_edges: ['edge-001'],
            status: 'in_progress',
            message: 'Push recorded for 1 edge(s).',
            skipped: [{ edge_id: 'edge-old', namespace: 'ns3', reason: 'offline since 2026-09-28T00:00:00Z' }],
            warnings: [],
          },
        },
      },
    };

    test('should trigger global reload as one operation', async () => {
      apiClient.post.mockResolvedValueOnce(mockReloadResponse);

      const result = await edgeGatewayService.reloadAllEdges();

      expect(apiClient.post).toHaveBeenCalledWith('/edges/reload-all');
      expect(result).toMatchObject({ operationId: 'push-all', scope: 'all', targetEdges: ['edge-001'], status: 'in_progress' });
      expect(result.skipped).toEqual([{ edgeId: 'edge-old', namespace: 'ns3', reason: 'offline since 2026-09-28T00:00:00Z' }]);
      expect(result.skippedTotal).toBe(1);
    });

    test('should return null when response has no data', async () => {
      apiClient.post.mockResolvedValueOnce({ data: {} });

      const result = await edgeGatewayService.reloadAllEdges();

      expect(result).toBeNull();
    });

    test('should throw error on API failure', async () => {
      apiClient.post.mockRejectedValueOnce({
        response: { data: { message: 'Global reload failed' } },
      });

      await expect(edgeGatewayService.reloadAllEdges()).rejects.toThrow('Global reload failed');
    });

    test('should use default error message when none provided', async () => {
      apiClient.post.mockRejectedValueOnce(new Error('Network error'));

      await expect(edgeGatewayService.reloadAllEdges()).rejects.toThrow('Failed to trigger global reload');
    });
  });

  describe('getReloadStatus', () => {
    const mockStatusResponse = {
      data: {
        data: {
          attributes: {
            operation_id: 'push-001',
            status: 'partially_failed',
            progress: 100,
            message: 'Push finished: 1 updated, 1 failed',
            target_namespace: 'production',
            target_edges: ['edge-001', 'edge-002'],
            counts: { succeeded: 1, failed: 1 },
            edges: [
              { edge_id: 'edge-001', status: 'succeeded', phase: 'READY', attempts: 1, max_attempts: 3 },
              { edge_id: 'edge-002', status: 'failed', phase: 'FAILED', message: 'Failed to update SQLite: disk full', attempts: 1, max_attempts: 3 },
            ],
            warnings: [],
          },
        },
      },
    };

    test('should fetch the per-edge report', async () => {
      apiClient.get.mockResolvedValueOnce(mockStatusResponse);

      const result = await edgeGatewayService.getReloadStatus('push-001');

      expect(apiClient.get).toHaveBeenCalledWith('/reload-operations/push-001/status');
      expect(result).toMatchObject({ operationId: 'push-001', status: 'partially_failed', progress: 100, counts: { succeeded: 1, failed: 1 } });
      expect(result.edges[1]).toMatchObject({ edgeId: 'edge-002', status: 'failed', message: 'Failed to update SQLite: disk full', attempts: 1, maxAttempts: 3, reachable: null });
    });

    test('should return null when response has no data', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await edgeGatewayService.getReloadStatus('op-001');

      expect(result).toBeNull();
    });

    test('should throw error on API failure', async () => {
      apiClient.get.mockRejectedValueOnce({
        response: { status: 404, data: { errors: [{ detail: 'push operation not found: op-001' }] } },
      });

      await expect(edgeGatewayService.getReloadStatus('op-001')).rejects.toThrow('push operation not found: op-001');
    });
  });

  describe('getPushProgress', () => {
    test('uses the per-edge report when available', async () => {
      apiClient.get.mockResolvedValueOnce({ data: { data: { attributes: { operation_id: 'push-1', status: 'succeeded', edges: [] } } } });

      const result = await edgeGatewayService.getPushProgress('push-1');

      expect(result.status).toBe('succeeded');
      expect(result.edges).toEqual([]);
      expect(apiClient.get).toHaveBeenCalledTimes(1);
    });

    test('falls back to the listing in Community Edition (402)', async () => {
      apiClient.get
        .mockRejectedValueOnce({ response: { status: 402, data: { errors: [{ detail: 'Enterprise Edition' }] } } })
        .mockResolvedValueOnce({
          data: {
            data: [
              { attributes: { operation_id: 'push-0', status: 'succeeded' } },
              { attributes: { operation_id: 'push-1', status: 'in_progress', progress: 50, message: 'Pushing to 2 edge(s): 1 updated, 1 reloading', counts: { succeeded: 1, sent: 1 } } },
            ],
          },
        });

      const result = await edgeGatewayService.getPushProgress('push-1');

      expect(apiClient.get).toHaveBeenLastCalledWith('/edges/reload-operations');
      expect(result).toMatchObject({ operationId: 'push-1', status: 'in_progress', progress: 50, edges: null });
    });

    test('other errors are not hidden by the fallback', async () => {
      apiClient.get.mockRejectedValueOnce({ response: { status: 500, data: { errors: [{ detail: 'database is down' }] } } });

      await expect(edgeGatewayService.getPushProgress('push-1')).rejects.toThrow('database is down');
      expect(apiClient.get).toHaveBeenCalledTimes(1);
    });
  });

  describe('deleteEdgeGateway', () => {
    test('should delete an edge gateway', async () => {
      apiClient.delete.mockResolvedValueOnce({});

      const result = await edgeGatewayService.deleteEdgeGateway('edge-001');

      expect(apiClient.delete).toHaveBeenCalledWith('/edges/edge-001');
      expect(result).toEqual({ success: true });
    });

    test('should throw error on API failure', async () => {
      apiClient.delete.mockRejectedValueOnce({
        response: { data: { errors: [{ detail: 'Cannot delete active edge' }] } },
      });

      await expect(edgeGatewayService.deleteEdgeGateway('edge-001')).rejects.toThrow('Cannot delete active edge');
    });

    test('should use default error message when none provided', async () => {
      apiClient.delete.mockRejectedValueOnce(new Error('Network error'));

      await expect(edgeGatewayService.deleteEdgeGateway('edge-001')).rejects.toThrow('Failed to delete edge gateway');
    });
  });

  describe('getConnectionStatus', () => {
    test('should return disconnected status when no heartbeat', () => {
      const result = edgeGatewayService.getConnectionStatus(null);

      expect(result).toEqual({
        status: 'disconnected',
        color: 'error',
        label: 'Disconnected',
      });
    });

    test('should return connected status for recent heartbeat (< 5 minutes)', () => {
      const recentHeartbeat = new Date(Date.now() - 2 * 60 * 1000).toISOString(); // 2 minutes ago

      const result = edgeGatewayService.getConnectionStatus(recentHeartbeat);

      expect(result).toEqual({
        status: 'connected',
        color: 'success',
        label: 'Connected',
      });
    });

    test('should return stale status for heartbeat 5-15 minutes ago', () => {
      const staleHeartbeat = new Date(Date.now() - 10 * 60 * 1000).toISOString(); // 10 minutes ago

      const result = edgeGatewayService.getConnectionStatus(staleHeartbeat);

      expect(result).toEqual({
        status: 'stale',
        color: 'warning',
        label: 'Stale',
      });
    });

    test('should return disconnected status for heartbeat > 15 minutes ago', () => {
      const oldHeartbeat = new Date(Date.now() - 20 * 60 * 1000).toISOString(); // 20 minutes ago

      const result = edgeGatewayService.getConnectionStatus(oldHeartbeat);

      expect(result).toEqual({
        status: 'disconnected',
        color: 'error',
        label: 'Disconnected',
      });
    });
  });

  describe('formatLastHeartbeat', () => {
    test('should return "Never" for null heartbeat', () => {
      const result = edgeGatewayService.formatLastHeartbeat(null);

      expect(result).toBe('Never');
    });

    test('should return "Just now" for heartbeat < 1 minute ago', () => {
      const recentHeartbeat = new Date(Date.now() - 30 * 1000).toISOString(); // 30 seconds ago

      const result = edgeGatewayService.formatLastHeartbeat(recentHeartbeat);

      expect(result).toBe('Just now');
    });

    test('should return minutes ago for heartbeat 1-59 minutes ago', () => {
      const minutesAgo = new Date(Date.now() - 30 * 60 * 1000).toISOString(); // 30 minutes ago

      const result = edgeGatewayService.formatLastHeartbeat(minutesAgo);

      expect(result).toBe('30 minutes ago');
    });

    test('should return hours ago for heartbeat 1-24 hours ago', () => {
      const hoursAgo = new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString(); // 5 hours ago

      const result = edgeGatewayService.formatLastHeartbeat(hoursAgo);

      expect(result).toBe('5 hours ago');
    });

    test('should return date for heartbeat > 24 hours ago', () => {
      const daysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000); // 2 days ago

      const result = edgeGatewayService.formatLastHeartbeat(daysAgo.toISOString());

      expect(result).toBe(daysAgo.toLocaleDateString());
    });
  });
});
