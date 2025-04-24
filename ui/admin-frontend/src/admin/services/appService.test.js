import * as appService from './appService';
import apiClient from '../utils/apiClient';

describe('appService', () => {
  describe('createApp', () => {
    it('should POST to /apps with correct payload and return data', async () => {
      const mockAppData = {
        name: 'Test App',
        description: 'desc',
        userId: '42',
        llmIds: [1,2],
        datasourceIds: [3],
        setBudget: true,
        monthlyBudget: '100',
        budgetStartDate: '2024-01-01T00:00:00Z'
      };
      const expectedPayload = {
        data: {
          type: 'apps',
          attributes: {
            name: 'Test App',
            description: 'desc',
            user_id: 42,
            llm_ids: [1,2],
            datasource_ids: [3],
            monthly_budget: 100,
            budget_start_date: new Date('2024-01-01T00:00:00Z').toISOString()
          }
        }
      };
      const mockResponse = { data: { data: { id: 'app-1', ...mockAppData } } };
      jest.spyOn(apiClient, 'post').mockResolvedValueOnce(mockResponse);
      const result = await appService.createApp(mockAppData);
      expect(apiClient.post).toHaveBeenCalledWith('/apps', expectedPayload);
      expect(result).toEqual(mockResponse.data.data);
    });
    it('should handle missing optional fields', async () => {
      const mockAppData = { name: 'App', description: '', setBudget: false };
      jest.spyOn(apiClient, 'post').mockResolvedValueOnce({ data: { data: {} } });
      await appService.createApp(mockAppData);
      expect(apiClient.post).toHaveBeenCalledWith(
        '/apps',
        expect.objectContaining({
          data: expect.objectContaining({ attributes: expect.objectContaining({ monthly_budget: null, budget_start_date: null }) })
        })
      );
    });
    it('should throw handled error on failure', async () => {
      jest.spyOn(apiClient, 'post').mockRejectedValueOnce(new Error('fail'));
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      await expect(appService.createApp({})).rejects.toThrow('handled');
    });
  });

  describe('updateApp', () => {
    it('should PATCH to /apps/:id with correct payload and return data', async () => {
      const mockAppId = 'app-1';
      const mockAppData = {
        name: 'Updated App',
        description: 'desc',
        userId: '42',
        llmIds: [1],
        datasourceIds: [2],
        setBudget: true,
        monthlyBudget: '200',
        budgetStartDate: '2024-02-01T00:00:00Z'
      };
      const expectedPayload = {
        data: {
          type: 'apps',
          attributes: {
            name: 'Updated App',
            description: 'desc',
            user_id: 42,
            llm_ids: [1],
            datasource_ids: [2],
            monthly_budget: 200,
            budget_start_date: new Date('2024-02-01T00:00:00Z').toISOString()
          }
        }
      };
      const mockResponse = { data: { data: { id: mockAppId, ...mockAppData } } };
      jest.spyOn(apiClient, 'patch').mockResolvedValueOnce(mockResponse);
      const result = await appService.updateApp(mockAppId, mockAppData);
      expect(apiClient.patch).toHaveBeenCalledWith(`/apps/${mockAppId}`, expectedPayload);
      expect(result).toEqual(mockResponse.data.data);
    });
    it('should handle missing optional fields', async () => {
      const mockAppId = 'app-2';
      const mockAppData = { name: 'App', description: '', setBudget: false };
      jest.spyOn(apiClient, 'patch').mockResolvedValueOnce({ data: { data: {} } });
      await appService.updateApp(mockAppId, mockAppData);
      expect(apiClient.patch).toHaveBeenCalledWith(
        `/apps/${mockAppId}`,
        expect.objectContaining({
          data: expect.objectContaining({ attributes: expect.objectContaining({ monthly_budget: null, budget_start_date: null }) })
        })
      );
    });
    it('should throw handled error on failure', async () => {
      jest.spyOn(apiClient, 'patch').mockRejectedValueOnce(new Error('fail'));
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      await expect(appService.updateApp('id', {})).rejects.toThrow('handled');
    });
  });

  describe('activateCredential', () => {
    it('should POST to /apps/:id/activate-credential and return data', async () => {
      const mockAppId = 'app-1';
      const mockResponse = { data: { data: { success: true } } };
      jest.spyOn(apiClient, 'post').mockResolvedValueOnce(mockResponse);
      const result = await appService.activateCredential(mockAppId);
      expect(apiClient.post).toHaveBeenCalledWith(`/apps/${mockAppId}/activate-credential`);
      expect(result).toEqual(mockResponse.data.data);
    });
    it('should throw handled error on failure', async () => {
      jest.spyOn(apiClient, 'post').mockRejectedValueOnce(new Error('fail'));
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      await expect(appService.activateCredential('id')).rejects.toThrow('handled');
    });
  });
});
