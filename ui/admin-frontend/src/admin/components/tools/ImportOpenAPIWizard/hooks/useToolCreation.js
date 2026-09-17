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
        // Store what was entered. `|| 50` used to turn an explicit 0 into 50
        // (UX review F-06); only an empty field falls back to the wizard's
        // default of 25 (Public), the same default index.js starts with.
        privacy_score: toolConfig.privacy_score === '' || toolConfig.privacy_score == null
          ? 25
          : Number(toolConfig.privacy_score),
        auth_schema_name: authDetails.name || toolConfig.auth_schema_name,
        auth_key: toolConfig.auth_key || '',
        // Access methods: an imported tool is chat only unless the admin
        // turned REST or MCP on in the last step.
        rest_access_enabled: Boolean(toolConfig.rest_access_enabled),
        mcp_access_enabled: Boolean(toolConfig.mcp_access_enabled),
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
