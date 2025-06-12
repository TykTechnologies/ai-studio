import * as userService from './userService';
import apiClient from '../utils/apiClient';

describe('userService', () => {
  describe('createUser', () => {
    it('should POST to /users with correct payload and return data', async () => {
      const mockUserData = {
        name: 'Test User',
        email: 'test@example.com',
        password: 'password123',
        isAdmin: true,
        showPortal: true,
        showChat: false,
        emailVerified: true,
        notificationsEnabled: true,
        accessToSSOConfig: true,
        groups: [1, 2]
      };
      const expectedPayload = {
        data: {
          type: 'User',
          attributes: {
            name: 'Test User',
            email: 'test@example.com',
            password: 'password123',
            is_admin: true,
            show_portal: true,
            show_chat: false,
            email_verified: true,
            notifications_enabled: true,
            access_to_sso_config: true,
            groups: [1, 2]
          }
        }
      };
      const mockResponse = { data: { data: { id: 'user-1', ...mockUserData } } };
      jest.spyOn(apiClient, 'post').mockResolvedValueOnce(mockResponse);
      const result = await userService.createUser(mockUserData);
      expect(apiClient.post).toHaveBeenCalledWith('/users', expectedPayload);
      expect(result).toEqual(mockResponse.data.data);
    });

    it('should default show_chat to true if undefined and include default boolean fields', async () => {
      const mockUserData = {
        name: 'Test User',
        email: 'test@example.com',
        password: 'password123',
        isAdmin: false,
        showPortal: false
      };
      const expectedPayload = expect.objectContaining({
        data: expect.objectContaining({
          attributes: expect.objectContaining({ 
            show_chat: true,
            email_verified: false,
            notifications_enabled: false,
            access_to_sso_config: false,
            groups: []
          })
        })
      });
      jest.spyOn(apiClient, 'post').mockResolvedValueOnce({ data: { data: {} } });
      await userService.createUser(mockUserData);
      expect(apiClient.post).toHaveBeenCalledWith('/users', expectedPayload);
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('fail');
      jest.spyOn(apiClient, 'post').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      await expect(userService.createUser({})).rejects.toThrow('handled');
    });
  });

  describe('updateUser', () => {
    it('should PATCH to /users/:id with correct payload and return data', async () => {
      const mockUserId = 'user-1';
      const mockUserData = {
        name: 'Updated User',
        email: 'updated@example.com',
        password: 'newpass',
        isAdmin: false,
        showPortal: false,
        showChat: true,
        emailVerified: false,
        notificationsEnabled: false,
        accessToSSOConfig: false,
        groups: [3]
      };
      const expectedPayload = {
        data: {
          type: 'User',
          attributes: {
            name: 'Updated User',
            email: 'updated@example.com',
            password: 'newpass',
            is_admin: false,
            show_portal: false,
            show_chat: true,
            email_verified: false,
            notifications_enabled: false,
            access_to_sso_config: false,
            groups: [3],
            skip_quick_start: undefined
          }
        }
      };
      const mockResponse = { data: { data: { id: mockUserId, ...mockUserData } } };
      jest.spyOn(apiClient, 'patch').mockResolvedValueOnce(mockResponse);
      const result = await userService.updateUser(mockUserId, mockUserData);
      expect(apiClient.patch).toHaveBeenCalledWith(`/users/${mockUserId}`, expectedPayload);
      expect(result).toEqual(mockResponse.data.data);
    });

    it('should default show_chat to true if undefined and include default boolean fields', async () => {
      const mockUserId = 'user-2';
      const mockUserData = {
        name: 'User',
        email: 'user@example.com',
        isAdmin: false,
        showPortal: false
      };
      const expectedPayload = expect.objectContaining({
        data: expect.objectContaining({
          attributes: expect.objectContaining({ 
            show_chat: true,
            email_verified: false,
            notifications_enabled: false,
            access_to_sso_config: false,
            groups: []
          })
        })
      });
      jest.spyOn(apiClient, 'patch').mockResolvedValueOnce({ data: { data: {} } });
      await userService.updateUser(mockUserId, mockUserData);
      expect(apiClient.patch).toHaveBeenCalledWith(`/users/${mockUserId}`, expectedPayload);
    });

    it('should exclude password field when not provided', async () => {
      const mockUserId = 'user-3';
      const mockUserData = {
        name: 'User',
        email: 'user@example.com',
        isAdmin: false,
        showPortal: false
      };
      const expectedPayload = expect.objectContaining({
        data: expect.objectContaining({
          attributes: expect.not.objectContaining({ 
            password: expect.anything()
          })
        })
      });
      jest.spyOn(apiClient, 'patch').mockResolvedValueOnce({ data: { data: {} } });
      await userService.updateUser(mockUserId, mockUserData);
      expect(apiClient.patch).toHaveBeenCalledWith(`/users/${mockUserId}`, expectedPayload);
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('fail');
      jest.spyOn(apiClient, 'patch').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      await expect(userService.updateUser('id', {})).rejects.toThrow('handled');
    });
  });
});
