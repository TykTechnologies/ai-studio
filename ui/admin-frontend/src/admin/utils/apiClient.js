import axios from "axios";
import { getConfig } from "../../config";

const createApiClient = () => {
  const config = getConfig();
  return axios.create({
    baseURL: `${config.API_BASE_URL}/api/v1`,
    withCredentials: true,
  });
};

let apiClient = createApiClient();

// Function to fetch CSRF token
const fetchCSRFToken = async () => {
  const config = getConfig();
  try {
    const response = await axios.get(`${config.API_BASE_URL}/csrf-token`, {
      withCredentials: true,
    });
    return response.headers["x-csrf-token"];
  } catch (error) {
    console.error("Error fetching CSRF token:", error);
    return null;
  }
};

// Request interceptor to add CSRF token
apiClient.interceptors.request.use(
  async (config) => {
    if (config.method !== "get") {
      const token = await fetchCSRFToken();
      if (token) {
        config.headers["X-CSRF-Token"] = token;
      }
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  },
);

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      window.location.href = "/login";
    }
    return Promise.reject(error);
  },
);

// Export a function to reinitialize the client with updated config
export const reinitializeApiClient = () => {
  apiClient = createApiClient();
};

export default apiClient;
