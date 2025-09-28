import apiClient from '../utils/apiClient';

class PluginService {
  // Hook type constants matching backend
  static HOOK_TYPES = {
    PRE_AUTH: 'pre_auth',
    AUTH: 'auth',
    POST_AUTH: 'post_auth',
    ON_RESPONSE: 'on_response',
    DATA_COLLECTION: 'data_collection',
  };

  static HOOK_TYPE_LABELS = {
    [PluginService.HOOK_TYPES.PRE_AUTH]: 'Pre-Authentication',
    [PluginService.HOOK_TYPES.AUTH]: 'Authentication',
    [PluginService.HOOK_TYPES.POST_AUTH]: 'Post-Authentication',
    [PluginService.HOOK_TYPES.ON_RESPONSE]: 'Response Processing',
    [PluginService.HOOK_TYPES.DATA_COLLECTION]: 'Data Collection',
  };

  async listPlugins(page = 1, limit = 50, hookType = '', isActive) {
    try {
      const params = {
        page,
        limit,
        ...(hookType && { hook_type: hookType }),
        ...(isActive !== undefined && { is_active: isActive }),
      };
      
      const response = await apiClient.get('/plugins', { params });
      
      if (response.data?.data) {
        return {
          data: response.data.data.map(plugin => ({
            id: plugin.id,
            name: plugin.attributes.name,
            slug: plugin.attributes.slug,
            description: plugin.attributes.description,
            command: plugin.attributes.command,
            checksum: plugin.attributes.checksum,
            config: plugin.attributes.config || {},
            hookType: plugin.attributes.hook_type,
            isActive: plugin.attributes.is_active,
            namespace: plugin.attributes.namespace || 'global',
            pluginType: plugin.attributes.plugin_type || 'gateway',
            ociReference: plugin.attributes.oci_reference || '',
            manifest: plugin.attributes.manifest || {},
            createdAt: plugin.attributes.created_at,
            updatedAt: plugin.attributes.updated_at,
          })),
          meta: response.data.meta || {},
        };
      }
      
      return { data: [], meta: {} };
    } catch (error) {
      console.error('Error fetching plugins:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch plugins');
    }
  }

  async getPlugin(id) {
    try {
      const response = await apiClient.get(`/plugins/${id}`);
      
      if (response.data?.data) {
        const plugin = response.data.data;
        return {
          id: plugin.id,
          name: plugin.attributes.name,
          slug: plugin.attributes.slug,
          description: plugin.attributes.description,
          command: plugin.attributes.command,
          checksum: plugin.attributes.checksum,
          config: plugin.attributes.config || {},
          hookType: plugin.attributes.hook_type,
          isActive: plugin.attributes.is_active,
          namespace: plugin.attributes.namespace || 'global',
          pluginType: plugin.attributes.plugin_type || 'gateway',
          ociReference: plugin.attributes.oci_reference || '',
          manifest: plugin.attributes.manifest || {},
          createdAt: plugin.attributes.created_at,
          updatedAt: plugin.attributes.updated_at,
          // Include associated LLMs if present with full data
          llms: plugin.relationships?.llms?.data?.map(llm => ({
            id: llm.id,
            name: llm.attributes?.name || 'Unknown LLM',
            vendor: llm.attributes?.vendor || 'Unknown',
            isActive: llm.attributes?.active !== undefined ? llm.attributes.active : true,
          })) || [],
        };
      }
      
      return null;
    } catch (error) {
      console.error('Error fetching plugin:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch plugin');
    }
  }

