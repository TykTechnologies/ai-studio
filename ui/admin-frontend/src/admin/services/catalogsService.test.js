import * as catalogsService from './catalogsService';
import apiClient from '../utils/apiClient';

describe('catalogsService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('getCatalogues', () => {
    it('should get catalogues with pagination by default', async () => {
      const mockResponse = {
        data: { data: [{ id: 'cat-1' }, { id: 'cat-2' }] },
        headers: {
          'x-total-count': '10',
          'x-total-pages': '5'
        }
      };
      
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      
      const result = await catalogsService.getCatalogues(2);
      
      expect(apiClient.get).toHaveBeenCalledWith('/catalogues', { params: { page: 2 } });
      expect(result).toEqual({
        data: mockResponse.data.data,
        totalCount: 10,
        totalPages: 5
      });
    });

    it('should get all catalogues when all flag is true', async () => {
      const mockData = [{ id: 'cat-1' }, { id: 'cat-2' }, { id: 'cat-3' }];
      const mockResponse = {
        data: { data: mockData }
      };
      
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      
      const result = await catalogsService.getCatalogues(1, true);
      
      expect(apiClient.get).toHaveBeenCalledWith('/catalogues', { params: { all: true } });
      expect(result).toEqual(mockData);
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('API Error');
      jest.spyOn(apiClient, 'get').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('Handled API Error'));
      
      await expect(catalogsService.getCatalogues()).rejects.toThrow('Handled API Error');
    });
  });

  describe('getDataCatalogues', () => {
    it('should get data catalogues with pagination by default', async () => {
      const mockResponse = {
        data: { data: [{ id: 'data-cat-1' }, { id: 'data-cat-2' }] },
        headers: {
          'x-total-count': '8',
          'x-total-pages': '4'
        }
      };
      
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      
      const result = await catalogsService.getDataCatalogues(2);
      
      expect(apiClient.get).toHaveBeenCalledWith('/data-catalogues', { params: { page: 2 } });
      expect(result).toEqual({
        data: mockResponse.data.data,
        totalCount: 8,
        totalPages: 4
      });
    });

    it('should get all data catalogues when all flag is true', async () => {
      const mockData = [{ id: 'data-cat-1' }, { id: 'data-cat-2' }, { id: 'data-cat-3' }];
      const mockResponse = {
        data: { data: mockData }
      };
      
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      
      const result = await catalogsService.getDataCatalogues(1, true);
      
      expect(apiClient.get).toHaveBeenCalledWith('/data-catalogues', { params: { all: true } });
      expect(result).toEqual(mockData);
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('API Error');
      jest.spyOn(apiClient, 'get').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('Handled API Error'));
      
      await expect(catalogsService.getDataCatalogues()).rejects.toThrow('Handled API Error');
    });
  });

  describe('getToolCatalogues', () => {
    it('should get tool catalogues with pagination by default', async () => {
      const mockResponse = {
        data: { data: [{ id: 'tool-cat-1' }, { id: 'tool-cat-2' }] },
        headers: {
          'x-total-count': '6',
          'x-total-pages': '3'
        }
      };

      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);

      const result = await catalogsService.getToolCatalogues(2);

      expect(apiClient.get).toHaveBeenCalledWith('/tool-catalogues', { params: { page: 2 } });
      expect(result).toEqual({
        data: mockResponse.data.data,
        totalCount: 6,
        totalPages: 3
      });
    });

    it('should get all tool catalogues when all flag is true', async () => {
      const mockData = [{ id: 'tool-cat-1' }, { id: 'tool-cat-2' }, { id: 'tool-cat-3' }];
      const mockResponse = {
        data: { data: mockData }
      };

      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);

      const result = await catalogsService.getToolCatalogues(1, true);

      expect(apiClient.get).toHaveBeenCalledWith('/tool-catalogues', { params: { all: true } });
      expect(result).toEqual(mockData);
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('API Error');
      jest.spyOn(apiClient, 'get').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('Handled API Error'));
      
      await expect(catalogsService.getToolCatalogues()).rejects.toThrow('Handled API Error');
    });
  });
});