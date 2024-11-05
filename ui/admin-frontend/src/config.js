let config = {
  API_BASE_URL: "http://localhost:8080", // Default fallback values
  WEBSOCKET_HOST: "ws://localhost:8080",
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

export default config;
