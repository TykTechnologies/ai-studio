import apiClient from '../utils/apiClient';

/**
 * Marketplace Management Service
 * Handles API calls for managing multiple marketplace sources (Enterprise Edition)
 */
class MarketplaceManagementService {
  /**
   * List all marketplace indexes
   * @returns {Promise<Array>} List of marketplace indexes
   */
  async listMarketplaces() {
    const response = await apiClient.get('/admin/marketplaces');
    return response.data.data;
  }

  /**
   * Get a specific marketplace by ID
   * @param {number} id - Marketplace ID
   * @returns {Promise<Object>} Marketplace details
   */
  async getMarketplace(id) {
    const response = await apiClient.get(`/admin/marketplaces/${id}`);
    return response.data.data;
  }

  /**
   * Add a new marketplace
   * @param {string} url - Marketplace index URL
   * @param {boolean} isDefault - Set as default marketplace
   * @returns {Promise<Object>} Created marketplace
   */
  async addMarketplace(url, isDefault = false) {
    const response = await apiClient.post('/admin/marketplaces', {
      url,
      is_default: isDefault,
    });
    return response.data.data;
  }

  /**
   * Update marketplace properties
   * @param {number} id - Marketplace ID
   * @param {Object} updates - Updates to apply
   * @param {boolean} updates.is_active - Activate/deactivate
   * @param {boolean} updates.is_default - Set as default
   * @returns {Promise<Object>} Response message
   */
  async updateMarketplace(id, updates) {
    const response = await apiClient.put(`/admin/marketplaces/${id}`, updates);
    return response.data;
  }

  /**
   * Remove a marketplace
   * @param {number} id - Marketplace ID
   * @returns {Promise<Object>} Response message
   */
  async removeMarketplace(id) {
    const response = await apiClient.delete(`/admin/marketplaces/${id}`);
    return response.data;
  }

  /**
   * Validate a marketplace URL before adding
   * @param {string} url - Marketplace index URL to validate
   * @returns {Promise<Object>} Validation result
   */
  async validateMarketplaceURL(url) {
    const response = await apiClient.post('/admin/marketplaces/validate', { url });
    return response.data.data;
  }

  /**
   * Trigger manual sync for a marketplace
   * @param {number} id - Marketplace ID
   * @returns {Promise<Object>} Response message
   */
  async syncMarketplace(id) {
    const response = await apiClient.post(`/admin/marketplaces/${id}/sync`, {});
    return response.data;
  }

  /**
   * Set a marketplace as default
   * @param {number} id - Marketplace ID
   * @returns {Promise<Object>} Response message
   */
  async setDefaultMarketplace(id) {
    return this.updateMarketplace(id, { is_default: true });
  }

  /**
   * Toggle marketplace active status
   * @param {number} id - Marketplace ID
   * @param {boolean} active - New active status
   * @returns {Promise<Object>} Response message
   */
  async toggleMarketplace(id, active) {
    return this.updateMarketplace(id, { is_active: active });
  }
}

export default new MarketplaceManagementService();
