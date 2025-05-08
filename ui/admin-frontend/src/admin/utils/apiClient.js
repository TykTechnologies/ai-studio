import axios from 'axios';
import { fetchCSRFToken } from './urlUtils';

let apiClientInstance = null;

const createApiClient = () => {
  const instance = axios.create({
    baseURL: '/api/v1',
    withCredentials: true,
  });

  // Add a request interceptor to include CSRF token for mutating requests
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

  // Add a response interceptor to handle errors
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

// Function to reinitialize the API client with new configuration
export const reinitializeApiClient = () => {
  apiClientInstance = createApiClient();
  return apiClientInstance;
};

export default apiClientInstance;
