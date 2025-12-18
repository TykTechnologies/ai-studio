import React, { useState, useEffect } from "react";
import { useParams } from "react-router-dom";
import {
  Typography,
  Box,
  CircularProgress,
  Alert,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  TableContainer,
  Table,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
  Paper,
  Chip,
  IconButton,
  FormControl,
  Select,
  MenuItem,
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import CheckIcon from "@mui/icons-material/Check";
import pubClient from '../../admin/utils/pubClient';
import { getConfig } from '../../config';

// Helper function to generate example curl commands
const generateCurlExample = (operation, toolDetails, selectedApiToken = null) => {
  if (!operation || !toolDetails) return 'curl example not available';
  
  // Generate slug from tool name for the URL
  const toolSlug = toolDetails.attributes.name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "");
  const operationId = operation.operation_id;
  const method = operation.method.toUpperCase();
  
  // Build parts of the curl command as separate lines
  const parts = [];
  parts.push(`curl -X ${method}`);
  
  // Add content type header if there's a request body
  if (operation.request_body && operation.request_body.content_type) {
    parts.push(`  -H "Content-Type: ${operation.request_body.content_type}"`);
  }
  
  // Add authorization header with selected API token or placeholder
  const apiKey = selectedApiToken || 'YOUR_API_KEY';
  parts.push(`  -H "Authorization: Bearer ${apiKey}"`);
  
  // Build the URL - correct format for the proxy endpoint with proper gateway URL
  const config = getConfig();
  const currentHost = window.location.hostname;
  const protocol = window.location.protocol; // Get the current protocol (http: or https:)
  // Use toolDisplayURL from config if available, otherwise fall back to proxyURL, then default gateway
  const baseUrl = config.toolDisplayURL || config.proxyURL || `${protocol}//${currentHost}:9090`;
  // The correct URL format is just /tools/{toolSlug} - the operation ID goes in the request body
  let url = `${baseUrl}/tools/${toolSlug}`;
  
  // Add path parameters if any
  if (operation.parameters && operation.parameters.length > 0) {
    const pathParams = operation.parameters.filter(p => p.in === 'path');
    if (pathParams.length > 0) {
      pathParams.forEach(param => {
        const paramValue = `{${param.name}}`;
        url += `/${paramValue}`;
      });
    }
  }
  
  // Add query parameters if any
  if (operation.parameters && operation.parameters.length > 0) {
    const queryParams = operation.parameters.filter(p => p.in === 'query');
    if (queryParams.length > 0) {
      url += '?';
      queryParams.forEach((param, index) => {
        const paramValue = param.schema?.type === 'number' ? '1' : 
                          param.schema?.type === 'boolean' ? 'true' : 
                          param.schema?.enum?.length > 0 ? param.schema.enum[0] : 
                          `{${param.name}}`;
        url += `${param.name}=${paramValue}${index < queryParams.length - 1 ? '&' : ''}`;
      });
    }
  }
  
  // Add URL to parts
  parts.push(`  ${url}`);
  
  // Add request body if applicable
  if ((method === 'POST' || method === 'PUT' || method === 'PATCH')) {
    
    // Generate an example request body with only the required operation_id
    const exampleBody = {
      // Always include the operation_id in the request body for tool proxy
      operation_id: operationId
    };
    
    // Only add parameters section if we have URL or query parameters
    if (operation.parameters && operation.parameters.length > 0) {
      const paramData = {};
      operation.parameters.forEach(param => {
        if (param.in === 'query' || param.in === 'path') {
          if (param.example) {
            paramData[param.name] = [param.example.toString()];
          } else if (param.schema && param.schema.type === 'string') {
            paramData[param.name] = [`example_${param.name}`];
          } else if (param.schema && (param.schema.type === 'number' || param.schema.type === 'integer')) {
            paramData[param.name] = ['42'];
          } else if (param.schema && param.schema.type === 'boolean') {
            paramData[param.name] = ['true'];
          }
        }
      });
      
      // Only add parameters if we have any
      if (Object.keys(paramData).length > 0) {
        exampleBody.parameters = paramData;
      }
    }
    
    // If there's a schema defined, add example payload properties
    if (operation.request_body && operation.request_body.schema && operation.request_body.schema.properties) {
      const payloadData = {};
      Object.entries(operation.request_body.schema.properties).forEach(([propName, propSchema]) => {
        if (propSchema.example) {
          payloadData[propName] = propSchema.example;
        } else if (propSchema.type === 'string') {
          payloadData[propName] = `example_${propName}`;
        } else if (propSchema.type === 'number' || propSchema.type === 'integer') {
          payloadData[propName] = 42;
        } else if (propSchema.type === 'boolean') {
          payloadData[propName] = true;
        } else if (propSchema.type === 'array') {
          payloadData[propName] = [];
        } else if (propSchema.type === 'object') {
          payloadData[propName] = {};
        }
      });
      
      // Only add payload if we have properties
      if (Object.keys(payloadData).length > 0) {
        exampleBody.payload = payloadData;
      }
    }
    
    // Add example headers if needed (currently not populated)
    // Only add if we actually have custom headers
    if (operation.headers && operation.headers.length > 0) {
      const headerData = {};
      operation.headers.forEach(header => {
        headerData[header.name] = [header.example || `example_${header.name}`];
      });
      
      if (Object.keys(headerData).length > 0) {
        exampleBody.headers = headerData;
      }
    }
    
    const bodyJson = JSON.stringify(exampleBody, null, 2).replace(/'/g, "\\'");
    parts.push(`  -d '${bodyJson}'`);
  }
  
  // Join parts with newlines and backslashes for continuation
  return parts.join(' \\\n');
};

const ToolDocumentationPage = () => {
  const { id } = useParams();
  const [documentationData, setDocumentationData] = useState([]);
  const [toolDetails, setToolDetails] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [expandedAccordion, setExpandedAccordion] = useState(false);
  const [copiedIndex, setCopiedIndex] = useState(null);
  const [userApps, setUserApps] = useState([]);
  const [selectedApp, setSelectedApp] = useState('');

  const handleAccordionChange = (panel) => (event, isExpanded) => {
    setExpandedAccordion(isExpanded ? panel : false);
  };

  const handleCopyToClipboard = (text, index) => {
    navigator.clipboard.writeText(text)
      .then(() => {
        setCopiedIndex(index);
        setTimeout(() => setCopiedIndex(null), 2000); // Reset after 2 seconds
      })
      .catch((err) => {
        console.error("Failed to copy text: ", err);
      });
  };

  // Helper component for code blocks with copy button
  const CodeBlock = ({ children, copyText, index, fontSize = '0.9rem', sx = {} }) => (
    <Paper sx={{ p: 2, backgroundColor: '#292929', borderRadius: 1, position: 'relative', ...sx }}>
      <Typography sx={{ fontFamily: 'monospace', color: 'white', fontSize, pr: '40px', whiteSpace: 'pre-wrap' }}>
        {children}
      </Typography>
      <IconButton 
        onClick={() => handleCopyToClipboard(copyText || children, index)} 
        sx={{ 
          position: 'absolute', 
          top: '8px', 
          right: '8px', 
          color: 'white',
          backgroundColor: 'rgba(255,255,255,0.1)',
          '&:hover': { backgroundColor: 'rgba(255,255,255,0.2)' }
        }}
        size="small"
        title="Copy to clipboard"
      >
        {copiedIndex === index ? <CheckIcon fontSize="small" /> : <ContentCopyIcon fontSize="small" />}
      </IconButton>
    </Paper>
  );

  const handleAppSelection = (event) => {
    setSelectedApp(event.target.value);
  };

  const getSelectedApiToken = () => {
    if (!selectedApp) return null;
    const app = userApps.find(app => app.id === selectedApp);
    return app?.api_secret || null;
  };

  useEffect(() => {
    const fetchData = async () => {
      if (!id) {
        setLoading(false);
        setError({ message: "Tool ID is missing." });
        return;
      }
      setLoading(true);
      setError(null);
      setDocumentationData(null);
      setToolDetails(null);

      try {
        // Fetch documentation
        const docsResponse = await pubClient.get(`/common/tools/${id}/docs`);
        setDocumentationData(docsResponse.data);
        
        // Fetch accessible tools and find the tool by ID
        const toolsResponse = await pubClient.get('/common/accessible-tools');
        const tool = toolsResponse.data.find(t => t.id === id);
        
        if (tool) {
          setToolDetails(tool);
        }

        // Fetch user apps that have access to this tool
        try {
          const userAppsResponse = await pubClient.get(`/common/tools/${id}/user-apps`);
          setUserApps(userAppsResponse.data.data || []);
        } catch (userAppsError) {
          console.warn("Failed to fetch user apps:", userAppsError);
          // Don't fail the whole page if user apps fetch fails
          setUserApps([]);
        }
        
        setLoading(false);
      } catch (err) {
        console.error("Error fetching tool data:", err);
        setError(
          err.response?.data?.errors?.[0] || {
            message: "Failed to fetch tool data. Please try again later.",
            detail: err.message,
          },
        );
        setLoading(false);
      }
    };

    fetchData();
  }, [id]);

  if (loading) {
    return (
      <Box sx={{ p: 3, maxWidth: '1400px' }}>
        <Box sx={{ display: "flex", alignItems: "center", mt: 2 }}>
          <CircularProgress size={24} />
          <Typography variant="h6" sx={{ ml: 2 }}>
            Loading documentation...
          </Typography>
        </Box>
      </Box>
    );
  }

  if (error) {
    return (
      <Box sx={{ p: 3, maxWidth: '1400px' }}>
        <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 3 }}>
          Tool API Documentation
        </Typography>
        <Alert severity="error" variant="outlined">
          <Typography variant="h6" component="div">
            Error
          </Typography>
          <Typography>
            {error.title || error.message || "An unexpected error occurred."}
          </Typography>
          {error.detail && <Typography variant="body2" sx={{mt:1}}>{error.detail}</Typography>}
        </Alert>
      </Box>
    );
  }

  if (!documentationData || documentationData.length === 0) {
    return (
      <Box sx={{ p: 3, maxWidth: '1400px' }}>
        <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 1 }}>
          {toolDetails ? toolDetails.attributes.name : 'Tool'} API Documentation
        </Typography>
        {toolDetails && toolDetails.attributes.description && (
          <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
            {toolDetails.attributes.description}
          </Typography>
        )}
        <Alert severity="info" variant="outlined">
          No API documentation found for this tool, or the tool does not have an OAS specification.
        </Alert>
      </Box>
    );
  }

  return (
    <Box sx={{ p: 3, maxWidth: '1400px' }}>
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 1 }}>
        {toolDetails ? toolDetails.attributes.name : 'Tool'} API Documentation
      </Typography>
      {toolDetails && toolDetails.attributes.description && (
        <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
          {toolDetails.attributes.description}
        </Typography>
      )}

      {/* App Selection Section - Swagger UI style */}
      {userApps.length > 0 && (
        <Box sx={{ 
          display: 'flex', 
          alignItems: 'center', 
          gap: 2, 
          mb: 3, 
          p: 2, 
          backgroundColor: '#fafafa', 
          borderRadius: 1,
          border: '1px solid #e0e0e0'
        }}>
          <Typography variant="body2" sx={{ fontWeight: 500, minWidth: 'fit-content' }}>
            Authorize:
          </Typography>
          <FormControl size="small" sx={{ minWidth: 200 }}>
            <Select
              value={selectedApp}
              onChange={handleAppSelection}
              displayEmpty
              sx={{ 
                backgroundColor: 'white',
                '& .MuiSelect-select': {
                  py: 1,
                  fontSize: '0.875rem'
                }
              }}
            >
              <MenuItem value="">
                <em>Select app to prefill tokens</em>
              </MenuItem>
              {userApps.map((app) => (
                <MenuItem key={app.id} value={app.id}>
                  {app.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          {selectedApp && (
            <Chip 
              label={`Using: ${userApps.find(app => app.id === selectedApp)?.name}`}
              size="small"
              color="success"
              variant="outlined"
              sx={{ 
                backgroundColor: '#e8f5e8',
                borderColor: '#4caf50',
                fontSize: '0.75rem'
              }}
            />
          )}
        </Box>
      )}

      {/* MCP Support Section */}
      <Paper sx={{ p: 3, mb: 3, backgroundColor: '#f8f9fa', border: '1px solid #e9ecef' }} elevation={1}>
        <Typography variant="h5" component="h2" gutterBottom sx={{ mb: 2, display: 'flex', alignItems: 'center' }}>
          🔗 MCP (Model Context Protocol) Support
        </Typography>
        <Typography variant="body1" sx={{ mb: 2 }}>
          This tool is available through the Model Context Protocol (MCP), enabling seamless integration with MCP-compatible clients 
          such as Claude Desktop, Zed, and other AI applications.
        </Typography>
        
        <Box sx={{ mb: 3 }}>
          <Typography variant="h6" gutterBottom sx={{ fontWeight: 'bold' }}>
            MCP Connection Endpoint
          </Typography>
          <CodeBlock index="mcp-endpoint">
            {(() => {
              const config = getConfig();
              const currentHost = window.location.hostname;
              const protocol = window.location.protocol;
              const baseUrl = config.toolDisplayURL || config.proxyURL || `${protocol}//${currentHost}:9090`;
              const toolSlug = toolDetails?.attributes?.name?.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || 'tool-name';
              return `${baseUrl}/tools/${toolSlug}/mcp`;
            })()}
          </CodeBlock>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            This is the default MCP endpoint using StreamableHTTP format. Use this URL with Bearer token authentication in the Authorization header. For older clients that require SSE transport, use <code>/mcp/sse</code> instead.
          </Typography>
        </Box>

        <Box sx={{ mb: 3 }}>
          <Typography variant="h6" gutterBottom sx={{ fontWeight: 'bold' }}>
            Authentication
          </Typography>
          
          <Typography variant="subtitle2" gutterBottom sx={{ fontWeight: 'bold', mb: 1 }}>
            Authorization Header (Recommended for MCP)
          </Typography>
          <Typography variant="body2" sx={{ mb: 2 }}>
            Include your API token in the Authorization header:
          </Typography>
          <CodeBlock index="auth-header">
            Authorization: Bearer {getSelectedApiToken() || 'YOUR_API_TOKEN_HERE'}
          </CodeBlock>
        </Box>

        <Accordion sx={{ mb: 2 }}>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Typography variant="h6" sx={{ fontWeight: 'bold' }}>
              Client Integration Examples
            </Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Box sx={{ mb: 3 }}>
              <Typography variant="subtitle1" gutterBottom sx={{ fontWeight: 'bold' }}>
                Claude Desktop Configuration
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                Add this tool to your Claude Desktop configuration using the mcp-remote library with Bearer authentication:
              </Typography>
              <CodeBlock index="claude-desktop-config" fontSize="0.85rem">
{(() => {
  const config = getConfig();
  const currentHost = window.location.hostname;
  const protocol = window.location.protocol;
  const baseUrl = config.toolDisplayURL || config.proxyURL || `${protocol}//${currentHost}:9090`;
  const toolSlug = toolDetails?.attributes?.name?.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || 'tool-name';
  const toolName = toolDetails?.attributes?.name?.replace(/[^a-zA-Z0-9]/g, '_') || 'tool_name';
  const envVarName = toolName.toUpperCase() + '_API_TOKEN';
  const apiToken = getSelectedApiToken() || 'YOUR_API_TOKEN_HERE';

  return `{
  "mcpServers": {
    "${toolName}": {
      "command": "npx",
      "args": [
        "mcp-remote",
        "${baseUrl}/tools/${toolSlug}/mcp",
        "--header",
        "Authorization: Bearer \${${envVarName}}"
      ],
      "env": {
        "${envVarName}": "${apiToken}"
      }
    }
  }
}`;
})()}
              </CodeBlock>
              <Alert severity="success" sx={{ mt: 2 }}>
                <Typography variant="body2">
                  <strong>Easy Setup:</strong> {getSelectedApiToken() ? 
                    "API token has been automatically filled from your selected app." :
                    "Just replace YOUR_API_TOKEN_HERE with your actual API token."
                  } 
                  Uses secure Bearer token authentication!
                </Typography>
              </Alert>
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography variant="subtitle1" gutterBottom sx={{ fontWeight: 'bold' }}>
                MCP Client Connection (Node.js)
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                Connect to this tool programmatically using the MCP SDK with Bearer token authentication:
              </Typography>
              
              <Typography variant="body2" sx={{ mb: 1, fontWeight: 'bold' }}>
                StreamableHTTP Transport (Default)
              </Typography>
              <CodeBlock index="nodejs-mcp-streamable" fontSize="0.85rem">
{`import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamable.js';

// Create StreamableHTTP transport (default endpoint)
const transport = new StreamableHTTPClientTransport(
  new URL('${(() => {
    const config = getConfig();
    const currentHost = window.location.hostname;
    const protocol = window.location.protocol;
    const baseUrl = config.toolDisplayURL || config.proxyURL || `${protocol}//${currentHost}:9090`;
    const toolSlug = toolDetails?.attributes?.name?.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || 'tool-name';
    return `${baseUrl}/tools/${toolSlug}/mcp`;
  })()}'),
  {
    headers: {
      'Authorization': 'Bearer ${getSelectedApiToken() || 'YOUR_API_TOKEN_HERE'}'
    }
  }
);

const client = new Client({
  name: "my-client",
  version: "1.0.0"
}, {
  capabilities: {
    tools: {}
  }
});

await client.connect(transport);

// List available tools
const result = await client.listTools();
console.log('Available tools:', result.tools);

// Call a tool
const response = await client.callTool({
  name: "operation_name",
  arguments: {
    // your parameters here
  }
});`}
              </CodeBlock>
              
              <Typography variant="body2" sx={{ mb: 1, fontWeight: 'bold', mt: 2 }}>
                SSE Transport (For Older Clients)
              </Typography>
              <CodeBlock index="nodejs-mcp-sse" fontSize="0.85rem">
{`import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { SSEClientTransport } from '@modelcontextprotocol/sdk/client/sse.js';

// Create SSE transport (for older clients that don't support StreamableHTTP)
const transport = new SSEClientTransport(
  new URL('${(() => {
    const config = getConfig();
    const currentHost = window.location.hostname;
    const protocol = window.location.protocol;
    const baseUrl = config.toolDisplayURL || config.proxyURL || `${protocol}//${currentHost}:9090`;
    const toolSlug = toolDetails?.attributes?.name?.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || 'tool-name';
    return `${baseUrl}/tools/${toolSlug}/mcp/sse`;
  })()}'),
  new URL('${(() => {
    const config = getConfig();
    const currentHost = window.location.hostname;
    const protocol = window.location.protocol;
    const baseUrl = config.toolDisplayURL || config.proxyURL || `${protocol}//${currentHost}:9090`;
    const toolSlug = toolDetails?.attributes?.name?.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || 'tool-name';
    return `${baseUrl}/tools/${toolSlug}/mcp/message`;
  })()}'),
  {
    headers: {
      'Authorization': 'Bearer ${getSelectedApiToken() || 'YOUR_API_TOKEN_HERE'}'
    }
  }
);

