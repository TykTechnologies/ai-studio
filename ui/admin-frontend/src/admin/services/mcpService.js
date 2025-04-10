import pubClient from "../utils/pubClient";

/**
 * MCP Service - Provides methods for interacting with the MCP server API
 */
const mcpService = {
  /**
   * Server Management
   */

  // Get all MCP servers
  getServers: async () => {
    const response = await pubClient.get("/common/mcp-servers");
    return response.data;
  },

  // Get a specific MCP server by ID
  getServer: async (id) => {
    const response = await pubClient.get(`/common/mcp-servers/${id}`);
    return response.data;
  },

  // Create a new MCP server
  createServer: async (data) => {
    const response = await pubClient.post("/common/mcp-servers", data);
    return response.data;
  },

  // Update an existing MCP server
  updateServer: async (id, data) => {
    const response = await pubClient.patch(`/common/mcp-servers/${id}`, data);
    return response.data;
  },

  // Delete an MCP server
  deleteServer: async (id) => {
    await pubClient.delete(`/common/mcp-servers/${id}`);
    return true;
  },

  /**
   * Server Control
   */

  // Start an MCP server
  startServer: async (id) => {
    await pubClient.post(`/common/mcp-servers/${id}/start`);
    return true;
  },

  // Stop an MCP server
  stopServer: async (id) => {
    await pubClient.post(`/common/mcp-servers/${id}/stop`);
    return true;
  },

  // Restart an MCP server
  restartServer: async (id) => {
    await pubClient.post(`/common/mcp-servers/${id}/restart`);
    return true;
  },

  /**
   * Tool Management
   */

  // Get all tools associated with an MCP server
  getServerTools: async (id) => {
    const response = await pubClient.get(`/common/mcp-servers/${id}/tools`);
    return response.data;
  },

  // Add a tool to an MCP server
  addToolToServer: async (serverId, toolId) => {
    // Convert toolId to a number if it's a string
    const numericToolId = parseInt(toolId, 10);
    
    const response = await pubClient.post(`/common/mcp-servers/${serverId}/tools`, {
      tool_id: numericToolId,
    });
    return response.data;
  },

  // Remove a tool from an MCP server
  removeToolFromServer: async (serverId, toolId) => {
    await pubClient.delete(`/common/mcp-servers/${serverId}/tools/${toolId}`);
    return true;
  },

  // Get all available tools (that can be added to an MCP server)
  getAvailableTools: async () => {
    const response = await pubClient.get("/common/accessible-tools");
    // Return the response in the expected format with a data property
    if (Array.isArray(response.data)) {
      // If the response is already an array, wrap it in a data property
      return { data: response.data };
    }
    return response.data; // If it already has a data property
  },

  /**
   * Session Management
   */

  // Get all active sessions for an MCP server
  getSessions: async (serverId) => {
    // Assuming there's an endpoint to get sessions by server ID
    const response = await pubClient.get(`/common/mcp/sessions?mcp_server_id=${serverId}`);
    return response.data;
  },

  // Create a new session
  createSession: async (data) => {
    const response = await pubClient.post("/common/mcp/sessions", data);
    return response.data;
  },

  // End a session
  endSession: async (sessionId) => {
    await pubClient.delete(`/common/mcp/sessions/${sessionId}`);
    return true;
  },

  // Update session activity (keep alive)
  updateSessionActivity: async (sessionId) => {
    await pubClient.patch(`/common/mcp/sessions/${sessionId}/activity`);
    return true;
  },

  /**
   * MCP Event Helper
   */
  
  // Create an EventSource for real-time updates from an MCP server
  createEventSource: (serverId) => {
    const eventSource = new EventSource(`/common/mcp/sse?server_id=${serverId}`);
    return eventSource;
  }
};

export default mcpService;
