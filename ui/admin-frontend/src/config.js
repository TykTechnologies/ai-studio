const config = {
  API_BASE_URL: process.env.REACT_APP_API_BASE_URL || "http://localhost:8080",
  WEBSOCKET_HOST: process.env.REACT_APP_WEBSOCKET_HOST || "ws://localhost:8080",
};

export default config;
