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
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import CheckIcon from "@mui/icons-material/Check";
import pubClient from '../../admin/utils/pubClient';
import { getConfig } from '../../config';

// Helper function to generate example curl commands
const generateCurlExample = (operation, toolDetails) => {
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
  
  // Add authorization placeholder
  parts.push('  -H "Authorization: Bearer YOUR_API_KEY"');
  
  // Build the URL - correct format for the proxy endpoint with proper gateway URL
  const config = getConfig();
  const currentHost = window.location.hostname;
  const protocol = window.location.protocol; // Get the current protocol (http: or https:)
  // Use proxyURL from config if available, otherwise use protocol + hostname:9090 (default gateway port)
  const baseUrl = config.proxyURL || `${protocol}//${currentHost}:9090`;
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
      <Box sx={{ p: 3 }}>
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
      <Box sx={{ p: 3 }}>
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
      <Box sx={{ p: 3 }}>
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
    <Box sx={{ p: 3 }}>
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 1 }}>
        {toolDetails ? toolDetails.attributes.name : 'Tool'} API Documentation
      </Typography>
      {toolDetails && toolDetails.attributes.description && (
        <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
          {toolDetails.attributes.description}
        </Typography>
      )}
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
                <Typography variant="body1" paragraph sx={{mb: 2}}>
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
                    {generateCurlExample(operation, toolDetails)}
                  </Typography>
                  <IconButton 
                    onClick={() => handleCopyToClipboard(generateCurlExample(operation, toolDetails), index)} 
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
