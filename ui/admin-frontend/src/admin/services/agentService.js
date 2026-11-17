import apiClient from '../utils/apiClient';

// fromResponse maps an agent config from the API to the shape the admin UI
// uses. workspaceId is the agent's Workspace (Enterprise; undefined on
// Community Edition).
const fromResponse = (agent) => ({
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
  workspaceId: agent.workspace_id,
  createdAt: agent.created_at,
  updatedAt: agent.updated_at,
});

// failure wraps an API error in an Error with the API's detail as message,
// keeping status and response so callers can tell a 404 (an agent in
// another Workspace looks missing) and read field errors (workspaceFieldError).
const failure = (error, fallback) => {
  const err = new Error(error.response?.data?.errors?.[0]?.detail || fallback);
  err.status = error.response?.status;
  err.response = error.response;
  return err;
};

// workspaceAttr: workspace_id is sent only to choose a Workspace (create)
// or to move the agent (edit); the caller decides and passes workspaceId.
const workspaceAttr = (agentData) =>
  agentData.workspaceId !== undefined && agentData.workspaceId !== null
    ? { workspace_id: Number(agentData.workspaceId) }
    : {};

class AgentService {
  /**
   * List all agent configurations with pagination and filtering
   */
  async listAgents(page = 1, limit = 25, namespace = '', isActive, extraParams = {}) {
    try {
      const params = {
        page,
        limit,
        ...(namespace && { namespace }),
        ...(isActive !== undefined && { is_active: isActive }),
        ...extraParams, // e.g. the Workspace filter ({ workspace_id })
      };

      const response = await apiClient.get('/agents', { params });

      if (response.data?.data) {
        return {
          data: response.data.data.map(fromResponse),
          meta: response.data.meta || {},
        };
      }

      return { data: [], meta: {} };
    } catch (error) {
      console.error('Error fetching agents:', error);
      throw failure(error, 'Failed to fetch agents');
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
        return fromResponse(agent);
      }

      return null;
    } catch (error) {
      console.error('Error fetching agent:', error);
      throw failure(error, 'Failed to fetch agent');
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
        ...workspaceAttr(agentData),
      };

      const response = await apiClient.post('/agents', payload);

      if (response.data?.data) {
        const agent = response.data.data;
        return fromResponse(agent);
      }

      return null;
    } catch (error) {
      console.error('Error creating agent:', error);
      throw failure(error, 'Failed to create agent');
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
        ...workspaceAttr(agentData),
      };

      const response = await apiClient.put(`/agents/${id}`, payload);

      if (response.data?.data) {
        const agent = response.data.data;
        return fromResponse(agent);
      }

      return null;
    } catch (error) {
      console.error('Error updating agent:', error);
      throw failure(error, 'Failed to update agent');
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
