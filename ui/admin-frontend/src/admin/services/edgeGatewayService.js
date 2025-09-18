import apiClient from '../utils/apiClient';

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

  async triggerConfigurationReload(namespace, targetType = 'namespace') {
    try {
      const endpoint = targetType === 'namespace' 
        ? `/namespaces/${namespace}/reload`
        : `/edges/${namespace}/reload`; // namespace is actually edge ID in this case
      
      const response = await apiClient.post(endpoint);
      
      if (response.data?.data) {
        const operation = response.data.data;
        return {
          operationId: operation.attributes.operation_id,
          targetNamespace: operation.attributes.target_namespace,
          status: operation.attributes.status,
          message: operation.attributes.message,
        };
      }
      
      return null;
    } catch (error) {
      console.error('Error triggering configuration reload:', error);
      throw new Error(error.response?.data?.message || 'Failed to trigger configuration reload');
    }
  }

  async getReloadStatus(operationId) {
    try {
      const response = await apiClient.get(`/reload-operations/${operationId}/status`);
      
      if (response.data?.data) {
        const operation = response.data.data;
        return {
          operationId: operation.attributes.operation_id,
          status: operation.attributes.status,
          progress: operation.attributes.progress,
          message: operation.attributes.message,
          targetNamespace: operation.attributes.target_namespace,
          targetEdges: operation.attributes.target_edges || [],
          initiatedBy: operation.attributes.initiated_by,
          initiatedAt: operation.attributes.initiated_at,
        };
      }
      
      return null;
    } catch (error) {
      console.error('Error fetching reload status:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch reload status');
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
}

export default new EdgeGatewayService();