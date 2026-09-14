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

// User API key lifecycle. Rolling returns the fresh key once in the
// response; revoking clears it. The server refuses to roll a key for an
// SSO-provisioned user unless ALLOW_SSO_USER_API_KEYS is set.
export const rollApiKey = async (userId) => {
  try {
    const response = await apiClient.post(`/users/${userId}/roll-api-key`);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const revokeApiKey = async (userId) => {
  try {
    const response = await apiClient.delete(`/users/${userId}/api-key`);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

// Account switch. Disabling blocks every authentication path for the user
// until they are enabled again.
export const setUserDisabled = async (userId, disabled) => {
  try {
    const response = await apiClient.post(`/users/${userId}/${disabled ? "disable" : "enable"}`);
    return response.data?.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

// Display helpers shared by the Users list and the user detail page.
export const AUTH_SOURCE_LABELS = {
  local: "Self-registered",
  admin: "Admin-created",
  sso: "SSO",
};

export const authSourceLabel = (source) => AUTH_SOURCE_LABELS[source] || "Unknown";

export const formatLastLogin = (attributes) => {
  if (!attributes?.last_login_at) return "Never";
  const when = new Date(attributes.last_login_at).toLocaleString();
  const method = attributes.last_login_method === "sso"
    ? "SSO"
    : attributes.last_login_method === "password" ? "password" : "";
  return method ? `${when} (${method})` : when;
};