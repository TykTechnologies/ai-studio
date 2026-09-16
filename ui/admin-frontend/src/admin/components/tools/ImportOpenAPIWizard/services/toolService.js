import apiClient from '../../../../utils/apiClient';
import { encodeSpec, validateSpec } from '../utils/specUtils';

/**
 * Create a new tool
 * @param {object} toolData - The tool data
 * @returns {Promise} Tool creation response
 */
export const createTool = async (toolData) => {
  try {
    // Validate and encode the OAS spec
    validateSpec(toolData.oas_spec);
    const encodedSpec = encodeSpec(toolData.oas_spec);

    // Prepare the tool input
    const toolInput = {
      data: {
        type: 'tools',
        attributes: {
          name: toolData.name,
          description: toolData.description,
          tool_type: toolData.tool_type,
          oas_spec: encodedSpec,
          privacy_score: parseInt(toolData.privacy_score) || 0,
          auth_schema_name: toolData.auth_schema_name,
          auth_key: toolData.auth_key,
          operations: toolData.operations || [],
          file_stores: toolData.file_stores || [],
          filters: toolData.filters || [],
          dependencies: toolData.dependencies || []
        },
      },
    };
    // Create the tool
    const response = await apiClient.post('/tools', toolInput);
    return response.data;
  } catch (error) {
    console.error('Error creating tool:', error);
    throw error;
  }
};

/**
 * Create tool with operations
 * @param {object} toolData - Tool data
 * @param {string[]} operations - Array of operation IDs
 * @returns {Promise} Created tool with operations
 */
export const createToolWithOperations = async (toolData, operations) => {
  try {
    // Include operations in the tool data
    const toolWithOperations = {
      ...toolData,
      operations: operations || []
    };
    
    // Create the tool with operations
    const tool = await createTool(toolWithOperations);
    return tool;
  } catch (error) {
    console.error('Error creating tool with operations:', error);
    throw error;
  }
};

// --- Tyk Dashboard import (Enterprise) ---
// The wizard reads a Dashboard through a saved Tyk connection. These routes
// are scoped to the tools permission and return no secrets.

/** Whether the Tyk integration is available and switched on. */
export const getTykStatus = async () => {
  const response = await apiClient.get('/tyk-mcp/status');
  return response.data;
};

/** Non-disabled Tyk connections, as a slim projection. */
export const listTykConnections = async () => {
  const response = await apiClient.get('/tools/import/tyk/connections');
  return response.data || [];
};

/** Tyk OAS APIs on one connection. */
export const listTykAPIs = async (connectionId, q = '') => {
  const response = await apiClient.get(`/tools/import/tyk/connections/${connectionId}/apis`, {
    params: q ? { q } : undefined,
  });
  return response.data || [];
};

/** One Tyk OAS API definition with upstream credentials masked. */
export const getTykAPIDocument = async (connectionId, apiId) => {
  const response = await apiClient.get(
    `/tools/import/tyk/connections/${connectionId}/apis/${encodeURIComponent(apiId)}`
  );
  return response.data;
};
