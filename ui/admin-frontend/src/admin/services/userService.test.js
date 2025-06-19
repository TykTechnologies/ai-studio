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

  describe('skipQuickStartForUser', () => {
    it('should POST to /users/:id/skip-quick-start and return data', async () => {
      const mockUserId = 'user-1';
      const mockResponse = { data: { success: true } };
      jest.spyOn(apiClient, 'post').mockResolvedValueOnce(mockResponse);
      
      const result = await userService.skipQuickStartForUser(mockUserId);
      
      expect(apiClient.post).toHaveBeenCalledWith(`/users/${mockUserId}/skip-quick-start`);
      expect(result).toEqual(mockResponse.data);
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('fail');
      jest.spyOn(apiClient, 'post').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      
      await expect(userService.skipQuickStartForUser('user-1')).rejects.toThrow('handled');
    });
  });

  describe('getUsers', () => {
    it('should GET /users with default page and return paginated data', async () => {
      const mockResponse = {
        data: {
          data: [
            { id: 'user-1', name: 'User 1' },
            { id: 'user-2', name: 'User 2' }
          ]
        },
        headers: {
          'x-total-count': '50',
          'x-total-pages': '5'
        }
      };
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      
      const result = await userService.getUsers();
      
      expect(apiClient.get).toHaveBeenCalledWith('/users', {
        params: { page: 1 }
      });
      expect(result).toEqual({
        data: mockResponse.data.data,
        totalCount: 50,
        totalPages: 5
      });
    });

    it('should GET /users with custom page and options', async () => {
      const mockResponse = {
        data: { data: [] },
        headers: {
          'x-total-count': '0',
          'x-total-pages': '0'
        }
      };
      const options = { search: 'test', filter: 'admin' };
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      
      const result = await userService.getUsers(3, options);
      
      expect(apiClient.get).toHaveBeenCalledWith('/users', {
        params: { page: 3, search: 'test', filter: 'admin' }
      });
      expect(result).toEqual({
        data: [],
        totalCount: 0,
        totalPages: 0
      });
    });

    it('should handle missing headers gracefully', async () => {
      const mockResponse = {
        data: { data: [] },
        headers: {}
      };
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      
      const result = await userService.getUsers();
      
      expect(result).toEqual({
        data: [],
        totalCount: 0,
        totalPages: 0
      });
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('fail');
      jest.spyOn(apiClient, 'get').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      
      await expect(userService.getUsers()).rejects.toThrow('handled');
    });
  });

  describe('getUser', () => {
    it('should GET /users/:id and return user data', async () => {
      const mockUserId = 'user-1';
      const mockResponse = {
        data: {
          id: 'user-1',
          name: 'Test User',
          email: 'test@example.com'
        }
      };
      jest.spyOn(apiClient, 'get').mockResolvedValueOnce(mockResponse);
      
      const result = await userService.getUser(mockUserId);
      
      expect(apiClient.get).toHaveBeenCalledWith(`/users/${mockUserId}`);
      expect(result).toEqual(mockResponse.data);
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('fail');
      jest.spyOn(apiClient, 'get').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      
      await expect(userService.getUser('user-1')).rejects.toThrow('handled');
    });
  });

  describe('deleteUser', () => {
    it('should DELETE /users/:id and return response data', async () => {
      const mockUserId = 'user-1';
      const mockResponse = { data: { success: true } };
      jest.spyOn(apiClient, 'delete').mockResolvedValueOnce(mockResponse);
      
      const result = await userService.deleteUser(mockUserId);
      
      expect(apiClient.delete).toHaveBeenCalledWith(`/users/${mockUserId}`);
      expect(result).toEqual(mockResponse.data);
    });

    it('should throw handled error on failure', async () => {
      const error = new Error('fail');
      jest.spyOn(apiClient, 'delete').mockRejectedValueOnce(error);
      jest.spyOn(require('./utils/errorHandler'), 'handleApiError').mockImplementation(e => new Error('handled'));
      
      await expect(userService.deleteUser('user-1')).rejects.toThrow('handled');
    });
  });
});
