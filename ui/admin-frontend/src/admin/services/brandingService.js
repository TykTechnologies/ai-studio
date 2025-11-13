import apiClient from '../utils/apiClient';
import pubClient from '../utils/pubClient';
import { handleApiError } from './utils/errorHandler';

/**
 * Get current branding settings
 * Public endpoint - no authentication required
 */
export const getBrandingSettings = async () => {
  try {
    const response = await pubClient.get('/api/v1/branding/settings');
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/**
 * Update branding settings (admin only)
 * @param {Object} settings - Branding settings to update
 * @param {string} settings.app_title - Custom application title
 * @param {string} settings.primary_color - Primary brand color (hex)
 * @param {string} settings.secondary_color - Secondary brand color (hex)
 * @param {string} settings.background_color - Background color (hex)
 * @param {string} settings.custom_css - Custom CSS
 */
export const updateBrandingSettings = async (settings) => {
  try {
    const response = await apiClient.put('/branding/settings', settings);
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/**
 * Upload custom logo (admin only)
 * @param {File} file - Logo file (PNG/JPG/SVG, max 2MB)
 */
export const uploadLogo = async (file) => {
  try {
    const formData = new FormData();
    formData.append('file', file);

    const response = await apiClient.post('/branding/logo', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/**
 * Upload custom favicon (admin only)
 * @param {File} file - Favicon file (ICO/PNG, max 100KB)
 */
export const uploadFavicon = async (file) => {
  try {
    const formData = new FormData();
    formData.append('file', file);

    const response = await apiClient.post('/branding/favicon', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/**
 * Reset branding to defaults (admin only)
 */
export const resetBrandingToDefaults = async () => {
  try {
    const response = await apiClient.post('/branding/reset');
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/**
 * Get logo URL
 */
export const getLogoUrl = () => {
  return '/api/v1/branding/logo';
};

/**
 * Get favicon URL
 */
export const getFaviconUrl = () => {
  return '/api/v1/branding/favicon';
};
