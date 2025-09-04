import axios from "axios";
import { getBaseUrl, fetchCSRFToken } from "./urlUtils";
import cacheService from "./cacheService";
import { CACHE_KEYS } from "./constants";

const isDev = process.env.NODE_ENV === "development";

// Map to store pending requests to prevent duplicate concurrent calls
const pendingRequests = new Map();

// Enhanced cache configuration for different endpoints
const CACHE_CONFIG = {
  '/common/system': {
    key: CACHE_KEYS.FEATURES,
    expiry: 30 * 60 * 1000, // 30 minutes - system features rarely change
  },
  '/common/me': {
    key: 'tyk_ai_studio_user_profile',
    expiry: 5 * 60 * 1000, // 5 minutes - user data changes infrequently
  },
  // Add more cacheable endpoints as needed
};

const cachedPubClient = axios.create({
  baseURL: getBaseUrl(),
  withCredentials: true,
});

// Enhanced request interceptor with caching and deduplication
cachedPubClient.interceptors.request.use(
  async (config) => {
    // Add CSRF token for non-GET requests
    if (config.method !== "get") {
      const token = await fetchCSRFToken();
      if (token) {
        config.headers["X-CSRF-Token"] = token;
      }
    }

    // Handle caching for GET requests
    if (config.method === "get" || !config.method) {
      const cacheConfig = CACHE_CONFIG[config.url];
      
      if (cacheConfig) {
        // Check if we have cached data
        const cachedData = cacheService.get(cacheConfig.key);
        if (cachedData) {
          // Return cached data as a resolved promise
          return Promise.reject({
            isCache: true,
            data: cachedData,
            config
          });
        }

        // Check if there's already a pending request for this endpoint
        const requestKey = `${config.method || 'get'}:${config.url}`;
        if (pendingRequests.has(requestKey)) {
          // Return the existing pending promise
          return Promise.reject({
            isPending: true,
            promise: pendingRequests.get(requestKey),
            config
          });
        }
      }
    }

    return config;
  },
  (error) => {
    return Promise.reject(error);
  },
);

// Enhanced response interceptor with caching
cachedPubClient.interceptors.response.use(
  (response) => {
    const cacheConfig = CACHE_CONFIG[response.config.url];
    
    // Cache successful GET responses
    if (cacheConfig && (response.config.method === "get" || !response.config.method)) {
      cacheService.set(cacheConfig.key, response.data, cacheConfig.expiry);
    }

    // Remove from pending requests
    const requestKey = `${response.config.method || 'get'}:${response.config.url}`;
    pendingRequests.delete(requestKey);

    return response;
  },
  (error) => {
    // Handle cached responses
    if (error.isCache) {
      return Promise.resolve({
        data: error.data,
        status: 200,
        statusText: 'OK (cached)',
        headers: {},
        config: error.config,
        fromCache: true
      });
    }

    // Handle pending requests
    if (error.isPending) {
      return error.promise;
    }

    // Handle authentication errors
    if (error.response && error.response.status === 401) {
      // Clear any stored auth data including cached user data
      localStorage.clear();
      cacheService.clear('tyk_ai_studio_');

      // Only redirect if we're not already on the login page
      if (!window.location.pathname.includes("/login")) {
        window.location.href = "/login";
      }
      return Promise.reject(error);
    }

    // Remove from pending requests on error
    if (error.config) {
      const requestKey = `${error.config.method || 'get'}:${error.config.url}`;
      pendingRequests.delete(requestKey);
    }

    return Promise.reject(error);
  },
);

// Override the get method to implement request deduplication
const originalGet = cachedPubClient.get;
cachedPubClient.get = function(url, config = {}) {
  const requestKey = `get:${url}`;
  
  // If there's already a pending request, return that promise
  if (pendingRequests.has(requestKey)) {
    return pendingRequests.get(requestKey);
  }
  
  // Create new request and store it
  const requestPromise = originalGet.call(this, url, config)
    .finally(() => {
      // Clean up after request completes
      pendingRequests.delete(requestKey);
    });
  
  pendingRequests.set(requestKey, requestPromise);
  return requestPromise;
};

export const logout = async () => {
  try {
    await cachedPubClient.post("/common/logout");
    localStorage.clear();
    cacheService.clear('tyk_ai_studio_');
    window.location.href = "/login";
  } catch (error) {
    console.error("Logout failed:", error);
    // Only redirect if we're not already on the login page
    if (!window.location.pathname.includes("/login")) {
      window.location.href = "/login";
    }
  }
};

// Function to invalidate specific cache entries
export const invalidateCache = (endpoints) => {
  if (Array.isArray(endpoints)) {
    endpoints.forEach(endpoint => {
      const cacheConfig = CACHE_CONFIG[endpoint];
      if (cacheConfig) {
        cacheService.remove(cacheConfig.key);
      }
    });
  }
};

// Function to clear all user-related cache (useful on login/logout)
export const clearUserCache = () => {
  cacheService.clear('tyk_ai_studio_');
};

// Export a function to reinitialize the client with updated config
export const reinitializeCachedPubClient = () => {
  cachedPubClient.defaults.baseURL = isDev ? "" : getBaseUrl();
};

export default cachedPubClient;
