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
 * Get providers list
 * @returns {Promise} Providers list response
 */
export const getProviders = async () => {
  try {
    const response = await apiClient.get('/providers');
    return response.data.data;
  } catch (error) {
    console.error('Error fetching providers:', error);
    throw error;
  }
};

/**
 * Configure provider
 * @param {string} providerId - Provider ID
 * @param {object} config - Provider configuration
 * @returns {Promise} Provider configuration response
 */
export const configureProvider = async (providerId, config) => {
  try {
    await apiClient.post(`/providers/${providerId}/configure`, { config });
    const response = await apiClient.get(`/providers/${providerId}/specs`);
    return response.data.data;
  } catch (error) {
    console.error('Error configuring provider:', error);
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
