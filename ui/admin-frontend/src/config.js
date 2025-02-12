import pubClient from "./admin/utils/pubClient";
import { getBaseUrl } from "./admin/utils/urlUtils";

const isDev = process.env.NODE_ENV === "development";
const getHost = () => window.location.host;
const getProtocol = () => window.location.protocol;

export let config = {
  API_BASE_URL: `${getProtocol()}//${getHost()}`, // Uses current window host
  WEBSOCKET_HOST: `ws${window.location.protocol === "https:" ? "s" : ""}://${getHost()}`,
};

export const getConfig = () => config;

export const loadConfig = async () => {
  try {
    console.log("CALLING CONFIG");
    const response = await pubClient.get("/auth/config");
    if (response.status === 200) {
      const dynamicConfig = response.data;
      config = {
        ...config,
        ...dynamicConfig,
      };
    }
  } catch (error) {
    console.error("Failed to load configuration:", error);
  }
  return config;
};
