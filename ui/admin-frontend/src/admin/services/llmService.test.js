import * as llmService from './llmService';
import apiClient from '../utils/apiClient';

describe('llmService', () => {
  describe('createLLM', () => {
    it('should POST to /llms with correct payload and return data', async () => {
      const mockLLMData = {
        name: 'LLM',
        apiKey: 'key',
        apiEndpoint: 'url',
        privacyScore: 5,
        shortDescription: 'short',
        longDescription: 'long',
        logoUrl: 'logo',
        llmProvider: 'vendor',
        active: false,
        filters: [1],
        defaultModel: 'def',
        allowedModels: ['gpt']
      };
      const expectedPayload = {
        data: {
          type: 'LLM',
          attributes: {
            name: 'LLM',
            api_key: 'key',
            api_endpoint: 'url',
            privacy_score: 5,
            short_description: 'short',
            long_description: 'long',
            logo_url: 'logo',
            vendor: 'vendor',
            active: false,
            filters: [1],
            default_model: 'def',
            allowed_models: ['gpt']
          }
        }
      };
      const mockResponse = { data: { data: { id: 'llm-1', ...mockLLMData } } };
      jest.spyOn(apiClient, 'post').mockResolvedValueOnce(mockResponse);
      const result = await llmService.createLLM(mockLLMData);
      expect(apiClient.post).toHaveBeenCalledWith('/llms', expectedPayload);
      expect(result).toEqual(mockResponse.data.data);
    });
    it('should handle default values for optional fields', async () => {
      const mockLLMData = { name: 'LLM', apiKey: '', apiEndpoint: '', privacyScore: 1, llmProvider: 'vendor' };
      jest.spyOn(apiClient, 'post').mockResolvedValueOnce({ data: { data: {} } });
      await llmService.createLLM(mockLLMData);
      expect(apiClient.post).toHaveBeenCalledWith(
        '/llms',
        expect.objectContaining({
          data: expect.objectContaining({ attributes: expect.objectContaining({ short_description: '', long_description: '', logo_url: '', active: true, filters: [], default_model: '', allowed_models: [] }) })
        })
      );
    });
    it('should throw handled error on failure', async () => {
      jest.spyOn(apiClient, 'post').mockRejectedValueOnce(new Error('fail'));
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      await expect(llmService.createLLM({})).rejects.toThrow('handled');
    });
  });

  describe('updateLLM', () => {
    it('should PATCH to /llms/:id with correct payload and return data', async () => {
      const mockLLMId = 'llm-1';
      const mockLLMData = {
        name: 'Updated',
        apiKey: 'k',
        apiEndpoint: 'u',
        privacyScore: 2,
        shortDescription: 's',
        longDescription: 'l',
        logoUrl: 'logo',
        llmProvider: 'vendor',
        active: true,
        filters: [2],
        defaultModel: 'def',
        allowedModels: ['gpt4']
      };
      const expectedPayload = {
        data: {
          type: 'LLM',
          attributes: {
            name: 'Updated',
            api_key: 'k',
            api_endpoint: 'u',
            privacy_score: 2,
            short_description: 's',
            long_description: 'l',
            logo_url: 'logo',
            vendor: 'vendor',
            active: true,
            filters: [2],
            default_model: 'def',
            allowed_models: ['gpt4']
          }
        }
      };
      const mockResponse = { data: { data: { id: mockLLMId, ...mockLLMData } } };
      jest.spyOn(apiClient, 'patch').mockResolvedValueOnce(mockResponse);
      const result = await llmService.updateLLM(mockLLMId, mockLLMData);
      expect(apiClient.patch).toHaveBeenCalledWith(`/llms/${mockLLMId}`, expectedPayload);
      expect(result).toEqual(mockResponse.data.data);
    });
    it('should handle default values for optional fields', async () => {
      const mockLLMId = 'llm-2';
      const mockLLMData = { name: 'LLM', apiKey: '', apiEndpoint: '', privacyScore: 1, llmProvider: 'vendor' };
      jest.spyOn(apiClient, 'patch').mockResolvedValueOnce({ data: { data: {} } });
      await llmService.updateLLM(mockLLMId, mockLLMData);
      expect(apiClient.patch).toHaveBeenCalledWith(
        `/llms/${mockLLMId}`,
        expect.objectContaining({
          data: expect.objectContaining({ attributes: expect.objectContaining({ short_description: '', long_description: '', logo_url: '', active: true, filters: [], default_model: '', allowed_models: [] }) })
        })
      );
    });
    it('should throw handled error on failure', async () => {
      jest.spyOn(apiClient, 'patch').mockRejectedValueOnce(new Error('fail'));
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      await expect(llmService.updateLLM('id', {})).rejects.toThrow('handled');
    });
  });
});