  async createPlugin(pluginData) {
    try {
      const payload = {
        name: pluginData.name,
        slug: pluginData.slug,
        description: pluginData.description || '',
        command: pluginData.command,
        checksum: pluginData.checksum || '',
        config: pluginData.config || {},
        hook_type: pluginData.hookType,
        is_active: pluginData.isActive !== undefined ? pluginData.isActive : true,
        namespace: pluginData.namespace || '',
        plugin_type: pluginData.pluginType || 'gateway',
        oci_reference: pluginData.ociReference || '',
        load_immediately: pluginData.loadImmediately || false,
      };

      const response = await apiClient.post('/plugins', payload);
      
      if (response.data?.data) {
        const plugin = response.data.data;
        return {
          id: plugin.id,
          name: plugin.attributes.name,
          slug: plugin.attributes.slug,
          description: plugin.attributes.description,
          command: plugin.attributes.command,
          checksum: plugin.attributes.checksum,
          config: plugin.attributes.config || {},
          hookType: plugin.attributes.hook_type,
          isActive: plugin.attributes.is_active,
          namespace: plugin.attributes.namespace || 'global',
          createdAt: plugin.attributes.created_at,
          updatedAt: plugin.attributes.updated_at,
        };
      }
      
      return null;
    } catch (error) {
      console.error('Error creating plugin:', error);
      throw new Error(error.response?.data?.message || 'Failed to create plugin');
    }
  }

  async updatePlugin(id, pluginData) {
    try {
      const payload = {
        name: pluginData.name,
        slug: pluginData.slug,
        description: pluginData.description || '',
        command: pluginData.command,
        checksum: pluginData.checksum || '',
        config: pluginData.config || {},
        hook_type: pluginData.hookType,
        is_active: pluginData.isActive !== undefined ? pluginData.isActive : true,
        namespace: pluginData.namespace || '',
        plugin_type: pluginData.pluginType || 'gateway',
        oci_reference: pluginData.ociReference || '',
        load_immediately: pluginData.loadImmediately || false,
      };

      const response = await apiClient.patch(`/plugins/${id}`, payload);
      
      if (response.data?.data) {
        const plugin = response.data.data;
        return {
          id: plugin.id,
          name: plugin.attributes.name,
          slug: plugin.attributes.slug,
          description: plugin.attributes.description,
          command: plugin.attributes.command,
          checksum: plugin.attributes.checksum,
          config: plugin.attributes.config || {},
          hookType: plugin.attributes.hook_type,
          isActive: plugin.attributes.is_active,
          namespace: plugin.attributes.namespace || 'global',
          createdAt: plugin.attributes.created_at,
          updatedAt: plugin.attributes.updated_at,
        };
      }
      
      return null;
    } catch (error) {
      console.error('Error updating plugin:', error);
      throw new Error(error.response?.data?.message || 'Failed to update plugin');
    }
  }

  async deletePlugin(id) {
    try {
      await apiClient.delete(`/plugins/${id}`);
      return true;
    } catch (error) {
      console.error('Error deleting plugin:', error);
      throw new Error(error.response?.data?.message || 'Failed to delete plugin');
    }
  }

  async getPluginsForLLM(llmId) {
    try {
      const response = await apiClient.get(`/llms/${llmId}/plugins`);
      
      if (response.data?.data) {
        return response.data.data.map(plugin => ({
          id: plugin.id,
          name: plugin.attributes.name,
          slug: plugin.attributes.slug,
          description: plugin.attributes.description,
          command: plugin.attributes.command,
          hookType: plugin.attributes.hook_type,
          isActive: plugin.attributes.is_active,
          namespace: plugin.attributes.namespace || 'global',
          // Include pivot data if available for ordering
          pivot: plugin.pivot || {},
        }));
      }
      
      return [];
    } catch (error) {
      console.error('Error fetching plugins for LLM:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch plugins for LLM');
    }
  }

  async updateLLMPlugins(llmId, pluginIds) {
    try {
      const payload = {
        plugin_ids: pluginIds,
      };
      
      const response = await apiClient.put(`/llms/${llmId}/plugins`, payload);
      
      if (response.data?.data) {
        return response.data.data.map(plugin => ({
          id: plugin.id,
          name: plugin.attributes.name,
          hookType: plugin.attributes.hook_type,
          isActive: plugin.attributes.is_active,
        }));
      }
      
      return [];
    } catch (error) {
      console.error('Error updating LLM plugins:', error);
      throw new Error(error.response?.data?.message || 'Failed to update LLM plugins');
    }
  }

  // Utility methods
  getHookTypeLabel(hookType) {
    return PluginService.HOOK_TYPE_LABELS[hookType] || hookType;
  }

