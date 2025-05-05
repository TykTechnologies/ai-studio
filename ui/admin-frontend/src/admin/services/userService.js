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
          show_chat: userData.showChat !== undefined ? userData.showChat : true
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
          password: userData.password,
          is_admin: userData.isAdmin,
          show_portal: userData.showPortal,
          show_chat: userData.showChat !== undefined ? userData.showChat : true,
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