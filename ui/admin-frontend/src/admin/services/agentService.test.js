import agentService from './agentService';
import apiClient from '../utils/apiClient';

// Mock apiClient
jest.mock('../utils/apiClient');

describe('AgentService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore?.();
  });

  describe('listAgents', () => {
    const mockAgentResponse = {
      data: {
        data: [
          {
            id: 1,
            name: 'Test Agent',
            slug: 'test-agent',
            description: 'A test agent',
            plugin_id: 10,
            plugin: { id: 10, name: 'Agent Plugin' },
            app_id: 5,
            app: { id: 5, name: 'Test App' },
            config: { setting: 'value' },
            groups: [{ id: 1, name: 'Group 1' }],
            is_active: true,
            namespace: 'test-ns',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-02T00:00:00Z',
          },
        ],
        meta: { total: 1, page: 1 },
      },
    };

    test('should fetch agents with default parameters', async () => {
      apiClient.get.mockResolvedValueOnce(mockAgentResponse);

      const result = await agentService.listAgents();

      expect(apiClient.get).toHaveBeenCalledWith('/agents', {
        params: { page: 1, limit: 25 },
      });
      expect(result.data).toHaveLength(1);
      expect(result.data[0]).toEqual({
        id: 1,
        name: 'Test Agent',
        slug: 'test-agent',
        description: 'A test agent',
        pluginId: 10,
        plugin: { id: 10, name: 'Agent Plugin' },
        appId: 5,
        app: { id: 5, name: 'Test App' },
        config: { setting: 'value' },
        groups: [{ id: 1, name: 'Group 1' }],
        isActive: true,
        namespace: 'test-ns',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-02T00:00:00Z',
      });
    });

    test('should fetch agents with custom parameters', async () => {
      apiClient.get.mockResolvedValueOnce(mockAgentResponse);

      await agentService.listAgents(2, 50, 'custom-ns', true);

      expect(apiClient.get).toHaveBeenCalledWith('/agents', {
        params: { page: 2, limit: 50, namespace: 'custom-ns', is_active: true },
      });
    });

    test('should return empty data when response has no data', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await agentService.listAgents();

      expect(result).toEqual({ data: [], meta: {} });
    });

    test('should throw error on API failure', async () => {
      apiClient.get.mockRejectedValueOnce({
        response: { data: { errors: [{ detail: 'Server error' }] } },
      });

      await expect(agentService.listAgents()).rejects.toThrow('Server error');
    });

    test('should use default error message when none provided', async () => {
      apiClient.get.mockRejectedValueOnce(new Error('Network error'));

      await expect(agentService.listAgents()).rejects.toThrow('Failed to fetch agents');
    });
  });

  describe('getAgent', () => {
    const mockAgentResponse = {
      data: {
        data: {
          id: 1,
          name: 'Test Agent',
          slug: 'test-agent',
          description: 'A test agent',
          plugin_id: 10,
          plugin: { id: 10, name: 'Agent Plugin' },
          app_id: 5,
          app: { id: 5, name: 'Test App' },
          config: { setting: 'value' },
          groups: [{ id: 1, name: 'Group 1' }],
          is_active: true,
          namespace: 'test-ns',
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-02T00:00:00Z',
        },
      },
    };

    test('should fetch a single agent by ID', async () => {
      apiClient.get.mockResolvedValueOnce(mockAgentResponse);

      const result = await agentService.getAgent(1);

      expect(apiClient.get).toHaveBeenCalledWith('/agents/1');
      expect(result.id).toBe(1);
      expect(result.name).toBe('Test Agent');
    });

    test('should return null when agent not found', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await agentService.getAgent('nonexistent');

      expect(result).toBeNull();
    });

    test('should handle missing optional fields', async () => {
      const responseWithMissingFields = {
        data: {
          data: {
            id: 1,
            name: 'Minimal Agent',
            slug: 'minimal-agent',
            description: '',
            plugin_id: 10,
            plugin: null,
            app_id: 5,
            app: null,
            config: null,
            groups: null,
            is_active: false,
            namespace: null,
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        },
      };
      apiClient.get.mockResolvedValueOnce(responseWithMissingFields);

      const result = await agentService.getAgent(1);

      expect(result.config).toEqual({});
      expect(result.groups).toEqual([]);
      expect(result.namespace).toBe('');
    });

    test('should throw error on API failure', async () => {
      apiClient.get.mockRejectedValueOnce({
        response: { data: { errors: [{ detail: 'Agent not found' }] } },
      });

      await expect(agentService.getAgent(1)).rejects.toThrow('Agent not found');
    });
  });

  describe('createAgent', () => {
    const mockAgentData = {
      name: 'New Agent',
      description: 'A new agent',
      pluginId: '10',
      appId: '5',
      config: { setting: 'value' },
      groupIds: ['1', '2'],
      isActive: true,
      namespace: 'prod',
    };

    const mockResponse = {
      data: {
        data: {
          id: 2,
          name: 'New Agent',
          slug: 'new-agent',
          description: 'A new agent',
          plugin_id: 10,
          plugin: { id: 10, name: 'Plugin' },
          app_id: 5,
          app: { id: 5, name: 'App' },
          config: { setting: 'value' },
          groups: [{ id: 1 }, { id: 2 }],
          is_active: true,
          namespace: 'prod',
          created_at: '2024-01-03T00:00:00Z',
          updated_at: '2024-01-03T00:00:00Z',
        },
      },
    };

    test('should create a new agent', async () => {
      apiClient.post.mockResolvedValueOnce(mockResponse);

      const result = await agentService.createAgent(mockAgentData);

      expect(apiClient.post).toHaveBeenCalledWith('/agents', {
        name: 'New Agent',
        description: 'A new agent',
        plugin_id: 10,
        app_id: 5,
        config: { setting: 'value' },
        group_ids: [1, 2],
        is_active: true,
        namespace: 'prod',
      });
      expect(result.id).toBe(2);
      expect(result.name).toBe('New Agent');
    });

    test('should use default values for optional fields', async () => {
      apiClient.post.mockResolvedValueOnce(mockResponse);

      await agentService.createAgent({
        name: 'Minimal Agent',
        pluginId: '10',
        appId: '5',
      });

      expect(apiClient.post).toHaveBeenCalledWith('/agents', {
        name: 'Minimal Agent',
        description: '',
        plugin_id: 10,
        app_id: 5,
        config: {},
        group_ids: [],
        is_active: true,
        namespace: '',
      });
    });

    test('should return null when response has no data', async () => {
      apiClient.post.mockResolvedValueOnce({ data: {} });

      const result = await agentService.createAgent(mockAgentData);

      expect(result).toBeNull();
    });

    test('should throw error on API failure', async () => {
      apiClient.post.mockRejectedValueOnce({
        response: { data: { errors: [{ detail: 'Validation failed' }] } },
      });

      await expect(agentService.createAgent(mockAgentData)).rejects.toThrow('Validation failed');
    });
  });

  describe('updateAgent', () => {
    const mockAgentData = {
      name: 'Updated Agent',
      description: 'Updated description',
      pluginId: '10',
      appId: '5',
      config: { newSetting: 'newValue' },
      groupIds: ['1'],
      isActive: false,
      namespace: 'dev',
    };

    const mockResponse = {
      data: {
        data: {
          id: 1,
          name: 'Updated Agent',
          slug: 'updated-agent',
          description: 'Updated description',
          plugin_id: 10,
          plugin: null,
          app_id: 5,
          app: null,
          config: { newSetting: 'newValue' },
          groups: [{ id: 1 }],
          is_active: false,
          namespace: 'dev',
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-04T00:00:00Z',
        },
      },
    };

    test('should update an existing agent', async () => {
      apiClient.put.mockResolvedValueOnce(mockResponse);

      const result = await agentService.updateAgent(1, mockAgentData);

      expect(apiClient.put).toHaveBeenCalledWith('/agents/1', expect.objectContaining({
        name: 'Updated Agent',
        is_active: false,
        namespace: 'dev',
      }));
      expect(result.name).toBe('Updated Agent');
      expect(result.isActive).toBe(false);
    });

    test('should return null when response has no data', async () => {
      apiClient.put.mockResolvedValueOnce({ data: {} });

      const result = await agentService.updateAgent(1, mockAgentData);

      expect(result).toBeNull();
    });

    test('should throw error on API failure', async () => {
      apiClient.put.mockRejectedValueOnce({
        response: { data: { errors: [{ detail: 'Update failed' }] } },
      });

      await expect(agentService.updateAgent(1, mockAgentData)).rejects.toThrow('Update failed');
    });
  });

  describe('deleteAgent', () => {
    test('should delete an agent', async () => {
      apiClient.delete.mockResolvedValueOnce({});

      const result = await agentService.deleteAgent(1);

      expect(apiClient.delete).toHaveBeenCalledWith('/agents/1');
      expect(result).toBe(true);
    });

    test('should throw error on API failure', async () => {
      apiClient.delete.mockRejectedValueOnce({
        response: { data: { errors: [{ detail: 'Delete failed' }] } },
      });

      await expect(agentService.deleteAgent(1)).rejects.toThrow('Delete failed');
    });
  });

  describe('activateAgent', () => {
    test('should activate an agent', async () => {
      apiClient.post.mockResolvedValueOnce({});

      const result = await agentService.activateAgent(1);

      expect(apiClient.post).toHaveBeenCalledWith('/agents/1/activate');
      expect(result).toBe(true);
    });

    test('should throw error on API failure', async () => {
      apiClient.post.mockRejectedValueOnce({
        response: { data: { errors: [{ detail: 'Activation failed' }] } },
      });

      await expect(agentService.activateAgent(1)).rejects.toThrow('Activation failed');
    });
  });

  describe('deactivateAgent', () => {
    test('should deactivate an agent', async () => {
      apiClient.post.mockResolvedValueOnce({});

      const result = await agentService.deactivateAgent(1);

      expect(apiClient.post).toHaveBeenCalledWith('/agents/1/deactivate');
      expect(result).toBe(true);
    });

    test('should throw error on API failure', async () => {
      apiClient.post.mockRejectedValueOnce({
        response: { data: { errors: [{ detail: 'Deactivation failed' }] } },
      });

      await expect(agentService.deactivateAgent(1)).rejects.toThrow('Deactivation failed');
    });
  });

  describe('sendMessageWithSSE', () => {
    beforeEach(() => {
      // Mock localStorage
      Object.defineProperty(window, 'localStorage', {
        value: {
          getItem: jest.fn(() => 'test-token'),
        },
        writable: true,
      });

      // Mock fetch
      global.fetch = jest.fn();

      // Mock apiClient.defaults
      apiClient.defaults = { baseURL: 'http://localhost:8080' };
    });

    afterEach(() => {
      delete global.fetch;
    });

    test('should send message with correct payload', async () => {
      const mockBody = { getReader: jest.fn() }; // Mock ReadableStream-like object
      const mockResponse = {
        ok: true,
        body: mockBody,
      };
      global.fetch.mockResolvedValueOnce(mockResponse);

      const payload = {
        message: 'Hello',
        history: [],
        session_id: 'session-123',
      };

      await agentService.sendMessageWithSSE(1, payload);

      expect(global.fetch).toHaveBeenCalledWith(
        'http://localhost:8080/agents/1/message',
        {
          method: 'POST',
          headers: {
            'Authorization': 'Bearer test-token',
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(payload),
        }
      );
    });

    test('should throw error on non-ok response', async () => {
      const mockResponse = {
        ok: false,
        statusText: 'Internal Server Error',
      };
      global.fetch.mockResolvedValueOnce(mockResponse);

      await expect(
        agentService.sendMessageWithSSE(1, { message: 'Hello', history: [], session_id: '' })
      ).rejects.toThrow('Failed to send message: Internal Server Error');
    });

    test('should return response body on success', async () => {
      const mockBody = { getReader: jest.fn() }; // Mock ReadableStream-like object
      const mockResponse = {
        ok: true,
        body: mockBody,
      };
      global.fetch.mockResolvedValueOnce(mockResponse);

      const result = await agentService.sendMessageWithSSE(1, {
        message: 'Hello',
        history: [],
        session_id: '',
      });

      expect(result).toBe(mockBody);
    });
  });
});
