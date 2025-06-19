import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';

export const createUser = async (userData) => {
  try {
    const userPayload = {
      data: {
        type: "User",
        attributes: {
          name: userData.name,
          email: userData.email,
          password: userData.password,
          is_admin: userData.isAdmin,
          show_portal: userData.showPortal,
          show_chat: userData.showChat !== undefined ? userData.showChat : true,
          email_verified: userData.emailVerified || false,
          notifications_enabled: userData.notificationsEnabled || false,
          access_to_sso_config: userData.accessToSSOConfig || false,
          groups: userData.groups || []
        }
      }
    };
    
    const response = await apiClient.post('/users', userPayload);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const updateUser = async (userId, userData) => {
  try {
    const userPayload = {
      data: {
        type: "User",
        attributes: {
          name: userData.name,
          email: userData.email,
          ...(userData.password && { password: userData.password }),
          is_admin: userData.isAdmin,
          show_portal: userData.showPortal,
          show_chat: userData.showChat !== undefined ? userData.showChat : true,
          email_verified: userData.emailVerified || false,
          notifications_enabled: userData.notificationsEnabled || false,
          access_to_sso_config: userData.accessToSSOConfig || false,
          groups: userData.groups || [],
          skip_quick_start: userData.skipQuickStart
        }
      }
    };
    
    const response = await apiClient.patch(`/users/${userId}`, userPayload);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const skipQuickStartForUser = async (userId) => {
  try {
    const response = await apiClient.post(`/users/${userId}/skip-quick-start`);
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getUsers = async (page = 1, options = {}) => {
  try {
    const params = { 
      page,
      ...options
    };
    
    const response = await apiClient.get('/users', {
      params
    });
    
    return {
      data: response.data.data,
      totalCount: parseInt(response.headers['x-total-count'] || '0', 10),
      totalPages: parseInt(response.headers['x-total-pages'] || '0', 10)
    };
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getUser = async (userId) => {
  try {
    const response = await apiClient.get(`/users/${userId}`);
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const deleteUser = async (userId) => {
  try {
    const response = await apiClient.delete(`/users/${userId}`);
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};