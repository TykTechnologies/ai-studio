import yaml from "js-yaml";

/**
 * Utility functions for handling OpenAPI specifications
 */

/**
 * Validate URL format
 * @param {string} url - URL to validate
 * @returns {boolean} True if valid, throws error if invalid
 */
export const validateUrl = (url) => {
  try {
    new URL(url);
    return true;
  } catch (error) {
    throw new Error("Invalid URL format");
  }
};

/**
 * Parse specification string based on format
 * @param {string} spec - The specification string
 * @param {string} format - Format of the spec ('json' or 'yaml')
 * @returns {object} Parsed specification object
 */
const parseSpec = (spec, format = null) => {
  try {
    // If format is not provided, detect it
    if (!format) {
      format = detectFormat(spec);
    }

    if (format === "yaml" || format === "yml") {
      return yaml.load(spec);
    }
    return JSON.parse(spec);
  } catch (error) {
    throw new Error(`Invalid ${format.toUpperCase()} format: ${error.message}`);
  }
};

/**
 * Convert a string or object to base64 encoded string
 * @param {string|object} spec - The OpenAPI specification
 * @returns {string} Base64 encoded specification
 */
export const encodeSpec = (spec) => {
  try {
    // If spec is an object, stringify it
    const specString = typeof spec === "string" ? spec : JSON.stringify(spec);
    // Convert to base64
    return btoa(specString);
  } catch (error) {
    console.error("Failed to encode spec:", error);
    throw new Error("Failed to encode OpenAPI specification");
  }
};

/**
 * Validate OpenAPI specification
 * @param {string|object} spec - The OpenAPI specification
 * @param {string} format - Format of the spec ('json' or 'yaml')
 * @returns {boolean} True if valid, throws error if invalid
 */
export const validateSpec = (spec, format = null) => {
  try {
    // Parse the spec based on format
    const parsedSpec = typeof spec === "string" ? parseSpec(spec, format) : spec;

    // Basic OpenAPI validation
    if (!parsedSpec.openapi && !parsedSpec.swagger) {
      throw new Error("Missing OpenAPI/Swagger version");
    }

    if (!parsedSpec.paths) {
      throw new Error("Missing paths definition");
    }

    if (!parsedSpec.info) {
      throw new Error("Missing API information");
    }

    return true;
  } catch (error) {
    throw new Error("Invalid OpenAPI specification: " + error.message);
  }
};

/**
 * Extract operations from OpenAPI spec
 * @param {string|object} spec - The OpenAPI specification
 * @param {string} format - Format of the spec ('json' or 'yaml')
 * @returns {string[]} Array of operation IDs
 */
export const extractOperations = (spec, format = null) => {
  try {
    const specObj = typeof spec === "string" ? parseSpec(spec, format) : spec;
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
    console.error("Failed to extract operations:", error);
    return [];
  }
};

/**
 * Extract authentication details from OpenAPI spec
 * @param {string|object} spec - The OpenAPI specification
 * @param {string} format - Format of the spec ('json' or 'yaml')
 * @returns {object} Authentication details
 */
export const extractAuthDetails = (spec, format = null) => {
  try {
    const specObj = typeof spec === "string" ? parseSpec(spec, format) : spec;
    const auth = {
      type: "",
      name: "",
      in: "",
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
    console.error("Failed to extract auth details:", error);
    return { type: "", name: "", in: "" };
  }
};

/**
 * Detect specification format from content or filename
 * @param {string} content - The specification content
 * @param {string} filename - Optional filename
 * @returns {string} Format ('json' or 'yaml')
 */
export const detectFormat = (content, filename = "") => {
  // First try to determine from filename or URL
  if (filename) {
    // Handle both file paths and URLs
    const path = filename.toLowerCase();
    if (path.endsWith('.yaml') || path.endsWith('.yml')) {
      return "yaml";
    }
    if (path.endsWith('.json')) {
      return "json";
    }
  }

  // If no clear extension, try to detect from content
  // Check for common YAML indicators first
  if (content.trim().startsWith('openapi:') || content.trim().startsWith('swagger:')) {
    return "yaml";
  }

  // Try to parse as JSON
  try {
    JSON.parse(content);
    return "json";
  } catch {
    // If JSON parsing fails, try YAML
    try {
      yaml.load(content);
      return "yaml";
    } catch {
      throw new Error("Unable to determine specification format");
    }
  }
};
