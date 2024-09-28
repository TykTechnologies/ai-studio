import axios from "axios";
import config from "../../config";

const apiClient = axios.create({
  baseURL: `${config.API_BASE_URL}/api/v1`,
  withCredentials: true, // This is important for handling cookies
});

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      // Redirect to login page if unauthorized
      window.location.href = "/login";
    }
    return Promise.reject(error);
  },
);

export default apiClient;
