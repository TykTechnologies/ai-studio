// src/apiClient.js
import axios from "axios";

const apiClient = axios.create({
  baseURL: "http://localhost:8080/api/v1", // Update with your actual base URL
  headers: {
    "Content-Type": "application/json",
  },
});

// Dev mode token
const devToken = "YOUR_DEV_TOKEN_HERE"; // Replace with your actual token

// Request interceptor to add the Bearer token to headers
apiClient.interceptors.request.use(
  (config) => {
    // Check if dev mode token is set
    const token = devToken || localStorage.getItem("token"); // Use devToken if set, else check localStorage
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error),
);

export default apiClient;
