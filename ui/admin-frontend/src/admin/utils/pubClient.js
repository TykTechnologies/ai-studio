import axios from "axios";
import config from "../../config";

const pubClient = axios.create({
  baseURL: config.API_BASE_URL,
  withCredentials: true, // This is important for handling cookies
});

// Function to fetch CSRF token
const fetchCSRFToken = async () => {
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

export default pubClient;
