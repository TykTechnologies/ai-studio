import { useState } from 'react';
import { createToolWithOperations } from '../services/toolService';
import { extractOperations, extractAuthDetails } from '../utils/specUtils';

export const useToolCreation = () => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const createTool = async (toolConfig) => {
    setLoading(true);
    setError('');

    try {
      // Extract operations from spec
      const operations = extractOperations(toolConfig.oas_spec);
      
      // Extract auth details
      const authDetails = extractAuthDetails(toolConfig.oas_spec);

      // Prepare tool data
      const toolData = {
        name: toolConfig.name,
        description: toolConfig.description,
        tool_type: 'REST',
        oas_spec: toolConfig.oas_spec,
        privacy_score: toolConfig.privacy_score || 50,
        auth_schema_name: authDetails.name || toolConfig.auth_schema_name,
        auth_key: toolConfig.auth_key || '',
        file_stores: [],
        filters: [],
        dependencies: []
      };

      // Create tool with operations
      const result = await createToolWithOperations(toolData, operations);
      return result;
    } catch (error) {
      setError(error.message || 'Failed to create tool');
      throw error;
    } finally {
      setLoading(false);
    }
  };

  return {
    createTool,
    loading,
    error
  };
};
