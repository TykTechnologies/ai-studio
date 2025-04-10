import React, { useState, useEffect } from "react";
import {
  Typography,
  Container,
  Paper,
  CircularProgress,
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import AddIcon from "@mui/icons-material/Add";
import ServerIcon from "@mui/icons-material/Storage";
import Grid from "@mui/material/Grid";
import pubClient from "../../admin/utils/pubClient";
import useSystemFeatures from "../../admin/hooks/useSystemFeatures";
import { PrimaryButton } from "../../admin/styles/sharedStyles";

const PortalDashboard = () => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showPortal, setShowPortal] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      setLoading(true);
      const response = await pubClient.get("/common/me");
      setShowPortal(response.data.attributes.ui_options?.show_portal ?? true);
      setLoading(false);
    } catch (err) {
      console.error("Error fetching data:", err);
      setError("Failed to fetch data. Please try again later.");
      setLoading(false);
    }
  };

  const handleCreateApp = () => {
    navigate("/portal/app/new");
  };

  if (loading || featuresLoading) {
    return (
      <Container sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Container>
    );
  }

  if (error) {
    return (
      <Container>
        <Typography color="error" sx={{ textAlign: "center", mt: 4 }}>
          {error}
        </Typography>
      </Container>
    );
  }

  const showPortalFeatures =
    features.feature_portal || features.feature_gateway;

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
        AI Developer Portal
      </Typography>

      {showPortalFeatures && showPortal && (
        <Grid container spacing={4}>
          <Grid item xs={12} md={6}>
            <Paper sx={{ p: 4, textAlign: "center", height: "100%" }}>
              <Typography variant="h6" gutterBottom>
                Create and Manage AI Applications
              </Typography>
              <Typography variant="body1" paragraph>
                Build custom AI applications with our powerful tools and services.
                Apps provide access to LLMs and Data sources via the AI Gateway for
                your code.
              </Typography>
              <PrimaryButton
                variant="contained"
                color="primary"
                startIcon={<AddIcon />}
                onClick={handleCreateApp}
              >
                Create a new App
              </PrimaryButton>
            </Paper>
          </Grid>
          
          <Grid item xs={12} md={6}>
            <Paper sx={{ p: 4, textAlign: "center", height: "100%" }}>
              <Typography variant="h6" gutterBottom>
                MCP Servers
              </Typography>
              <Typography variant="body1" paragraph>
                Deploy and manage MCP servers to enable AI agents to interact with your tools
                through a standardized protocol. Connect your tools to AI systems seamlessly.
              </Typography>
              <PrimaryButton
                variant="contained"
                color="primary"
                startIcon={<ServerIcon />}
                onClick={() => navigate("/portal/mcp-servers")}
              >
                Manage MCP Servers
              </PrimaryButton>
            </Paper>
          </Grid>
        </Grid>
      )}
    </Container>
  );
};

export default PortalDashboard;
