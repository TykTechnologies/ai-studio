import * as credentialService from './credentialService';
import apiClient from '../utils/apiClient';

describe('credentialService', () => {
  describe('getCredential', () => {
    it('should GET /credentials/:id and return data', async () => {
      const credentialId = 'cred-1';
      const mockResponse = { data: { data: { id: credentialId, foo: 'bar' } } };
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      const result = await credentialService.getCredential(credentialId);
      expect(apiClient.get).toHaveBeenCalledWith(`/credentials/${credentialId}`);
      expect(result).toEqual(mockResponse.data.data);
    });
    it('should throw handled error on failure', async () => {
      jest.spyOn(apiClient, 'get').mockRejectedValueOnce(new Error('fail'));
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      await expect(credentialService.getCredential('id')).rejects.toThrow('handled');
    });
  });
});
