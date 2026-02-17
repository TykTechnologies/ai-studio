import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  Box,
  Grid,
  Card,
  CardContent,
  Typography,
  CircularProgress,
  Container,
  Chip,
} from "@mui/material";
import pubClient from "../../admin/utils/pubClient";
import { PrimaryButton } from "../../admin/styles/sharedStyles";

/**
 * PluginResourceListView is the Portal's default browse view for plugin resource instances.
 * It shows a card grid of available instances for a given resource type, with a
 * "Build App" button that navigates to AppBuilder with the resource pre-selected.
 *
 * Route: /portal/resources/:pluginId/:slug
 */
const PluginResourceListView = () => {
  const { pluginId, slug } = useParams();
  const navigate = useNavigate();
  const [instances, setInstances] = useState([]);
  const [resourceTypeName, setResourceTypeName] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchData = async () => {
      try {
        // Fetch the accessible plugin resources (includes type metadata + instances)
        const response = await pubClient.get(
          "/common/accessible-plugin-resources",
        );
        const types = response.data?.data || [];
        const matchingType = types.find(
          (t) =>
            String(t.plugin_id) === String(pluginId) && t.slug === slug,
        );
        if (matchingType) {
          setResourceTypeName(matchingType.name);
          setInstances(matchingType.instances || []);
        }
      } catch (err) {
        console.error("Error fetching plugin resources:", err);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, [pluginId, slug]);

  if (loading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="200px"
      >
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Container
      maxWidth={false}
      sx={{ px: 3, py: 3, boxSizing: "border-box", width: "100%" }}
    >
      <Typography variant="h4" component="h1" gutterBottom>
        {resourceTypeName || slug}
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
        Browse available {resourceTypeName || "resources"} and add them to
        your apps.
      </Typography>

      {instances.length === 0 ? (
        <Typography variant="body1" color="text.secondary">
          No {resourceTypeName || "resources"} are currently available.
        </Typography>
      ) : (
        <Grid container spacing={3}>
          {instances.map((inst) => (
            <Grid item xs={12} sm={6} md={4} key={inst.id}>
              <Card
                sx={{
                  height: "100%",
                  display: "flex",
                  flexDirection: "column",
                }}
              >
                <CardContent sx={{ flexGrow: 1 }}>
                  <Typography variant="h6" gutterBottom>
                    {inst.name}
                  </Typography>
                  {inst.description && (
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{ mb: 2 }}
                    >
                      {inst.description}
                    </Typography>
                  )}
                  {inst.privacy_score > 0 && (
                    <Chip
                      label={`Privacy: ${inst.privacy_score}`}
                      size="small"
                      color="primary"
                      variant="outlined"
                      sx={{ mb: 1 }}
                    />
                  )}
                </CardContent>
                <Box sx={{ p: 2, pt: 0 }}>
                  <PrimaryButton
                    size="small"
                    onClick={() =>
                      navigate(
                        `/portal/app/new?plugin_resource=${pluginId}:${slug}:${inst.id}`,
                      )
                    }
                  >
                    Build App
                  </PrimaryButton>
                </Box>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}
    </Container>
  );
};

export default PluginResourceListView;
