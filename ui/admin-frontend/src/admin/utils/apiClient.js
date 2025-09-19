import axios from 'axios';
import { fetchCSRFToken } from './urlUtils';

let apiClientInstance = null;

const createApiClient = () => {
  const instance = axios.create({
    baseURL: '/api/v1',
    withCredentials: true,
  });

  instance.interceptors.request.use(
    async (config) => {
      if (config.method !== 'get') {
        const token = await fetchCSRFToken();
        if (token) {
          config.headers["X-CSRF-Token"] = token;
        }
      }
      return config;
    },
    (error) => Promise.reject(error)
  );

  instance.interceptors.response.use(
    (response) => response,
    (error) => {
      if (error.response?.status === 401) {
        // Handle unauthorized access
        window.location.href = '/login';
      }
      return Promise.reject(error);
    }
  );

  // No method overrides


  return instance;
};

// Initialize the API client instance
apiClientInstance = createApiClient();

// Provider API endpoints
export const providerAPI = {
  listProviders: () => apiClientInstance.get('/providers'),
  configureProvider: (providerId, config) => apiClientInstance.post(`/providers/${providerId}/configure`, {
    config,
  }),
  getProviderSpecs: (providerId) => apiClientInstance.get(`/providers/${providerId}/specs`),
};

// App-Tool API endpoints
export const appToolAPI = {
  getAppTools: (appId) => apiClientInstance.get(`/apps/${appId}/tools`),
  addToolToApp: (appId, toolId) => apiClientInstance.post(`/apps/${appId}/tools/${toolId}`),
  removeToolFromApp: (appId, toolId) => apiClientInstance.delete(`/apps/${appId}/tools/${toolId}`),
  // Assuming a general endpoint to get all available tools for selection
  // Adjust if tools are fetched differently (e.g., from tool-catalogues)
  listAvailableTools: (params) => apiClientInstance.get('/tools', { params: { all: true, ...params } }),
};

// Plugin RPC call function for secure plugin communication
export const pluginRPCCall = async (pluginId, method, payload = {}) => {
  // Security: Check admin context from entitlements
  const entitlements = window.adminEntitlements;
  if (!entitlements?.is_admin) {
    throw new Error('Plugin RPC calls require admin permissions');
  }

  // Call through secure apiClient (handles auth, CSRF, CORS)
  try {
    const response = await apiClientInstance.post(`/plugins/${pluginId}/rpc/${method}`, payload);
    return response.data;
  } catch (error) {
    console.error(`Plugin RPC call failed: ${method}`, error);
    throw error;
  }
};

// Function to reinitialize the API client with new configuration
export const reinitializeApiClient = () => {
  apiClientInstance = createApiClient();
  return apiClientInstance;
};

export default apiClientInstance;
