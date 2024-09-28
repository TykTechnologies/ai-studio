import axios from "axios";
import config from "../../config";

const pubClient = axios.create({
  baseURL: config.API_BASE_URL,
  withCredentials: true, // This is important for handling cookies
});

export default pubClient;