// Client setup and usage is the same for both transports
const client = new Client({
  name: "my-client", 
  version: "1.0.0"
}, {
  capabilities: { tools: {} }
});

await client.connect(transport);`}
              </CodeBlock>
            </Box>
          </AccordionDetails>
        </Accordion>

        <Alert severity="info" sx={{ mt: 2 }}>
          <Typography variant="body2">
            <strong>Note:</strong> MCP support allows this tool to be used directly within AI applications that support the Model Context Protocol. 
            The tool's operations are automatically converted to MCP-compatible schemas. The default <code>/mcp</code> endpoint uses StreamableHTTP format (modern, single endpoint), 
            while the <code>/mcp/sse</code> endpoint provides SSE transport for older clients. Authentication is handled via Bearer tokens in the Authorization header for secure and standard authentication.
          </Typography>
        </Alert>
      </Paper>

      <Typography variant="h5" component="h2" gutterBottom sx={{ mb: 2, mt: 4 }}>
        📋 API Operations
      </Typography>
      {documentationData.map((operation, index) => (
        <Accordion
          key={operation.operation_id || index}
          expanded={expandedAccordion === `panel${index}`}
          onChange={handleAccordionChange(`panel${index}`)}
          sx={{ mb: 1 }}
        >
          <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            aria-controls={`panel${index}-content`}
            id={`panel${index}-header`}
          >
            <Chip
              label={operation.method.toUpperCase()}
              size="small"
              sx={{
                mr: 2,
                fontWeight: 'bold',
                minWidth: '70px',
                backgroundColor: operation.method.toLowerCase() === 'get' ? 'success.light' :
                                 operation.method.toLowerCase() === 'post' ? 'info.light' :
                                 operation.method.toLowerCase() === 'put' ? 'warning.light' :
                                 operation.method.toLowerCase() === 'delete' ? 'error.light' : 'default',
                color: 'white'
              }}
            />
            <Typography variant="subtitle1" sx={{ mr: 2, flexShrink: 0, fontWeight: 'medium' }}>
              {operation.operation_id || `Operation ${index + 1}`}
            </Typography>
            {/* Remove upstream endpoint path as per requirements */}
          </AccordionSummary>
          <AccordionDetails sx={{ backgroundColor: '#f9f9f9' }}>
            <Box sx={{ py: 2 }}>
              {operation.description && (
                <Typography variant="body1" sx={{mb: 2}}>
                  {operation.description}
                </Typography>
              )}

              {operation.parameters && operation.parameters.length > 0 && (
                <Box sx={{ mb: 3 }}>
                  <Typography variant="h6" gutterBottom component="div" sx={{mb:1}}>
                    Parameters
                  </Typography>
                  <TableContainer component={Paper} elevation={2}>
                    <Table size="small">
                      <TableHead sx={{ backgroundColor: 'grey.200' }}>
                        <TableRow>
                          <TableCell sx={{fontWeight: 'bold'}}>Name</TableCell>
                          <TableCell sx={{fontWeight: 'bold'}}>In</TableCell>
                          <TableCell sx={{fontWeight: 'bold'}}>Required</TableCell>
                          <TableCell sx={{fontWeight: 'bold'}}>Description</TableCell>
                          <TableCell sx={{fontWeight: 'bold'}}>Schema</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {operation.parameters.map((param, pIndex) => (
                          <TableRow key={param.name || pIndex}>
                            <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.875rem' }}>{param.name}</TableCell>
                            <TableCell>{param.in}</TableCell>
                            <TableCell>{param.required ? "Yes" : "No"}</TableCell>
                            <TableCell>{param.description}</TableCell>
                            <TableCell>
                              {param.schema ? (
                                <Box>
                                  <Typography variant="body2" sx={{ mb: 1 }}>
                                    <strong>Type:</strong> {param.schema.type || 'Not specified'}
                                  </Typography>
                                  {param.schema.format && (
                                    <Typography variant="body2" sx={{ mb: 1 }}>
                                      <strong>Format:</strong> {param.schema.format}
                                    </Typography>
                                  )}
                                  {param.schema.enum && (
                                    <Box>
                                      <Typography variant="body2" sx={{ mb: 0.5 }}>
                                        <strong>Allowed values:</strong>
                                      </Typography>
                                      <Box sx={{ pl: 2 }}>
                                        {param.schema.enum.map((val, i) => (
                                          <Typography key={i} variant="body2" component="div" sx={{ fontFamily: 'monospace' }}>
                                            • {String(val)}
                                          </Typography>
                                        ))}
                                      </Box>
                                    </Box>
                                  )}
                                </Box>
                              ) : 'No schema information'}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </TableContainer>
                </Box>
              )}

              {operation.request_body && operation.request_body.schema && (
                 <Box sx={{ mb: 2 }}>
                  <Typography variant="h6" gutterBottom component="div" sx={{mb:1}}>
                    Request Body
                  </Typography>
                  <Paper sx={{ p: 2, backgroundColor: 'grey.50' }} elevation={2}>
                    <Typography variant="body2" sx={{mb: 0.5}}>
                      <strong>Content Type:</strong> {operation.request_body.content_type || "N/A"}
                    </Typography>
                    <Typography variant="body2" sx={{mb: 0.5}}>
                      <strong>Required:</strong> {operation.request_body.required ? "Yes" : "No"}
                    </Typography>
                    {operation.request_body.description && (
                        <Typography variant="body2" sx={{mb: 1}}>
                            <strong>Description:</strong> {operation.request_body.description}
                        </Typography>
                    )}
                    <Typography variant="subtitle2" sx={{mt:1, mb:1, fontWeight: 'bold'}}>Schema Properties:</Typography>
                    {operation.request_body.schema.properties ? (
                      <Box sx={{ ml: 2 }}>
                        {Object.entries(operation.request_body.schema.properties).map(([propName, propSchema]) => (
                          <Box key={propName} sx={{ mb: 2 }}>
                            <Typography variant="body2" sx={{ fontWeight: 'bold', fontFamily: 'monospace' }}>
                              {propName}{operation.request_body.schema.required?.includes(propName) ? ' *' : ''}
                            </Typography>
                            <Box sx={{ ml: 2 }}>
                              <Typography variant="body2">
                                <strong>Type:</strong> {propSchema.type || 'Not specified'}
                              </Typography>
                              {propSchema.description && (
                                <Typography variant="body2">
                                  <strong>Description:</strong> {propSchema.description}
                                </Typography>
                              )}
                              {propSchema.format && (
                                <Typography variant="body2">
                                  <strong>Format:</strong> {propSchema.format}
                                </Typography>
                              )}
                              {propSchema.enum && (
                                <Box sx={{ mt: 0.5 }}>
                                  <Typography variant="body2">
                                    <strong>Allowed values:</strong>
                                  </Typography>
                                  <Box sx={{ pl: 2 }}>
                                    {propSchema.enum.map((val, i) => (
                                      <Typography key={i} variant="body2" component="div" sx={{ fontFamily: 'monospace' }}>
                                        • {String(val)}
                                      </Typography>
                                    ))}
                                  </Box>
                                </Box>
                              )}
                            </Box>
                          </Box>
                        ))}
                      </Box>
                    ) : (
                      <Typography variant="body2" color="text.secondary">No detailed property information available</Typography>
                    )}
                    
                    {/* Include original schema in an expandable section for developers */}
                    <Accordion sx={{ mt: 2, backgroundColor: 'background.paper' }}>
                      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                        <Typography variant="body2">View Raw Schema</Typography>
                      </AccordionSummary>
                      <AccordionDetails>
                        <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontSize: '0.8rem', backgroundColor: 'grey.100', padding: '8px', borderRadius: '4px' }}>
                          {JSON.stringify(operation.request_body.schema, null, 2)}
                        </pre>
                      </AccordionDetails>
                    </Accordion>
                  </Paper>
                </Box>
              )}
               {!operation.request_body?.schema && operation.request_body && (operation.request_body.description || typeof operation.request_body.required === 'boolean') && (
                 <Box sx={{ mb: 2 }}>
                  <Typography variant="h6" gutterBottom component="div" sx={{mb:1}}>
                    Request Body
                  </Typography>
                  <Paper sx={{ p: 2, backgroundColor: 'grey.50' }} elevation={2}>
                    {operation.request_body.description && (
                        <Typography variant="body2" sx={{mb: 1}}>
                            <strong>Description:</strong> {operation.request_body.description}
                        </Typography>
                    )}
                     <Typography variant="body2" sx={{mb: 0.5}}>
                      <strong>Required:</strong> {operation.request_body.required ? "Yes" : "No"}
                    </Typography>
                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>No schema defined for this request body.</Typography>
                  </Paper>
                </Box>
              )}
              {/* Example curl command */}
              <Box sx={{ mt: 3 }}>
                <Typography variant="h6" gutterBottom component="div">
                  Example Request
                </Typography>
                <Paper sx={{ p: 2, backgroundColor: '#292929', position: 'relative' }} elevation={3}>
                  <Typography variant="body2" sx={{ fontFamily: 'monospace', color: 'white', whiteSpace: 'pre-wrap', wordBreak: 'break-word', pr: '40px' }}>
                    {generateCurlExample(operation, toolDetails, getSelectedApiToken())}
                  </Typography>
                  <IconButton 
                    onClick={() => handleCopyToClipboard(generateCurlExample(operation, toolDetails, getSelectedApiToken()), index)} 
                    sx={{ 
                      position: 'absolute', 
                      top: '8px', 
                      right: '8px', 
                      color: 'white',
                      backgroundColor: 'rgba(255,255,255,0.1)',
                      '&:hover': { backgroundColor: 'rgba(255,255,255,0.2)' }
                    }}
                    size="small"
                    title="Copy to clipboard"
                  >
                    {copiedIndex === index ? <CheckIcon fontSize="small" /> : <ContentCopyIcon fontSize="small" />}
                  </IconButton>
                </Paper>
              </Box>
            </Box>
          </AccordionDetails>
        </Accordion>
      ))}
    </Box>
  );
};

export default ToolDocumentationPage;
