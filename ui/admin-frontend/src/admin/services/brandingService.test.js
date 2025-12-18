import * as brandingService from './brandingService';
import apiClient from '../utils/apiClient';
import pubClient from '../utils/pubClient';
import { handleApiError } from './utils/errorHandler';

// Mock dependencies
jest.mock('../utils/apiClient');
jest.mock('../utils/pubClient');
jest.mock('./utils/errorHandler');

describe('brandingService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    handleApiError.mockImplementation((error) => error);
  });

  describe('getBrandingSettings', () => {
    test('should fetch branding settings from public endpoint', async () => {
      const mockResponse = {
        data: {
          app_title: 'Custom App',
          primary_color: '#ff0000',
          secondary_color: '#00ff00',
          background_color: '#ffffff',
          has_custom_logo: true,
          has_custom_favicon: false,
        },
      };
      pubClient.get.mockResolvedValueOnce(mockResponse);

      const result = await brandingService.getBrandingSettings();

      expect(pubClient.get).toHaveBeenCalledWith('/api/v1/branding/settings');
      expect(result).toEqual(mockResponse.data);
    });

    test('should throw handled error on failure', async () => {
      const mockError = new Error('Network error');
      pubClient.get.mockRejectedValueOnce(mockError);
      handleApiError.mockReturnValueOnce(new Error('Handled error'));

      await expect(brandingService.getBrandingSettings()).rejects.toThrow('Handled error');
      expect(handleApiError).toHaveBeenCalledWith(mockError);
    });
  });

  describe('updateBrandingSettings', () => {
    test('should update branding settings', async () => {
      const settings = {
        app_title: 'New Title',
        primary_color: '#123456',
        secondary_color: '#654321',
        background_color: '#ffffff',
        custom_css: '.custom { color: red; }',
      };
      const mockResponse = { data: { success: true, ...settings } };
      apiClient.put.mockResolvedValueOnce(mockResponse);

      const result = await brandingService.updateBrandingSettings(settings);

      expect(apiClient.put).toHaveBeenCalledWith('/branding/settings', settings);
      expect(result).toEqual(mockResponse.data);
    });

    test('should throw handled error on failure', async () => {
      const mockError = new Error('Update failed');
      apiClient.put.mockRejectedValueOnce(mockError);
      handleApiError.mockReturnValueOnce(new Error('Handled error'));

      await expect(brandingService.updateBrandingSettings({})).rejects.toThrow('Handled error');
      expect(handleApiError).toHaveBeenCalledWith(mockError);
    });
  });

  describe('uploadLogo', () => {
    test('should upload logo file', async () => {
      const mockFile = new File(['logo content'], 'logo.png', { type: 'image/png' });
      const mockResponse = { data: { success: true, path: '/uploads/logo.png' } };
      apiClient.post.mockResolvedValueOnce(mockResponse);

      const result = await brandingService.uploadLogo(mockFile);

      expect(apiClient.post).toHaveBeenCalledWith(
        '/branding/logo',
        expect.any(FormData),
        { headers: { 'Content-Type': 'multipart/form-data' } }
      );

      // Verify FormData contains the file
      const [, formData] = apiClient.post.mock.calls[0];
      expect(formData.get('file')).toEqual(mockFile);
      expect(result).toEqual(mockResponse.data);
    });

    test('should throw handled error on failure', async () => {
      const mockFile = new File([''], 'logo.png');
      const mockError = new Error('Upload failed');
      apiClient.post.mockRejectedValueOnce(mockError);
      handleApiError.mockReturnValueOnce(new Error('Handled error'));

      await expect(brandingService.uploadLogo(mockFile)).rejects.toThrow('Handled error');
    });
  });

  describe('uploadFavicon', () => {
    test('should upload favicon file', async () => {
      const mockFile = new File(['favicon content'], 'favicon.ico', { type: 'image/x-icon' });
      const mockResponse = { data: { success: true, path: '/uploads/favicon.ico' } };
      apiClient.post.mockResolvedValueOnce(mockResponse);

      const result = await brandingService.uploadFavicon(mockFile);

      expect(apiClient.post).toHaveBeenCalledWith(
        '/branding/favicon',
        expect.any(FormData),
        { headers: { 'Content-Type': 'multipart/form-data' } }
      );

      // Verify FormData contains the file
      const [, formData] = apiClient.post.mock.calls[0];
      expect(formData.get('file')).toEqual(mockFile);
      expect(result).toEqual(mockResponse.data);
    });

    test('should throw handled error on failure', async () => {
      const mockFile = new File([''], 'favicon.ico');
      const mockError = new Error('Upload failed');
      apiClient.post.mockRejectedValueOnce(mockError);
      handleApiError.mockReturnValueOnce(new Error('Handled error'));

      await expect(brandingService.uploadFavicon(mockFile)).rejects.toThrow('Handled error');
    });
  });

  describe('resetBrandingToDefaults', () => {
    test('should reset branding to defaults', async () => {
      const mockResponse = { data: { success: true, reset: true } };
      apiClient.post.mockResolvedValueOnce(mockResponse);

      const result = await brandingService.resetBrandingToDefaults();

      expect(apiClient.post).toHaveBeenCalledWith('/branding/reset');
      expect(result).toEqual(mockResponse.data);
    });

    test('should throw handled error on failure', async () => {
      const mockError = new Error('Reset failed');
      apiClient.post.mockRejectedValueOnce(mockError);
      handleApiError.mockReturnValueOnce(new Error('Handled error'));

      await expect(brandingService.resetBrandingToDefaults()).rejects.toThrow('Handled error');
    });
  });

  describe('getLogoUrl', () => {
    test('should return logo URL', () => {
      const url = brandingService.getLogoUrl();
      expect(url).toBe('/api/v1/branding/logo');
    });
  });

  describe('getFaviconUrl', () => {
    test('should return favicon URL', () => {
      const url = brandingService.getFaviconUrl();
      expect(url).toBe('/api/v1/branding/favicon');
    });
  });
});
