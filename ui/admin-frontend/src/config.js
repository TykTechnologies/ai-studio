const isDev = process.env.NODE_ENV === "development";
const getHost = () => (isDev ? "localhost:8080" : `${window.location.host}`);
const getProtocol = () => window.location.protocol;

export let config = {
  API_BASE_URL: `${getProtocol()}//${getHost()}`, // Uses current window host
  WEBSOCKET_HOST: `ws${window.location.protocol === "https:" ? "s" : ""}://${window.location.host}`,
};

export const loadConfig = async () => {
  try {
    const response = await fetch("/config");
    if (response.ok) {
      const dynamicConfig = await response.json();
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

export const getConfig = () => config;
