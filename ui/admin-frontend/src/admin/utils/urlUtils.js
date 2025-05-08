import axios from "axios";

export const getBaseUrl = () => {
  const isDev = process.env.NODE_ENV === "development";
  const host = window.location.host;
  const protocol = window.location.protocol;
  return `${protocol}//${host}`;
};

export const fetchCSRFToken = async () => {
  try {
    const response = await axios.get(`${getBaseUrl()}/csrf-token`, {
      withCredentials: true,
    });
    return response.headers["x-csrf-token"];
  } catch (error) {
    console.error("Error fetching CSRF token:", error);
    return null;
  }
};
