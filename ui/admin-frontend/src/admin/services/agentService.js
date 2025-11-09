import apiClient from '../utils/apiClient';

class AgentService {
  /**
   * List all agent configurations with pagination and filtering
   */
  async listAgents(page = 1, limit = 25, namespace = '', isActive) {
    try {
      const params = {
        page,
        limit,
        ...(namespace && { namespace }),
        ...(isActive !== undefined && { is_active: isActive }),
      };

      const response = await apiClient.get('/agents', { params });

      if (response.data?.data) {
        return {
          data: response.data.data.map(agent => ({
            id: agent.id,
            name: agent.name,
            slug: agent.slug,
            description: agent.description,
            pluginId: agent.plugin_id,
            plugin: agent.plugin,
            appId: agent.app_id,
            app: agent.app,
            config: agent.config || {},
            groups: agent.groups || [],
            isActive: agent.is_active,
            namespace: agent.namespace || '',
            createdAt: agent.created_at,
            updatedAt: agent.updated_at,
          })),
          meta: response.data.meta || {},
        };
      }

      return { data: [], meta: {} };
    } catch (error) {
      console.error('Error fetching agents:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to fetch agents');
    }
  }

  /**
   * Get a single agent configuration by ID
   */
  async getAgent(id) {
    try {
      const response = await apiClient.get(`/agents/${id}`);

      if (response.data?.data) {
        const agent = response.data.data;
        return {
          id: agent.id,
          name: agent.name,
          slug: agent.slug,
          description: agent.description,
          pluginId: agent.plugin_id,
          plugin: agent.plugin,
          appId: agent.app_id,
          app: agent.app,
          config: agent.config || {},
          groups: agent.groups || [],
          isActive: agent.is_active,
          namespace: agent.namespace || '',
          createdAt: agent.created_at,
          updatedAt: agent.updated_at,
        };
      }

      return null;
    } catch (error) {
      console.error('Error fetching agent:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to fetch agent');
    }
  }

  /**
   * Create a new agent configuration
   */
  async createAgent(agentData) {
    try {
      const payload = {
        name: agentData.name,
        description: agentData.description || '',
        plugin_id: parseInt(agentData.pluginId, 10),
        app_id: parseInt(agentData.appId, 10),
        config: agentData.config || {},
        group_ids: (agentData.groupIds || []).map(id => parseInt(id, 10)),
        is_active: agentData.isActive !== undefined ? agentData.isActive : true,
        namespace: agentData.namespace || '',
      };

      const response = await apiClient.post('/agents', payload);

      if (response.data?.data) {
        const agent = response.data.data;
        return {
          id: agent.id,
          name: agent.name,
          slug: agent.slug,
          description: agent.description,
          pluginId: agent.plugin_id,
          plugin: agent.plugin,
          appId: agent.app_id,
          app: agent.app,
          config: agent.config || {},
          groups: agent.groups || [],
          isActive: agent.is_active,
          namespace: agent.namespace || '',
          createdAt: agent.created_at,
          updatedAt: agent.updated_at,
        };
      }

      return null;
    } catch (error) {
      console.error('Error creating agent:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to create agent');
    }
  }

  /**
   * Update an existing agent configuration
   */
  async updateAgent(id, agentData) {
    try {
      const payload = {
        name: agentData.name,
        description: agentData.description || '',
        plugin_id: parseInt(agentData.pluginId, 10),
        app_id: parseInt(agentData.appId, 10),
        config: agentData.config || {},
        group_ids: (agentData.groupIds || []).map(id => parseInt(id, 10)),
        is_active: agentData.isActive !== undefined ? agentData.isActive : true,
        namespace: agentData.namespace || '',
      };

      const response = await apiClient.put(`/agents/${id}`, payload);

      if (response.data?.data) {
        const agent = response.data.data;
        return {
          id: agent.id,
          name: agent.name,
          slug: agent.slug,
          description: agent.description,
          pluginId: agent.plugin_id,
          plugin: agent.plugin,
          appId: agent.app_id,
          app: agent.app,
          config: agent.config || {},
          groups: agent.groups || [],
          isActive: agent.is_active,
          namespace: agent.namespace || '',
          createdAt: agent.created_at,
          updatedAt: agent.updated_at,
        };
      }

      return null;
    } catch (error) {
      console.error('Error updating agent:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to update agent');
    }
  }

  /**
   * Delete an agent configuration
   */
  async deleteAgent(id) {
    try {
      await apiClient.delete(`/agents/${id}`);
      return true;
    } catch (error) {
      console.error('Error deleting agent:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to delete agent');
    }
  }

  /**
   * Activate an agent configuration
   */
  async activateAgent(id) {
    try {
      await apiClient.post(`/agents/${id}/activate`);
      return true;
    } catch (error) {
      console.error('Error activating agent:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to activate agent');
    }
  }

  /**
   * Deactivate an agent configuration
   */
  async deactivateAgent(id) {
    try {
      await apiClient.post(`/agents/${id}/deactivate`);
      return true;
    } catch (error) {
      console.error('Error deactivating agent:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to deactivate agent');
    }
  }

  /**
   * Send a message to an agent (for testing purposes)
   * Returns an EventSource for SSE streaming
   */
  createMessageStream(agentId, message, history = [], sessionId = '') {
    const token = localStorage.getItem('token');
    const payload = {
      message,
      history,
      session_id: sessionId,
    };

    // Create EventSource for SSE
    const eventSource = new EventSource(
      `${apiClient.defaults.baseURL}/agents/${agentId}/message`,
      {
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      }
    );

    // Note: EventSource doesn't support POST with body directly
    // We need to use fetch API for POST, then read the stream
    return this.sendMessageWithSSE(agentId, payload);
  }

  /**
   * Internal method to send message with SSE using fetch
   */
  async sendMessageWithSSE(agentId, payload) {
    const token = localStorage.getItem('token');
    const baseURL = apiClient.defaults.baseURL || '';

    const response = await fetch(`${baseURL}/agents/${agentId}/message`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      throw new Error(`Failed to send message: ${response.statusText}`);
    }

    return response.body;
  }
}

export default new AgentService();
