import pubClient from '../../admin/utils/pubClient';

class AgentService {
  /**
   * List all accessible agents for the current user
   */
  async listAccessibleAgents() {
    try {
      const response = await pubClient.get('/common/agents');

      if (response.data?.data) {
        return response.data.data.map(agent => ({
          id: agent.id,
          name: agent.name,
          slug: agent.slug,
          description: agent.description,
          pluginId: agent.plugin_id,
          plugin: agent.plugin,
          appId: agent.app_id,
          app: agent.app,
          isActive: agent.is_active,
          namespace: agent.namespace || '',
        }));
      }

      return [];
    } catch (error) {
      console.error('Error fetching accessible agents:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to fetch agents');
    }
  }

  /**
   * Get a single agent by ID
   */
  async getAgent(id) {
    try {
      const response = await pubClient.get(`/common/agents/${id}`);

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
          isActive: agent.is_active,
          namespace: agent.namespace || '',
        };
      }

      return null;
    } catch (error) {
      console.error('Error fetching agent:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to fetch agent');
    }
  }

  /**
   * Establish SSE connection with an agent
   * Returns the EventSource for streaming
   */
  async connectToAgent(agentId, sessionId = '') {
    const token = localStorage.getItem('token');
    const baseURL = pubClient.defaults.baseURL || '';

    // EventSource doesn't support headers, so we need to pass token in URL
    // Note: This is not ideal for production, consider using a different auth mechanism
    const params = new URLSearchParams();
    if (sessionId) {
      params.append('session_id', sessionId);
    }
    params.append('token', token);

    const url = `${baseURL}/common/agents/${agentId}/stream?${params.toString()}`;

    const eventSource = new EventSource(url);

    return eventSource;
  }

  /**
   * Send a message to an agent session via POST
   */
  async sendMessage(agentId, message, history = [], sessionId) {
    try {
      // Send session_id as query parameter (consistent with chat handler)
      const response = await pubClient.post(`/common/agents/${agentId}/message?session_id=${sessionId}`, {
        message,
        history,
        // session_id no longer needed in body
      });

      return response.data;
    } catch (error) {
      console.error('Error sending message to agent:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to send message');
    }
  }
}

export default new AgentService();
