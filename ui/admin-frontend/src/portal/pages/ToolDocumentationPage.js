import React, { useState, useEffect } from "react";
import { useParams } from "react-router-dom";
import {
  Container,
  Typography,
  CircularProgress,
  Box,
  Alert,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import pubClient from "../../admin/utils/pubClient";

const ToolDocumentationPage = () => {
  const { id } = useParams();
  const [documentationData, setDocumentationData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [expandedAccordion, setExpandedAccordion] = useState(null);

  const handleAccordionChange = (panel) => (event, isExpanded) => {
    setExpandedAccordion(isExpanded ? panel : false);
  };

  useEffect(() => {
    const fetchDocumentation = async () => {
      if (!id) {
        setLoading(false);
        setError({ message: "Tool ID is missing." });
        return;
      }
      setLoading(true);
      setError(null);
      setDocumentationData(null);
      try {
        const response = await pubClient.get(`/common/tools/${id}/docs`);
        setDocumentationData(response.data);
        setLoading(false);
      } catch (err) {
        console.error("Error fetching tool documentation:", err);
        setError(
          err.response?.data?.errors?.[0] || {
            message: "Failed to fetch tool documentation. Please try again later.",
            detail: err.message,
          },
        );
        setLoading(false);
      }
    };

    fetchDocumentation();
  }, [id]);

  if (loading) {
    return (
      <Box
        sx={{
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
          height: "80vh",
        }}
      >
        <CircularProgress />
        <Typography variant="h6" sx={{ ml: 2 }}>
          Loading documentation...
        </Typography>
      </Box>
    );
  }

  if (error) {
    return (
      <Container sx={{ mt: 4 }}>
        <Alert severity="error" variant="outlined">
          <Typography variant="h6" component="div">
            Error
          </Typography>
          <Typography>
            {error.title || error.message || "An unexpected error occurred."}
          </Typography>
          {error.detail && <Typography variant="body2" sx={{mt:1}}>{error.detail}</Typography>}
        </Alert>
      </Container>
    );
  }

  if (!documentationData || documentationData.length === 0) {
    return (
      <Container sx={{ mt: 4 }}>
        <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 3 }}>
          Tool API Documentation
        </Typography>
        <Alert severity="info" variant="outlined">
          No API documentation found for this tool, or the tool does not have an OAS specification.
        </Alert>
      </Container>
    );
  }

  return (
    <Container sx={{ mt: 4, mb: 4 }}>
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 3 }}>
        Tool API Documentation
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
            <Typography sx={{ color: 'text.secondary', wordBreak: 'break-all' }}>{operation.path}</Typography>
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
                              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontSize: '0.8rem', backgroundColor: 'grey.100', padding: '4px 8px', borderRadius: '4px' }}>
                                {JSON.stringify(param.schema, null, 2)}
                              </pre>
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
                    <Typography variant="subtitle2" sx={{mt:1, mb:0.5, fontWeight: 'bold'}}>Schema:</Typography>
                    <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontSize: '0.8rem', backgroundColor: 'grey.100', padding: '8px', borderRadius: '4px' }}>
                      {JSON.stringify(operation.request_body.schema, null, 2)}
                    </pre>
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
            </Box>
          </AccordionDetails>
        </Accordion>
      ))}
    </Container>
  );
};

export default ToolDocumentationPage;
