import axios from "axios";

const pubClient = axios.create({
  baseURL: "http://localhost:8080/auth",
  withCredentials: true, // This is important for handling cookies
});

pubClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      // Redirect to login page if unauthorized
      window.location.href = "/login";
    }
    return Promise.reject(error);
  },
);

export default pubClient;
