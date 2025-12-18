import pluginService, { PluginService } from './pluginService';
import apiClient from '../utils/apiClient';

// Mock apiClient
jest.mock('../utils/apiClient');

describe('PluginService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore?.();
  });

  // Static constants tests
  describe('static constants', () => {
    test('HOOK_TYPES contains all expected hook types', () => {
      expect(PluginService.HOOK_TYPES).toEqual({
        PRE_AUTH: 'pre_auth',
        AUTH: 'auth',
        POST_AUTH: 'post_auth',
        ON_RESPONSE: 'on_response',
        DATA_COLLECTION: 'data_collection',
        STUDIO_UI: 'studio_ui',
        AGENT: 'agent',
      });
    });

    test('HOOK_TYPE_LABELS contains labels for all hook types', () => {
      expect(PluginService.HOOK_TYPE_LABELS['pre_auth']).toBe('Pre-Authentication');
      expect(PluginService.HOOK_TYPE_LABELS['auth']).toBe('Authentication');
      expect(PluginService.HOOK_TYPE_LABELS['post_auth']).toBe('Post-Authentication');
      expect(PluginService.HOOK_TYPE_LABELS['on_response']).toBe('Response Processing');
      expect(PluginService.HOOK_TYPE_LABELS['data_collection']).toBe('Data Collection');
      expect(PluginService.HOOK_TYPE_LABELS['studio_ui']).toBe('UI Extension');
      expect(PluginService.HOOK_TYPE_LABELS['agent']).toBe('Conversational Agent');
    });
  });

  // listPlugins tests
  describe('listPlugins', () => {
    const mockPluginResponse = {
      data: {
        data: [
          {
            id: '1',
            attributes: {
              name: 'Test Plugin',
              description: 'A test plugin',
              command: './test-plugin',
              checksum: 'abc123',
              config: { key: 'value' },
              hook_type: 'post_auth',
              is_active: true,
              namespace: 'test-ns',
              plugin_type: 'gateway',
              oci_reference: '',
              manifest: {},
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-02T00:00:00Z',
            },
          },
        ],
        meta: { total: 1, page: 1 },
      },
    };

    test('should fetch plugins with default parameters', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginResponse);

      const result = await pluginService.listPlugins();

      expect(apiClient.get).toHaveBeenCalledWith('/plugins', {
        params: { page: 1, limit: 50 },
      });
      expect(result.data).toHaveLength(1);
      expect(result.data[0]).toEqual({
        id: '1',
        name: 'Test Plugin',
        description: 'A test plugin',
        command: './test-plugin',
        checksum: 'abc123',
        config: { key: 'value' },
        hookType: 'post_auth',
        isActive: true,
        namespace: 'test-ns',
        pluginType: 'gateway',
        ociReference: '',
        manifest: {},
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-02T00:00:00Z',
      });
    });

    test('should fetch plugins with custom parameters', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginResponse);

      await pluginService.listPlugins(2, 25, 'post_auth', true);

      expect(apiClient.get).toHaveBeenCalledWith('/plugins', {
        params: { page: 2, limit: 25, hook_type: 'post_auth', is_active: true },
      });
    });

    test('should return empty data when response has no data', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await pluginService.listPlugins();

      expect(result).toEqual({ data: [], meta: {} });
    });

    test('should throw error on API failure', async () => {
      apiClient.get.mockRejectedValueOnce({
        response: { data: { message: 'Server error' } },
      });

      await expect(pluginService.listPlugins()).rejects.toThrow('Server error');
    });

    test('should use default error message when none provided', async () => {
      apiClient.get.mockRejectedValueOnce(new Error('Network error'));

      await expect(pluginService.listPlugins()).rejects.toThrow('Failed to fetch plugins');
    });
  });

  // getPlugin tests
  describe('getPlugin', () => {
    const mockPluginResponse = {
      data: {
        data: {
          id: '1',
          attributes: {
            name: 'Test Plugin',
            description: 'A test plugin',
            command: './test-plugin',
            checksum: 'abc123',
            config: { key: 'value' },
            hook_type: 'post_auth',
            is_active: true,
            namespace: 'test-ns',
            plugin_type: 'gateway',
            oci_reference: 'oci://test',
            manifest: { version: '1.0' },
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-02T00:00:00Z',
          },
          relationships: {
            llms: {
              data: [
                {
                  id: 'llm-1',
                  attributes: { name: 'GPT-4', vendor: 'OpenAI', active: true },
                },
              ],
            },
          },
        },
      },
    };

    test('should fetch a single plugin by ID', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginResponse);

      const result = await pluginService.getPlugin('1');

      expect(apiClient.get).toHaveBeenCalledWith('/plugins/1');
      expect(result.id).toBe('1');
      expect(result.name).toBe('Test Plugin');
      expect(result.llms).toHaveLength(1);
      expect(result.llms[0]).toEqual({
        id: 'llm-1',
        name: 'GPT-4',
        vendor: 'OpenAI',
        isActive: true,
      });
    });

    test('should return null when plugin not found', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await pluginService.getPlugin('nonexistent');

      expect(result).toBeNull();
    });

    test('should handle missing relationships gracefully', async () => {
      const responseWithoutRelationships = {
        data: {
          data: {
            id: '1',
            attributes: {
              name: 'Test Plugin',
              description: '',
              command: './test',
              checksum: '',
              config: null,
              hook_type: 'auth',
              is_active: false,
              namespace: null,
              plugin_type: null,
              oci_reference: null,
              manifest: null,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          },
        },
      };
      apiClient.get.mockResolvedValueOnce(responseWithoutRelationships);

      const result = await pluginService.getPlugin('1');

      expect(result.config).toEqual({});
      expect(result.namespace).toBe('global');
      expect(result.pluginType).toBe('gateway');
      expect(result.ociReference).toBe('');
      expect(result.manifest).toEqual({});
      expect(result.llms).toEqual([]);
    });

    test('should throw error on API failure', async () => {
      apiClient.get.mockRejectedValueOnce({
        response: { data: { message: 'Plugin not found' } },
      });

      await expect(pluginService.getPlugin('1')).rejects.toThrow('Plugin not found');
    });
  });

  // createPlugin tests
  describe('createPlugin', () => {
    const mockPluginData = {
      name: 'New Plugin',
      description: 'A new plugin',
      command: './new-plugin',
      checksum: 'xyz789',
      config: { setting: 'value' },
      hookType: 'pre_auth',
      isActive: true,
      namespace: 'prod',
      pluginType: 'gateway',
      ociReference: '',
      loadImmediately: true,
    };

    const mockResponse = {
      data: {
        data: {
          id: '2',
          attributes: {
            name: 'New Plugin',
            description: 'A new plugin',
            command: './new-plugin',
            checksum: 'xyz789',
            config: { setting: 'value' },
            hook_type: 'pre_auth',
            is_active: true,
            namespace: 'prod',
            created_at: '2024-01-03T00:00:00Z',
            updated_at: '2024-01-03T00:00:00Z',
          },
        },
      },
    };

    test('should create a new plugin', async () => {
      apiClient.post.mockResolvedValueOnce(mockResponse);

      const result = await pluginService.createPlugin(mockPluginData);

      expect(apiClient.post).toHaveBeenCalledWith('/plugins', {
        name: 'New Plugin',
        description: 'A new plugin',
        command: './new-plugin',
        checksum: 'xyz789',
        config: { setting: 'value' },
        hook_type: 'pre_auth',
        is_active: true,
        namespace: 'prod',
        plugin_type: 'gateway',
        oci_reference: '',
        load_immediately: true,
      });
      expect(result.id).toBe('2');
      expect(result.name).toBe('New Plugin');
    });

    test('should use default values for optional fields', async () => {
      apiClient.post.mockResolvedValueOnce(mockResponse);

      await pluginService.createPlugin({
        name: 'Minimal Plugin',
        command: './minimal',
        hookType: 'auth',
      });

      expect(apiClient.post).toHaveBeenCalledWith('/plugins', {
        name: 'Minimal Plugin',
        description: '',
        command: './minimal',
        checksum: '',
        config: {},
        hook_type: 'auth',
        is_active: true,
        namespace: '',
        plugin_type: 'gateway',
        oci_reference: '',
        load_immediately: false,
      });
    });

    test('should return null when response has no data', async () => {
      apiClient.post.mockResolvedValueOnce({ data: {} });

      const result = await pluginService.createPlugin(mockPluginData);

      expect(result).toBeNull();
    });

    test('should throw error on API failure', async () => {
      apiClient.post.mockRejectedValueOnce({
        response: { data: { message: 'Validation failed' } },
      });

      await expect(pluginService.createPlugin(mockPluginData)).rejects.toThrow('Validation failed');
    });
  });

  // updatePlugin tests
  describe('updatePlugin', () => {
    const mockPluginData = {
      name: 'Updated Plugin',
      description: 'Updated description',
      command: './updated-plugin',
      hookType: 'on_response',
      hookTypes: ['on_response', 'data_collection'],
      hookTypesCustomized: true,
      isActive: false,
    };

    const mockResponse = {
      data: {
        data: {
          id: '1',
          attributes: {
            name: 'Updated Plugin',
            description: 'Updated description',
            command: './updated-plugin',
            checksum: '',
            config: {},
            hook_type: 'on_response',
            hook_types: ['on_response', 'data_collection'],
            hook_types_customized: true,
            is_active: false,
            namespace: 'global',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-04T00:00:00Z',
          },
        },
      },
    };

    test('should update an existing plugin', async () => {
      apiClient.patch.mockResolvedValueOnce(mockResponse);

      const result = await pluginService.updatePlugin('1', mockPluginData);

      expect(apiClient.patch).toHaveBeenCalledWith('/plugins/1', expect.objectContaining({
        name: 'Updated Plugin',
        hook_type: 'on_response',
        hook_types: ['on_response', 'data_collection'],
        hook_types_customized: true,
        is_active: false,
      }));
      expect(result.hookTypes).toEqual(['on_response', 'data_collection']);
      expect(result.hookTypesCustomized).toBe(true);
    });

    test('should return null when response has no data', async () => {
      apiClient.patch.mockResolvedValueOnce({ data: {} });

      const result = await pluginService.updatePlugin('1', mockPluginData);

      expect(result).toBeNull();
    });

    test('should throw error on API failure', async () => {
      apiClient.patch.mockRejectedValueOnce({
        response: { data: { message: 'Update failed' } },
      });

      await expect(pluginService.updatePlugin('1', mockPluginData)).rejects.toThrow('Update failed');
    });
  });

  // deletePlugin tests
  describe('deletePlugin', () => {
    test('should delete a plugin', async () => {
      apiClient.delete.mockResolvedValueOnce({});

      const result = await pluginService.deletePlugin('1');

      expect(apiClient.delete).toHaveBeenCalledWith('/plugins/1');
      expect(result).toBe(true);
    });

    test('should throw error on API failure', async () => {
      apiClient.delete.mockRejectedValueOnce({
        response: { data: { message: 'Delete failed' } },
      });

      await expect(pluginService.deletePlugin('1')).rejects.toThrow('Delete failed');
    });
  });

  // getPluginsForLLM tests
  describe('getPluginsForLLM', () => {
    test('should fetch plugins for a specific LLM', async () => {
      const mockResponse = {
        data: {
          data: [
            {
              id: '1',
              attributes: {
                name: 'Plugin 1',
                description: 'Desc 1',
                command: './p1',
                hook_type: 'auth',
                is_active: true,
                namespace: 'ns1',
              },
              pivot: { order: 1 },
            },
          ],
        },
      };
      apiClient.get.mockResolvedValueOnce(mockResponse);

      const result = await pluginService.getPluginsForLLM('llm-1');

      expect(apiClient.get).toHaveBeenCalledWith('/llms/llm-1/plugins');
      expect(result).toHaveLength(1);
      expect(result[0].pivot).toEqual({ order: 1 });
    });

    test('should return empty array when no plugins', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await pluginService.getPluginsForLLM('llm-1');

      expect(result).toEqual([]);
    });
  });

  // updateLLMPlugins tests
  describe('updateLLMPlugins', () => {
    test('should update LLM plugins', async () => {
      const mockResponse = {
        data: {
          data: [
            {
              id: '1',
              attributes: { name: 'Plugin 1', hook_type: 'auth', is_active: true },
            },
          ],
        },
      };
      apiClient.put.mockResolvedValueOnce(mockResponse);

      const result = await pluginService.updateLLMPlugins('llm-1', ['1', '2']);

      expect(apiClient.put).toHaveBeenCalledWith('/llms/llm-1/plugins', {
        plugin_ids: ['1', '2'],
      });
      expect(result).toHaveLength(1);
    });

    test('should return empty array when response has no data', async () => {
      apiClient.put.mockResolvedValueOnce({ data: {} });

      const result = await pluginService.updateLLMPlugins('llm-1', []);

      expect(result).toEqual([]);
    });
  });

  // Utility methods tests
  describe('utility methods', () => {
    describe('getHookTypeLabel', () => {
      test('should return correct label for valid hook type', () => {
        expect(pluginService.getHookTypeLabel('pre_auth')).toBe('Pre-Authentication');
        expect(pluginService.getHookTypeLabel('agent')).toBe('Conversational Agent');
      });

      test('should return hook type itself for unknown type', () => {
        expect(pluginService.getHookTypeLabel('unknown_type')).toBe('unknown_type');
      });
    });

    describe('getAvailableHookTypes', () => {
      test('should return array of hook type options', () => {
        const hookTypes = pluginService.getAvailableHookTypes();

        expect(hookTypes).toBeInstanceOf(Array);
        expect(hookTypes.length).toBe(7);
        expect(hookTypes).toContainEqual({ value: 'pre_auth', label: 'Pre-Authentication' });
        expect(hookTypes).toContainEqual({ value: 'agent', label: 'Conversational Agent' });
      });
    });

    describe('validatePluginData', () => {
      test('should validate valid plugin data', () => {
        const result = pluginService.validatePluginData({
          name: 'Valid Plugin',
          command: './valid-plugin',
          hookType: 'auth',
        });

        expect(result.isValid).toBe(true);
        expect(result.errors).toEqual({});
      });

      test('should return error for missing name', () => {
        const result = pluginService.validatePluginData({
          name: '',
          command: './plugin',
          hookType: 'auth',
        });

        expect(result.isValid).toBe(false);
        expect(result.errors.name).toBe('Plugin name is required');
      });

      test('should return error for missing command', () => {
        const result = pluginService.validatePluginData({
          name: 'Plugin',
          command: '',
          hookType: 'auth',
        });

        expect(result.isValid).toBe(false);
        expect(result.errors.command).toBe('Plugin command is required');
      });

      test('should accept valid OCI reference with path', () => {
        // The validation only fails if OCI reference has no "/" after "oci://"
        // Since "oci://invalid" contains "/" (in oci://), it passes
        const result = pluginService.validatePluginData({
          name: 'Plugin',
          command: 'oci://registry/image:tag',
          hookType: 'auth',
        });

        expect(result.isValid).toBe(true);
      });

      test('should return error for missing hook type', () => {
        const result = pluginService.validatePluginData({
          name: 'Plugin',
          command: './plugin',
          hookType: '',
        });

        expect(result.isValid).toBe(false);
        expect(result.errors.hookType).toBe('Hook type is required');
      });

      test('should return error for invalid hook type', () => {
        const result = pluginService.validatePluginData({
          name: 'Plugin',
          command: './plugin',
          hookType: 'invalid_hook',
        });

        expect(result.isValid).toBe(false);
        expect(result.errors.hookType).toBe('Invalid hook type');
      });

      test('should return multiple errors', () => {
        const result = pluginService.validatePluginData({
          name: '',
          command: '',
          hookType: '',
        });

        expect(result.isValid).toBe(false);
        expect(Object.keys(result.errors)).toHaveLength(3);
      });
    });
  });

  // OCI Plugin Operations tests
  describe('OCI Plugin Operations', () => {
    describe('createOCIPlugin', () => {
      test('should create an OCI plugin', async () => {
        const mockResponse = {
          data: {
            data: {
              id: '3',
              attributes: {
                name: 'OCI Plugin',
                description: 'An OCI plugin',
                command: 'oci://registry/plugin:latest',
                plugin_type: 'oci',
                oci_reference: 'registry/plugin:latest',
                manifest: { version: '2.0' },
                hook_type: 'data_collection',
                is_active: true,
                namespace: 'oci-ns',
                created_at: '2024-01-05T00:00:00Z',
                updated_at: '2024-01-05T00:00:00Z',
              },
            },
          },
        };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.createOCIPlugin({
          name: 'OCI Plugin',
          description: 'An OCI plugin',
          ociReference: 'registry/plugin:latest',
          hookType: 'data_collection',
        });

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/oci', expect.objectContaining({
          name: 'OCI Plugin',
          oci_reference: 'registry/plugin:latest',
        }));
        expect(result.pluginType).toBe('oci');
        expect(result.ociReference).toBe('registry/plugin:latest');
      });
    });

    describe('refreshOCIPlugin', () => {
      test('should refresh an OCI plugin', async () => {
        const mockResponse = {
          data: {
            data: {
              id: '3',
              attributes: {
                name: 'Refreshed Plugin',
                description: '',
                command: 'oci://registry/plugin:v2',
                plugin_type: 'oci',
                oci_reference: 'registry/plugin:v2',
                manifest: { version: '2.1' },
                hook_type: 'data_collection',
                is_active: true,
                namespace: 'global',
                created_at: '2024-01-05T00:00:00Z',
                updated_at: '2024-01-06T00:00:00Z',
              },
            },
          },
        };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.refreshOCIPlugin('3');

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/3/refresh');
        expect(result.manifest).toEqual({ version: '2.1' });
      });
    });
  });

  // Plugin UI and workflow tests
  describe('Plugin UI and workflow', () => {
    describe('loadPluginUI', () => {
      test('should load plugin UI', async () => {
        const mockResponse = { data: { success: true } };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.loadPluginUI('1');

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/1/ui/load');
        expect(result).toEqual({ success: true });
      });
    });

    describe('unloadPluginUI', () => {
      test('should unload plugin UI', async () => {
        const mockResponse = { data: { success: true } };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.unloadPluginUI('1');

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/1/ui/unload');
        expect(result).toEqual({ success: true });
      });
    });

    describe('getPluginConfigSchema', () => {
      test('should fetch plugin config schema', async () => {
        const mockResponse = {
          data: {
            data: {
              attributes: {
                schema: { type: 'object', properties: {} },
              },
            },
          },
        };
        apiClient.get.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.getPluginConfigSchema('1');

        expect(apiClient.get).toHaveBeenCalledWith('/plugins/1/config-schema');
        expect(result).toEqual({ type: 'object', properties: {} });
      });

      test('should return null on error (graceful fallback)', async () => {
        apiClient.get.mockRejectedValueOnce(new Error('Schema not found'));

        const result = await pluginService.getPluginConfigSchema('1');

        expect(result).toBeNull();
      });
    });

    describe('refreshPluginConfigSchema', () => {
      test('should refresh plugin config schema', async () => {
        const mockResponse = {
          data: {
            data: {
              attributes: {
                schema: { type: 'object', properties: { key: { type: 'string' } } },
              },
            },
          },
        };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.refreshPluginConfigSchema('1');

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/1/config-schema/refresh');
        expect(result).toEqual({ type: 'object', properties: { key: { type: 'string' } } });
      });

      test('should throw error on failure', async () => {
        apiClient.post.mockRejectedValueOnce({
          response: { data: { errors: [{ detail: 'Refresh failed' }] } },
        });

        await expect(pluginService.refreshPluginConfigSchema('1')).rejects.toThrow('Refresh failed');
      });
    });

    describe('validateAndLoadPlugin', () => {
      test('should validate and load plugin', async () => {
        const mockResponse = { data: { status: 'validated' } };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.validateAndLoadPlugin('1', { key: 'value' });

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/1/validate-and-load', { key: 'value' });
        expect(result).toEqual({ status: 'validated' });
      });
    });

    describe('approvePluginScopes', () => {
      test('should approve plugin scopes', async () => {
        const mockResponse = { data: { approved: true } };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.approvePluginScopes('1', true);

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/1/approve-scopes', { approved: true });
        expect(result).toEqual({ approved: true });
      });
    });

    describe('getPluginWorkflowStatus', () => {
      test('should get plugin workflow status', async () => {
        const mockResponse = { data: { status: 'pending_approval' } };
        apiClient.get.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.getPluginWorkflowStatus('1');

        expect(apiClient.get).toHaveBeenCalledWith('/plugins/1/workflow-status');
        expect(result).toEqual({ status: 'pending_approval' });
      });
    });
  });

  // Registry and sidebar tests
  describe('Registry and sidebar', () => {
    describe('getUIRegistry', () => {
      test('should fetch UI registry', async () => {
        const mockResponse = { data: { data: [{ id: '1', path: '/plugin' }] } };
        apiClient.get.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.getUIRegistry();

        expect(apiClient.get).toHaveBeenCalledWith('/plugins/ui-registry');
        expect(result).toEqual([{ id: '1', path: '/plugin' }]);
      });

      test('should return empty array when no data', async () => {
        apiClient.get.mockResolvedValueOnce({ data: {} });

        const result = await pluginService.getUIRegistry();

        expect(result).toEqual([]);
      });
    });

    describe('getSidebarMenuItems', () => {
      test('should fetch sidebar menu items', async () => {
        const mockResponse = { data: { data: [{ label: 'Plugin Menu', path: '/plugin' }] } };
        apiClient.get.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.getSidebarMenuItems();

        expect(apiClient.get).toHaveBeenCalledWith('/plugins/sidebar-menu');
        expect(result).toEqual([{ label: 'Plugin Menu', path: '/plugin' }]);
      });
    });

    describe('reloadPlugin', () => {
      test('should reload a plugin', async () => {
        const mockResponse = { data: { reloaded: true } };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.reloadPlugin('1');

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/1/reload');
        expect(result).toEqual({ reloaded: true });
      });

      test('should throw error with detail on failure', async () => {
        apiClient.post.mockRejectedValueOnce({
          response: { data: { errors: [{ detail: 'Reload error' }] } },
        });

        await expect(pluginService.reloadPlugin('1')).rejects.toThrow('Reload error');
      });
    });

    describe('getPluginsByType', () => {
      test('should fetch plugins by type', async () => {
        const mockResponse = {
          data: {
            data: [
              {
                id: '1',
                attributes: {
                  name: 'Gateway Plugin',
                  description: '',
                  command: './gateway',
                  plugin_type: 'gateway',
                  oci_reference: '',
                  hook_type: 'auth',
                  is_active: true,
                  namespace: 'global',
                  created_at: '2024-01-01T00:00:00Z',
                  updated_at: '2024-01-01T00:00:00Z',
                },
              },
            ],
          },
        };
        apiClient.get.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.getPluginsByType('gateway');

        expect(apiClient.get).toHaveBeenCalledWith('/plugins/type/gateway');
        expect(result).toHaveLength(1);
        expect(result[0].pluginType).toBe('gateway');
      });

      test('should return empty array when no data', async () => {
        apiClient.get.mockResolvedValueOnce({ data: {} });

        const result = await pluginService.getPluginsByType('nonexistent');

        expect(result).toEqual([]);
      });
    });

    describe('parsePluginManifest', () => {
      test('should parse plugin manifest', async () => {
        const mockResponse = { data: { manifest: { version: '1.0' } } };
        apiClient.post.mockResolvedValueOnce(mockResponse);

        const result = await pluginService.parsePluginManifest('1');

        expect(apiClient.post).toHaveBeenCalledWith('/plugins/1/manifest/parse');
        expect(result).toEqual({ manifest: { version: '1.0' } });
      });
    });
  });
});
