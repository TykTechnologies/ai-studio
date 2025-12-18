import marketplaceService from './marketplaceService';
import apiClient from '../utils/apiClient';

// Mock apiClient
jest.mock('../utils/apiClient');

describe('MarketplaceService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('listPlugins', () => {
    const mockPluginListResponse = {
      data: {
        plugins: [
          {
            id: 'com.tyk.echo-agent',
            name: 'Echo Agent',
            version: '1.0.0',
            category: 'agent',
            publisher: 'Tyk',
            maturity: 'stable',
          },
        ],
        pagination: { page: 1, page_size: 20, total: 1 },
      },
    };

    test('should fetch plugins with no parameters', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      const result = await marketplaceService.listPlugins();

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins?');
      expect(result).toEqual(mockPluginListResponse.data);
    });

    test('should fetch plugins with page parameter', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      await marketplaceService.listPlugins({ page: 2 });

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins?page=2');
    });

    test('should fetch plugins with page_size parameter', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      await marketplaceService.listPlugins({ page_size: 50 });

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins?page_size=50');
    });

    test('should fetch plugins with category filter', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      await marketplaceService.listPlugins({ category: 'agent' });

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins?category=agent');
    });

    test('should fetch plugins with publisher filter', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      await marketplaceService.listPlugins({ publisher: 'Tyk' });

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins?publisher=Tyk');
    });

    test('should fetch plugins with maturity filter', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      await marketplaceService.listPlugins({ maturity: 'stable' });

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins?maturity=stable');
    });

    test('should fetch plugins with search query', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      await marketplaceService.listPlugins({ search: 'echo' });

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins?search=echo');
    });

    test('should fetch plugins with include_deprecated flag', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      await marketplaceService.listPlugins({ include_deprecated: true });

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins?include_deprecated=true');
    });

    test('should fetch plugins with all parameters', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginListResponse);

      await marketplaceService.listPlugins({
        page: 1,
        page_size: 20,
        category: 'agent',
        publisher: 'Tyk',
        maturity: 'stable',
        search: 'echo',
        include_deprecated: false,
      });

      expect(apiClient.get).toHaveBeenCalledWith(
        '/marketplace/plugins?page=1&page_size=20&category=agent&publisher=Tyk&maturity=stable&search=echo&include_deprecated=false'
      );
    });
  });

  describe('getPlugin', () => {
    const mockPluginResponse = {
      data: {
        id: 'com.tyk.echo-agent',
        name: 'Echo Agent',
        version: '1.0.0',
        description: 'A test echo agent',
        category: 'agent',
        publisher: 'Tyk',
        maturity: 'stable',
        repository_url: 'https://github.com/tyk/echo-agent',
      },
    };

    test('should fetch plugin without version', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginResponse);

      const result = await marketplaceService.getPlugin('com.tyk.echo-agent');

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins/com.tyk.echo-agent');
      expect(result).toEqual(mockPluginResponse.data);
    });

    test('should fetch plugin with specific version', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginResponse);

      await marketplaceService.getPlugin('com.tyk.echo-agent', '1.0.0');

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins/com.tyk.echo-agent?version=1.0.0');
    });

    test('should handle null version parameter', async () => {
      apiClient.get.mockResolvedValueOnce(mockPluginResponse);

      await marketplaceService.getPlugin('com.tyk.echo-agent', null);

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins/com.tyk.echo-agent');
    });
  });

  describe('getPluginVersions', () => {
    const mockVersionsResponse = {
      data: {
        versions: [
          { version: '1.0.0', released_at: '2024-01-01T00:00:00Z' },
          { version: '0.9.0', released_at: '2023-12-01T00:00:00Z' },
        ],
      },
    };

    test('should fetch all versions of a plugin', async () => {
      apiClient.get.mockResolvedValueOnce(mockVersionsResponse);

      const result = await marketplaceService.getPluginVersions('com.tyk.echo-agent');

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins/com.tyk.echo-agent/versions');
      expect(result).toEqual(mockVersionsResponse.data);
    });
  });

  describe('getInstallMetadata', () => {
    const mockInstallMetadataResponse = {
      data: {
        plugin_id: 'com.tyk.echo-agent',
        version: '1.0.0',
        config_schema: { type: 'object', properties: {} },
        required_permissions: ['read', 'write'],
        install_instructions: 'Follow these steps...',
      },
    };

    test('should fetch install metadata without version', async () => {
      apiClient.get.mockResolvedValueOnce(mockInstallMetadataResponse);

      const result = await marketplaceService.getInstallMetadata('com.tyk.echo-agent');

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins/com.tyk.echo-agent/install-metadata');
      expect(result).toEqual(mockInstallMetadataResponse.data);
    });

    test('should fetch install metadata with specific version', async () => {
      apiClient.get.mockResolvedValueOnce(mockInstallMetadataResponse);

      await marketplaceService.getInstallMetadata('com.tyk.echo-agent', '1.0.0');

      expect(apiClient.get).toHaveBeenCalledWith(
        '/marketplace/plugins/com.tyk.echo-agent/install-metadata?version=1.0.0'
      );
    });

    test('should handle null version parameter', async () => {
      apiClient.get.mockResolvedValueOnce(mockInstallMetadataResponse);

      await marketplaceService.getInstallMetadata('com.tyk.echo-agent', null);

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/plugins/com.tyk.echo-agent/install-metadata');
    });
  });

  describe('getAvailableUpdates', () => {
    const mockUpdatesResponse = {
      data: {
        updates: [
          {
            plugin_id: 'com.tyk.echo-agent',
            current_version: '0.9.0',
            latest_version: '1.0.0',
            release_notes: 'Bug fixes and improvements',
          },
        ],
      },
    };

    test('should fetch available updates', async () => {
      apiClient.get.mockResolvedValueOnce(mockUpdatesResponse);

      const result = await marketplaceService.getAvailableUpdates();

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/updates');
      expect(result).toEqual(mockUpdatesResponse.data);
    });
  });

  describe('syncMarketplace', () => {
    const mockSyncResponse = {
      data: {
        status: 'started',
        message: 'Marketplace sync initiated',
      },
    };

    test('should trigger marketplace sync', async () => {
      apiClient.post.mockResolvedValueOnce(mockSyncResponse);

      const result = await marketplaceService.syncMarketplace();

      expect(apiClient.post).toHaveBeenCalledWith('/marketplace/sync');
      expect(result).toEqual(mockSyncResponse.data);
    });
  });

  describe('getSyncStatus', () => {
    const mockSyncStatusResponse = {
      data: {
        indexes: [
          { name: 'tyk-official', status: 'synced', last_synced: '2024-01-01T12:00:00Z' },
          { name: 'community', status: 'syncing', last_synced: '2024-01-01T11:00:00Z' },
        ],
      },
    };

    test('should fetch sync status', async () => {
      apiClient.get.mockResolvedValueOnce(mockSyncStatusResponse);

      const result = await marketplaceService.getSyncStatus();

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/sync-status');
      expect(result).toEqual(mockSyncStatusResponse.data);
    });
  });

  describe('getCategories', () => {
    const mockCategoriesResponse = {
      data: {
        categories: ['agent', 'filter', 'tool', 'integration'],
      },
    };

    test('should fetch categories', async () => {
      apiClient.get.mockResolvedValueOnce(mockCategoriesResponse);

      const result = await marketplaceService.getCategories();

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/categories');
      expect(result).toEqual(['agent', 'filter', 'tool', 'integration']);
    });

    test('should return empty array when categories not present', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await marketplaceService.getCategories();

      expect(result).toEqual([]);
    });
  });

  describe('getPublishers', () => {
    const mockPublishersResponse = {
      data: {
        publishers: ['Tyk', 'Community', 'Third-Party'],
      },
    };

    test('should fetch publishers', async () => {
      apiClient.get.mockResolvedValueOnce(mockPublishersResponse);

      const result = await marketplaceService.getPublishers();

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/publishers');
      expect(result).toEqual(['Tyk', 'Community', 'Third-Party']);
    });

    test('should return empty array when publishers not present', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} });

      const result = await marketplaceService.getPublishers();

      expect(result).toEqual([]);
    });
  });

  describe('getStats', () => {
    const mockStatsResponse = {
      data: {
        total_plugins: 50,
        total_downloads: 10000,
        categories: { agent: 20, filter: 15, tool: 15 },
        publishers: { Tyk: 30, Community: 20 },
      },
    };

    test('should fetch marketplace stats', async () => {
      apiClient.get.mockResolvedValueOnce(mockStatsResponse);

      const result = await marketplaceService.getStats();

      expect(apiClient.get).toHaveBeenCalledWith('/marketplace/stats');
      expect(result).toEqual(mockStatsResponse.data);
    });
  });
});
