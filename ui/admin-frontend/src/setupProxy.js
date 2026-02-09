// setupProxy.js - Configure proxy for development server
// This allows the proxy target to be configured via environment variables,
// which is required for Docker Compose where containers communicate via service names.
//
// Environment variables:
//   PROXY_TARGET - The backend URL to proxy to (default: http://localhost:8080)
//
// In Docker Compose, set PROXY_TARGET=http://studio:8080

const { createProxyMiddleware } = require('http-proxy-middleware');

module.exports = function(app) {
  const target = process.env.PROXY_TARGET || 'http://localhost:8080';

  console.log(`[setupProxy] Proxying API requests to: ${target}`);

  // Proxy all API routes to the backend
  app.use(
    ['/api', '/auth', '/common', '/ws', '/csrf-token', '/health'],
    createProxyMiddleware({
      target: target,
      changeOrigin: true,
      ws: true, // Enable WebSocket proxy
      onError: (err, req, res) => {
        console.error(`[setupProxy] Proxy error: ${err.message}`);
      },
    })
  );
};
