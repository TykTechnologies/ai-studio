import axios from "axios";
import { getBaseUrl } from "./urlUtils";

const isDev = process.env.NODE_ENV === "development";

const pubClient = axios.create({
  baseURL: isDev ? "http://localhost:8080" : getBaseUrl(), // Hardcode backend URL for dev
  withCredentials: true,
});

pubClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      // Clear any stored auth data
      localStorage.clear();

      // Only redirect if we're not already on the login page
      if (!window.location.pathname.includes("/login")) {
        window.location.href = "/login";
      }
      return Promise.reject(error);
    }
    return Promise.reject(error);
  },
);

// Function to fetch CSRF token
const fetchCSRFToken = async () => {
  try {
    const response = await axios.get(isDev ? "http://localhost:8080/csrf-token" : `${getBaseUrl()}/csrf-token`, {
      withCredentials: true,
    });
    return response.headers["x-csrf-token"];
  } catch (error) {
    console.error("Error fetching CSRF token:", error);
    return null;
  }
};

// Request interceptor to add CSRF token
pubClient.interceptors.request.use(
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

export const logout = async () => {
  try {
    await pubClient.post("/common/logout");
    localStorage.clear();
    window.location.href = "/login";
  } catch (error) {
    console.error("Logout failed:", error);
    // Only redirect if we're not already on the login page
    if (!window.location.pathname.includes("/login")) {
      window.location.href = "/login";
    }
  }
};

// Export a function to reinitialize the client with updated config
export const reinitializePubClient = () => {
  pubClient.defaults.baseURL = isDev ? "" : getBaseUrl();
};

export default pubClient;
