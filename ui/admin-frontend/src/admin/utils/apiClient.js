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

  // Override the post method to add logging
  const originalPost = instance.post;
  instance.post = async function(url, data, config) {
    console.log(`API POST ${url}`, data);
    try {
      const response = await originalPost.call(this, url, data, config);
      console.log(`API POST ${url} response:`, response);
      return response;
    } catch (error) {
      console.error(`API POST ${url} error:`, error);
      throw error;
    }
  };
  
  // Add similar logging for other methods if needed (e.g., put, patch, delete)
  const originalDelete = instance.delete;
  instance.delete = async function(url, config) {
    console.log(`API DELETE ${url}`);
    try {
      const response = await originalDelete.call(this, url, config);
      console.log(`API DELETE ${url} response:`, response);
      return response;
    } catch (error) {
      console.error(`API DELETE ${url} error:`, error);
      throw error;
    }
  };


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


// Function to reinitialize the API client with new configuration
export const reinitializeApiClient = () => {
  apiClientInstance = createApiClient();
  return apiClientInstance;
};

export default apiClientInstance;
