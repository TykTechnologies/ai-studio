import axios from "axios";
import { basePath, csrfTokenHeader, csrfTokenURL } from "../../runtimeConfig";

// getBaseUrl is Studio's own URL: the page's origin plus the base path it is
// served under.
export const getBaseUrl = () => {
  const host = window.location.host;
  const protocol = window.location.protocol;
  return `${protocol}//${host}${basePath()}`;
};

// fetchCSRFToken gets a token for a cookie-authenticated write; send it in
// the csrfTokenHeader() request header.
export const fetchCSRFToken = async () => {
  try {
    const response = await axios.get(csrfTokenURL(), {
      withCredentials: true,
    });
    return response.headers[csrfTokenHeader().toLowerCase()];
  } catch (error) {
    console.error("Error fetching CSRF token:", error);
    return null;
  }
};