  getAvailableHookTypes() {
    return Object.entries(PluginService.HOOK_TYPE_LABELS).map(([value, label]) => ({
      value,
      label,
    }));
  }

  validatePluginData(pluginData) {
    const errors = {};

    if (!pluginData.name?.trim()) {
      errors.name = 'Plugin name is required';
    }

    if (!pluginData.slug?.trim()) {
      errors.slug = 'Plugin slug is required';
    }

    // Validate command (auto-detect OCI vs local from prefix)
    if (!pluginData.command?.trim()) {
      errors.command = 'Plugin command is required';
    } else if (pluginData.command.startsWith('oci://') && (!pluginData.command.includes('/'))) {
      errors.command = 'Invalid OCI reference format';
    }

    if (!pluginData.hookType) {
      errors.hookType = 'Hook type is required';
    } else if (!Object.values(PluginService.HOOK_TYPES).includes(pluginData.hookType)) {
      errors.hookType = 'Invalid hook type';
    }

    return {
      isValid: Object.keys(errors).length === 0,
      errors,
    };
  }

  // OCI Plugin Operations

  async createOCIPlugin(ociPluginData) {
    try {
      const payload = {
        name: ociPluginData.name,
        slug: ociPluginData.slug,
        description: ociPluginData.description || '',
        oci_reference: ociPluginData.ociReference,
        config: ociPluginData.config || {},
        hook_type: ociPluginData.hookType,
        is_active: ociPluginData.isActive !== undefined ? ociPluginData.isActive : true,
        namespace: ociPluginData.namespace || '',
      };

      const response = await apiClient.post('/plugins/oci', payload);

      if (response.data?.data) {
        const plugin = response.data.data;
        return {
          id: plugin.id,
          name: plugin.attributes.name,
          slug: plugin.attributes.slug,
          description: plugin.attributes.description,
          command: plugin.attributes.command,
          pluginType: plugin.attributes.plugin_type,
          ociReference: plugin.attributes.oci_reference,
          manifest: plugin.attributes.manifest || {},
          hookType: plugin.attributes.hook_type,
          isActive: plugin.attributes.is_active,
          namespace: plugin.attributes.namespace || 'global',
          createdAt: plugin.attributes.created_at,
          updatedAt: plugin.attributes.updated_at,
        };
      }

      return null;
    } catch (error) {
      console.error('Error creating OCI plugin:', error);
      throw new Error(error.response?.data?.message || 'Failed to create OCI plugin');
    }
  }

  async refreshOCIPlugin(id) {
    try {
      const response = await apiClient.post(`/plugins/${id}/refresh`);

      if (response.data?.data) {
        const plugin = response.data.data;
        return {
          id: plugin.id,
          name: plugin.attributes.name,
          slug: plugin.attributes.slug,
          description: plugin.attributes.description,
          command: plugin.attributes.command,
          pluginType: plugin.attributes.plugin_type,
          ociReference: plugin.attributes.oci_reference,
          manifest: plugin.attributes.manifest || {},
          hookType: plugin.attributes.hook_type,
          isActive: plugin.attributes.is_active,
          namespace: plugin.attributes.namespace || 'global',
          createdAt: plugin.attributes.created_at,
          updatedAt: plugin.attributes.updated_at,
        };
      }

      return null;
    } catch (error) {
      console.error('Error refreshing OCI plugin:', error);
      throw new Error(error.response?.data?.message || 'Failed to refresh OCI plugin');
    }
  }

  async parsePluginManifest(id) {
    try {
      const response = await apiClient.post(`/plugins/${id}/manifest/parse`);
      return response.data;
    } catch (error) {
      console.error('Error parsing plugin manifest:', error);
      throw new Error(error.response?.data?.message || 'Failed to parse plugin manifest');
    }
  }

  async loadPluginUI(id) {
    try {
      const response = await apiClient.post(`/plugins/${id}/ui/load`);
      return response.data;
    } catch (error) {
      console.error('Error loading plugin UI:', error);
      throw new Error(error.response?.data?.message || 'Failed to load plugin UI');
    }
  }

