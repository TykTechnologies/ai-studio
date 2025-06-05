import React, { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import {
  Box,
  Grid,
  Card,
  CardContent,
  Typography,
  Button,
  Chip,
  CircularProgress,
  Container,
  Divider,
} from "@mui/material";
import pubClient from "../../admin/utils/pubClient";
import DetailModal from "./DetailModal";
import { PrimaryButton } from "../../admin/styles/sharedStyles";

// Function to get color based on tool type
const getToolTypeColor = (type) => {
  if (!type) return { bg: 'primary.light', color: 'primary.contrastText' };
  
  const typeLC = type.toLowerCase();
  
  switch(typeLC) {
    case 'rest':
      return { bg: '#3f51b5', color: '#fff' };
    case 'graphql':
      return { bg: '#e535ab', color: '#fff' };
    case 'grpc':
      return { bg: '#244c5a', color: '#fff' };
    case 'websocket':
      return { bg: '#4caf50', color: '#fff' };
    case 'function':
    case 'functions':
      return { bg: '#ff9800', color: '#fff' };
    case 'ai':
    case 'ml':
    case 'llm':
      return { bg: '#9c27b0', color: '#fff' };
    default:
      return { bg: '#607d8b', color: '#fff' };
  }
};

const ToolListView = () => {
  const [tools, setTools] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const { catalogueId } = useParams();
  const navigate = useNavigate();
  const [openModal, setOpenModal] = useState(false);
  const [selectedTool, setSelectedTool] = useState(null);

  useEffect(() => {
    const fetchTools = async () => {
      try {
        const response = await pubClient.get(
          `/common/tool-catalogues/${catalogueId}/tools`,
        );
        setTools(response.data);
        setLoading(false);
      } catch (err) {
        console.error("Error fetching Tools:", err);
        setError("Failed to fetch Tools. Please try again later.");
        setLoading(false);
      }
    };

    fetchTools();
  }, [catalogueId]);

  const handleBuildApp = (toolId) => {
    navigate(`/portal/app/new?tool=${toolId}`);
  };

  const handleOpenModal = (tool) => {
    setSelectedTool(tool);
    setOpenModal(true);
  };

  const handleCloseModal = () => {
    setOpenModal(false);
  };

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Typography color="error" sx={{ textAlign: "center", mt: 4 }}>
        {error}
      </Typography>
    );
  }

  return (
    <Container
      maxWidth={false}
      sx={{
        px: 3,
        py: 3,
        boxSizing: "border-box",
        width: "100%",
      }}
    >
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 4 }}>
        Available Tools
      </Typography>
      <Grid container spacing={3}>
        {tools.map((tool) => (
          <Grid item xs={12} sm={6} md={4} key={tool.id}>
            <Card
              sx={{ height: "100%", display: "flex", flexDirection: "column" }}
            >
              <CardContent sx={{ flexGrow: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                  <Typography variant="h6" component="div">
                    {tool.attributes.name}
                  </Typography>
                  {tool.attributes.tool_type && (
                    <Chip
                      label={tool.attributes.tool_type}
                      size="small"
                      sx={{ 
                        bgcolor: getToolTypeColor(tool.attributes.tool_type).bg,
                        color: getToolTypeColor(tool.attributes.tool_type).color,
                        fontWeight: 'bold',
                        fontSize: '0.7rem',
                      }}
                    />
                  )}
                </Box>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  {tool.attributes.short_description}
                </Typography>
                <Typography variant="body2" color="text.defaultSubdued" sx={{ mb: 2 }}>
                  {tool.attributes.description}
                </Typography>
                
                {tool.attributes.operations && tool.attributes.operations.length > 0 && (
                  <>
                    <Divider sx={{ my: 1 }} />
                    <Typography variant="body2" fontWeight="bold" sx={{ mt: 1, mb: 1 }}>
                      Operations:
                    </Typography>
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                      {tool.attributes.operations.map((op, index) => (
                        <Chip 
                          key={index} 
                          label={op.name || op} 
                          size="small" 
                          color="default" 
                          variant="outlined"
                          sx={{ 
                            mb: 0.5, 
                            bgcolor: 'rgba(0,0,0,0.05)',
                            borderColor: 'rgba(0,0,0,0.2)',
                            color: 'text.secondary' 
                          }}
                        />
                      ))}
                    </Box>
                  </>
                )}
              </CardContent>
              <Box
                sx={{ p: 2, display: "flex", justifyContent: "space-between" }}
              >
                <Button variant="outlined" onClick={() => handleOpenModal(tool)}>
                  More
                </Button>
                <PrimaryButton
                  variant="contained"
                  onClick={() => handleBuildApp(tool.id)}
                >
                  Build App
                </PrimaryButton>
              </Box>
            </Card>
          </Grid>
        ))}
      </Grid>
      {selectedTool && (
        <DetailModal
          open={openModal}
          handleClose={handleCloseModal}
          title={selectedTool.attributes.name}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', mt: 2, mb: 1 }}>
            <Typography variant="subtitle1" fontWeight="bold">
              Description
            </Typography>
            {selectedTool.attributes.tool_type && (
              <Chip
                label={selectedTool.attributes.tool_type}
                size="small"
                sx={{ 
                  ml: 2,
                  bgcolor: getToolTypeColor(selectedTool.attributes.tool_type).bg,
                  color: getToolTypeColor(selectedTool.attributes.tool_type).color,
                  fontWeight: 'bold',
                  fontSize: '0.7rem',
                }}
              />
            )}
          </Box>
          <Typography variant="body1" sx={{ mt: 1, mb: 2 }}>
            {selectedTool.attributes.description || selectedTool.attributes.long_description}
          </Typography>
          
          <Typography variant="subtitle1" fontWeight="bold">
            Details
          </Typography>
          <Typography variant="body1" sx={{ mt: 1, mb: 2 }}>
            {selectedTool.attributes.long_description}
          </Typography>

          <Box sx={{ mt: 2, mb: 2 }}>
            <Button
              component={Link}
              to={`/common/tools/${selectedTool.id}/docs`}
              variant="contained"
              color="info" // Using 'info' or another appropriate color
              target="_blank" // Optional: Open in new tab
              rel="noopener noreferrer" // Optional: For security when using target="_blank"
            >
              View Full API Documentation
            </Button>
          </Box>
          
          {selectedTool.attributes.operations && selectedTool.attributes.operations.length > 0 && (
            <>
              <Typography variant="subtitle1" fontWeight="bold">
                Operations
              </Typography>
              <Box sx={{ mt: 1, display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                {selectedTool.attributes.operations.map((op, index) => (
                  <Box key={index} sx={{ 
                    mb: 1, 
                    p: 1.5, 
                    bgcolor: 'rgba(0,0,0,0.05)', 
                    border: '1px solid rgba(0,0,0,0.1)',
                    borderRadius: 1,
                    width: 'calc(50% - 8px)'
                  }}>
                    <Typography variant="body2" fontWeight="bold" color="text.primary">
                      {op.name || op}
                    </Typography>
                    {op.description && (
                      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                        {op.description}
                      </Typography>
                    )}
                  </Box>
                ))}
              </Box>
            </>
          )}
        </DetailModal>
      )}
    </Container>
  );
};

export default ToolListView;
