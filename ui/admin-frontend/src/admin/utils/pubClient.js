import axios from "axios";

const pubClient = axios.create({
  baseURL: "http://localhost:8080/",
  withCredentials: true, // This is important for handling cookies
});

export default pubClient;
