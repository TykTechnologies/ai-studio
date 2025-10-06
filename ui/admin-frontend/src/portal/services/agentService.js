import pubClient from '../../admin/utils/pubClient';

class AgentService {
  /**
   * List all accessible agents for the current user
   */
  async listAccessibleAgents() {
    try {
      const response = await pubClient.get('/agents');

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
      const response = await pubClient.get(`/agents/${id}`);

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
   * Send a message to an agent via SSE
   * Returns a readable stream
   */
  async sendMessage(agentId, message, history = [], sessionId = '') {
    try {
      const token = localStorage.getItem('token');
      const baseURL = pubClient.defaults.baseURL || '/api/v1';

      const response = await fetch(`${baseURL}/agents/${agentId}/message`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          message,
          history,
          session_id: sessionId,
        }),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.errors?.[0]?.detail || 'Failed to send message');
      }

      return response.body;
    } catch (error) {
      console.error('Error sending message to agent:', error);
      throw error;
    }
  }
}

export default new AgentService();
