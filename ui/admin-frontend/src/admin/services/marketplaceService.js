import apiClient from '../utils/apiClient';

const marketplaceService = {
  /**
   * List marketplace plugins with filtering and pagination
   * @param {Object} params - Query parameters
   * @param {number} params.page - Page number (default: 1)
   * @param {number} params.page_size - Page size (default: 20)
   * @param {string} params.category - Filter by category
   * @param {string} params.publisher - Filter by publisher
   * @param {string} params.maturity - Filter by maturity (alpha, beta, stable)
   * @param {string} params.search - Search query
   * @param {boolean} params.include_deprecated - Include deprecated plugins
   * @returns {Promise<Object>} - Paginated plugin list
   */
  listPlugins: async (params = {}) => {
    const queryParams = new URLSearchParams();

    if (params.page) queryParams.append('page', params.page);
    if (params.page_size) queryParams.append('page_size', params.page_size);
    if (params.category) queryParams.append('category', params.category);
    if (params.publisher) queryParams.append('publisher', params.publisher);
    if (params.maturity) queryParams.append('maturity', params.maturity);
    if (params.search) queryParams.append('search', params.search);
    if (params.include_deprecated !== undefined) {
      queryParams.append('include_deprecated', params.include_deprecated);
    }

    const response = await apiClient.get(`/marketplace/plugins?${queryParams.toString()}`);
    return response.data;
  },

  /**
   * Get a specific marketplace plugin
   * @param {string} pluginId - Plugin ID (e.g., "com.tyk.echo-agent")
   * @param {string} version - Optional version (defaults to latest)
   * @returns {Promise<Object>} - Plugin details
   */
  getPlugin: async (pluginId, version = null) => {
    const url = version
      ? `/marketplace/plugins/${pluginId}?version=${version}`
      : `/marketplace/plugins/${pluginId}`;
    const response = await apiClient.get(url);
    return response.data;
  },

  /**
   * Get all versions of a marketplace plugin
   * @param {string} pluginId - Plugin ID
   * @returns {Promise<Object>} - List of versions
   */
  getPluginVersions: async (pluginId) => {
    const response = await apiClient.get(`/marketplace/plugins/${pluginId}/versions`);
    return response.data;
  },

  /**
   * Get install metadata for pre-filling the plugin creation wizard
   * @param {string} pluginId - Plugin ID
   * @param {string} version - Optional version (defaults to latest)
   * @returns {Promise<Object>} - Install metadata for wizard
   */
  getInstallMetadata: async (pluginId, version = null) => {
    const url = version
      ? `/marketplace/plugins/${pluginId}/install-metadata?version=${version}`
      : `/marketplace/plugins/${pluginId}/install-metadata`;
    const response = await apiClient.get(url);
    return response.data;
  },

  /**
   * Get available plugin updates
   * @returns {Promise<Object>} - List of plugins with available updates
   */
  getAvailableUpdates: async () => {
    const response = await apiClient.get('/marketplace/updates');
    return response.data;
  },

  /**
   * Trigger manual marketplace sync
   * @returns {Promise<Object>} - Sync status
   */
  syncMarketplace: async () => {
    const response = await apiClient.post('/marketplace/sync');
    return response.data;
  },

  /**
   * Get marketplace sync status
   * @returns {Promise<Object>} - Sync status for all indexes
   */
  getSyncStatus: async () => {
    const response = await apiClient.get('/marketplace/sync-status');
    return response.data;
  },

  /**
   * Get available categories
   * @returns {Promise<Array>} - List of categories
   */
  getCategories: async () => {
    const response = await apiClient.get('/marketplace/categories');
    return response.data.categories || [];
  },

  /**
   * Get available publishers
   * @returns {Promise<Array>} - List of publishers
   */
  getPublishers: async () => {
    const response = await apiClient.get('/marketplace/publishers');
    return response.data.publishers || [];
  },

  /**
   * Get marketplace statistics
   * @returns {Promise<Object>} - Marketplace stats
   */
  getStats: async () => {
    const response = await apiClient.get('/marketplace/stats');
    return response.data;
  },
};

export default marketplaceService;
