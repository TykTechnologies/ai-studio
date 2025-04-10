import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Container,
  Typography,
  Box,
  Grid,
  CircularProgress,
  TextField,
  Button,
  Paper,
} from "@mui/material";
import {
  StyledPaper,
  PrimaryButton,
} from "../../admin/styles/sharedStyles";
import mcpService from "../../admin/services/mcpService";

const MCPServerCreation = () => {
  const navigate = useNavigate();
  const [formData, setFormData] = useState({
    name: "",
    description: "",
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const handleFormChange = (e) => {
    const { name, value } = e.target;
    setFormData({
      ...formData,
      [name]: value,
    });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const result = await mcpService.createServer(formData);
      const newServerId = result.data.id;
      // Navigate to the detail view of the newly created server
      navigate(`/portal/mcp-servers/${newServerId}`);
    } catch (err) {
      console.error("Error creating MCP server:", err);
      setError(
        "Failed to create MCP server. Please check your inputs and try again."
      );
      setLoading(false);
    }
  };

  const handleCancel = () => {
    navigate("/portal/mcp-servers");
  };

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
        Create MCP Server
      </Typography>

      <StyledPaper sx={{ p: 4, maxWidth: "800px", mx: "auto" }}>
        <form onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <Typography variant="body1" paragraph>
                MCP Servers enable AI agents to interact with your tools through a standardized protocol.
                Create a new server by providing the information below.
              </Typography>
            </Grid>

            <Grid item xs={12}>
              <Typography variant="subtitle2" gutterBottom>
                Server Name*
              </Typography>
              <TextField
                fullWidth
                name="name"
                value={formData.name}
                onChange={handleFormChange}
                placeholder="Enter a name for this MCP server"
                required
              />
            </Grid>

            <Grid item xs={12}>
              <Typography variant="subtitle2" gutterBottom>
                Description
              </Typography>
              <TextField
                fullWidth
                name="description"
                value={formData.description}
                onChange={handleFormChange}
                placeholder="Enter a description (optional)"
                multiline
                rows={4}
              />
            </Grid>

            {error && (
              <Grid item xs={12}>
                <Typography color="error">{error}</Typography>
              </Grid>
            )}

            <Grid item xs={12}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "flex-end",
                  mt: 2,
                  gap: 2,
                }}
              >
                <Button variant="outlined" onClick={handleCancel}>
                  Cancel
                </Button>
                <PrimaryButton
                  type="submit"
                  disabled={loading || !formData.name}
                >
                  {loading ? (
                    <CircularProgress size={24} sx={{ color: "white" }} />
                  ) : (
                    "Create Server"
                  )}
                </PrimaryButton>
              </Box>
            </Grid>
          </Grid>
        </form>
      </StyledPaper>

      <StyledPaper sx={{ p: 4, mt: 4, maxWidth: "800px", mx: "auto" }}>
        <Typography variant="h6" gutterBottom>
          About MCP Servers
        </Typography>
        <Typography variant="body1" paragraph>
          The Multi-Agent Communication Protocol (MCP) enables AI agents to interact with your tools through a standardized protocol. 
          Each MCP server can host multiple tools, allowing AI agents to discover and use them.
        </Typography>
        
        <Typography variant="subtitle1" gutterBottom>
          After creating your server:
        </Typography>
        <ul style={{ paddingLeft: "1.5rem" }}>
          <li>
            <Typography variant="body1" sx={{ mb: 1 }}>
              Add tools to make them available through the MCP server
            </Typography>
          </li>
          <li>
            <Typography variant="body1" sx={{ mb: 1 }}>
              Start the server to enable client connections
            </Typography>
          </li>
          <li>
            <Typography variant="body1" sx={{ mb: 1 }}>
              Monitor active sessions and server logs
            </Typography>
          </li>
          <li>
            <Typography variant="body1">
              Use the server endpoint in your client applications to establish MCP sessions
            </Typography>
          </li>
        </ul>
      </StyledPaper>
    </Container>
  );
};

export default MCPServerCreation;
