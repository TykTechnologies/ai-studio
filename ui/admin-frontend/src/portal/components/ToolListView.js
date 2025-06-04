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
import DetailModal from "./DetailModal";
import { getVendorName, getVendorLogo } from "../../admin/utils/vendorLogos";
import { PrimaryButton } from "../../admin/styles/sharedStyles";

const defaultLogo = "/generic-tool-logo.png";

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
          `/common/catalogues/${catalogueId}/tools`,
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
              <CardMedia
                component="img"
                sx={{ height: 140, objectFit: "contain" }}
                image={tool.attributes.logoURL || defaultLogo}
                alt={`${tool.attributes.name} logo`}
              />
              <CardContent sx={{ flexGrow: 1 }}>
                <Typography gutterBottom variant="h6" component="div">
                  {tool.attributes.name}
                </Typography>
                <Typography variant="body2" color="text.defaultSubdued">
                  {tool.attributes.short_description}
                </Typography>
                <Box sx={{ display: "flex", alignItems: "center", mt: 1 }}>
                  <img
                    src={getVendorLogo(tool.attributes.vendor)}
                    alt={getVendorName(tool.attributes.vendor)}
                    style={{
                      width: 24,
                      height: 24,
                      marginRight: 8,
                      objectFit: "contain",
                    }}
                    onError={(e) => {
                      e.target.onerror = null;
                      e.target.src =
                        process.env.PUBLIC_URL + "/images/placeholder-logo.png";
                    }}
                  />
                  <Typography variant="body2" color="text.defaultSubdued">
                    {getVendorName(tool.attributes.vendor)}
                  </Typography>
                </Box>
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
          <Typography variant="body1" sx={{ mt: 2 }}>
            {selectedTool.attributes.long_description}
          </Typography>
          <Box sx={{ display: "flex", alignItems: "center", mt: 2 }}>
            <Typography variant="subtitle1">Vendor:</Typography>
            <img
              src={getVendorLogo(selectedTool.attributes.vendor)}
              alt={getVendorName(selectedTool.attributes.vendor)}
              style={{
                width: 24,
                height: 24,
                marginLeft: 8,
                marginRight: 8,
                objectFit: "contain",
              }}
            />
            <Typography>
              {getVendorName(selectedTool.attributes.vendor)}
            </Typography>
          </Box>
        </DetailModal>
      )}
    </Container>
  );
};

export default ToolListView;