  async unloadPluginUI(id) {
    try {
      const response = await apiClient.post(`/plugins/${id}/ui/unload`);
      return response.data;
    } catch (error) {
      console.error('Error unloading plugin UI:', error);
      throw new Error(error.response?.data?.message || 'Failed to unload plugin UI');
    }
  }

  async getPluginsByType(pluginType) {
    try {
      const response = await apiClient.get(`/plugins/type/${pluginType}`);

      if (response.data?.data) {
        return response.data.data.map(plugin => ({
          id: plugin.id,
          name: plugin.attributes.name,
          slug: plugin.attributes.slug,
          description: plugin.attributes.description,
          command: plugin.attributes.command,
          pluginType: plugin.attributes.plugin_type,
          ociReference: plugin.attributes.oci_reference,
          hookType: plugin.attributes.hook_type,
          isActive: plugin.attributes.is_active,
          namespace: plugin.attributes.namespace || 'global',
          createdAt: plugin.attributes.created_at,
          updatedAt: plugin.attributes.updated_at,
        }));
      }

      return [];
    } catch (error) {
      console.error('Error fetching plugins by type:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch plugins by type');
    }
  }

  async getUIRegistry() {
    try {
      const response = await apiClient.get('/plugins/ui-registry');
      return response.data?.data || [];
    } catch (error) {
      console.error('Error fetching UI registry:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch UI registry');
    }
  }

  async getSidebarMenuItems() {
    try {
      const response = await apiClient.get('/plugins/sidebar-menu');
      return response.data?.data || [];
    } catch (error) {
      console.error('Error fetching sidebar menu items:', error);
      throw new Error(error.response?.data?.message || 'Failed to fetch sidebar menu items');
    }
  }

  async reloadPlugin(id) {
    try {
      const response = await apiClient.post(`/plugins/${id}/reload`);
      return response.data;
    } catch (error) {
      console.error('Error reloading plugin:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to reload plugin');
    }
  }

  async getPluginConfigSchema(pluginId) {
    try {
      const response = await apiClient.get(`/plugins/${pluginId}/config-schema`);
      return response.data?.data?.attributes?.schema || null;
    } catch (error) {
      console.error('Error fetching plugin config schema:', error);

      // Log the specific error for debugging
      if (error.response?.data?.errors?.[0]?.detail) {
        console.error('Backend error:', error.response.data.errors[0].detail);
      }

      // Return null instead of throwing to allow graceful fallback to JSON editor
      return null;
    }
  }

  async refreshPluginConfigSchema(pluginId) {
    try {
      const response = await apiClient.post(`/plugins/${pluginId}/config-schema/refresh`);
      return response.data?.data?.attributes?.schema || null;
    } catch (error) {
      console.error('Error refreshing plugin config schema:', error);

      // Log the specific error for debugging
      if (error.response?.data?.errors?.[0]?.detail) {
        console.error('Backend refresh error:', error.response.data.errors[0].detail);
      }

      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to refresh plugin schema');
    }
  }

  // Plugin workflow methods for step-by-step creation and approval

  async validateAndLoadPlugin(pluginId, commandData = {}) {
    try {
      const response = await apiClient.post(`/plugins/${pluginId}/validate-and-load`, commandData);
      return response.data;
    } catch (error) {
      console.error('Error validating and loading plugin:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to validate and load plugin');
    }
  }

  async approvePluginScopes(pluginId, approved) {
    try {
      const response = await apiClient.post(`/plugins/${pluginId}/approve-scopes`, {
        approved,
      });
      return response.data;
    } catch (error) {
      console.error('Error approving plugin scopes:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to approve plugin scopes');
    }
  }

  async getPluginWorkflowStatus(pluginId) {
    try {
      const response = await apiClient.get(`/plugins/${pluginId}/workflow-status`);
      return response.data;
    } catch (error) {
      console.error('Error getting plugin workflow status:', error);
      throw new Error(error.response?.data?.errors?.[0]?.detail || 'Failed to get plugin workflow status');
    }
  }
}

export default new PluginService();