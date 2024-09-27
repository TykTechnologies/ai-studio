import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  Box,
  Grid,
  Card,
  CardContent,
  CardMedia,
  Typography,
  Button,
  CircularProgress,
  Container,
} from "@mui/material";
import pubClient from "../../admin/utils/pubClient";

const defaultLogo = "/generic-llm-logo.png"; // Replace with actual path to default logo

const LLMListView = () => {
  const [llms, setLLMs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const { catalogueId } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    const fetchLLMs = async () => {
      try {
        const response = await pubClient.get(
          `/common/catalogues/${catalogueId}/llms`,
        );
        setLLMs(response.data);
        setLoading(false);
      } catch (err) {
        console.error("Error fetching LLMs:", err);
        setError("Failed to fetch LLMs. Please try again later.");
        setLoading(false);
      }
    };

    fetchLLMs();
  }, [catalogueId]);

  const handleBuildApp = (llmId) => {
    navigate(`/portal/app/new?llm=${llmId}`);
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
    <Container maxWidth="lg">
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 4 }}>
        Available LLMs
      </Typography>
      <Grid container spacing={3}>
        {llms.map((llm) => (
          <Grid item xs={12} sm={6} md={4} key={llm.id}>
            <Card
              sx={{ height: "100%", display: "flex", flexDirection: "column" }}
            >
              <CardMedia
                component="img"
                sx={{ height: 140, objectFit: "contain" }}
                image={llm.attributes.logoURL || defaultLogo}
                alt={`${llm.attributes.name} logo`}
              />
              <CardContent sx={{ flexGrow: 1 }}>
                <Typography gutterBottom variant="h5" component="div">
                  {llm.attributes.name}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {llm.attributes.short_description}
                </Typography>
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ mt: 1 }}
                >
                  Vendor: {llm.attributes.vendor}
                </Typography>
              </CardContent>
              <Box sx={{ p: 2 }}>
                <Button
                  variant="contained"
                  fullWidth
                  onClick={() => handleBuildApp(llm.id)}
                >
                  Build an App with this LLM
                </Button>
              </Box>
            </Card>
          </Grid>
        ))}
      </Grid>
    </Container>
  );
};

export default LLMListView;
