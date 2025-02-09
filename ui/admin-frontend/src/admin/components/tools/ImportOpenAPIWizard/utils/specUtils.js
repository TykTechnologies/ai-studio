/**
 * Utility functions for handling OpenAPI specifications
 */

/**
 * Convert a string or object to base64 encoded string
 * @param {string|object} spec - The OpenAPI specification
 * @returns {string} Base64 encoded specification
 */
export const encodeSpec = (spec) => {
  try {
    // If spec is an object, stringify it
    const specString = typeof spec === 'string' ? spec : JSON.stringify(spec);
    // Convert to base64
    return btoa(specString);
  } catch (error) {
    console.error('Failed to encode spec:', error);
    throw new Error('Failed to encode OpenAPI specification');
  }
};

/**
 * Validate OpenAPI specification
 * @param {string|object} spec - The OpenAPI specification
 * @returns {boolean} True if valid, throws error if invalid
 */
export const validateSpec = (spec) => {
  try {
    // If spec is a string, try to parse it
    if (typeof spec === 'string') {
      JSON.parse(spec);
    }
    
    // Additional OpenAPI validation could be added here
    return true;
  } catch (error) {
    throw new Error('Invalid OpenAPI specification: ' + error.message);
  }
};

/**
 * Extract operations from OpenAPI spec
 * @param {string|object} spec - The OpenAPI specification
 * @returns {string[]} Array of operation IDs
 */
export const extractOperations = (spec) => {
  try {
    const specObj = typeof spec === 'string' ? JSON.parse(spec) : spec;
    const operations = [];

    // Extract operationIds from paths
    if (specObj.paths) {
      Object.entries(specObj.paths).forEach(([path, methods]) => {
        Object.entries(methods).forEach(([method, operation]) => {
          if (operation.operationId) {
            operations.push(operation.operationId);
          }
        });
      });
    }

    return operations;
  } catch (error) {
    console.error('Failed to extract operations:', error);
    return [];
  }
};

/**
 * Extract authentication details from OpenAPI spec
 * @param {string|object} spec - The OpenAPI specification
 * @returns {object} Authentication details
 */
export const extractAuthDetails = (spec) => {
  try {
    const specObj = typeof spec === 'string' ? JSON.parse(spec) : spec;
    const auth = {
      type: '',
      name: '',
      in: '',
    };

    // Extract security schemes
    if (specObj.components?.securitySchemes) {
      const schemes = specObj.components.securitySchemes;
      // Get the first security scheme
      const [schemeName, scheme] = Object.entries(schemes)[0] || [];
      if (scheme) {
        auth.type = scheme.type;
        auth.name = schemeName;
        auth.in = scheme.in;
      }
    }

    return auth;
  } catch (error) {
    console.error('Failed to extract auth details:', error);
    return { type: '', name: '', in: '' };
  }
};
