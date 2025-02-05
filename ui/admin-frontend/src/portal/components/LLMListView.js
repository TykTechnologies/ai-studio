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

const defaultLogo = "/generic-llm-logo.png";

const LLMListView = () => {
  const [llms, setLLMs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const { catalogueId } = useParams();
  const navigate = useNavigate();
  const [openModal, setOpenModal] = useState(false);
  const [selectedLLM, setSelectedLLM] = useState(null);

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

  const handleOpenModal = (llm) => {
    setSelectedLLM(llm);
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
                <Typography gutterBottom variant="h6" component="div">
                  {llm.attributes.name}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {llm.attributes.short_description}
                </Typography>
                <Box sx={{ display: "flex", alignItems: "center", mt: 1 }}>
                  <img
                    src={getVendorLogo(llm.attributes.vendor)}
                    alt={getVendorName(llm.attributes.vendor)}
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
                  <Typography variant="body2" color="text.secondary">
                    {getVendorName(llm.attributes.vendor)}
                  </Typography>
                </Box>
              </CardContent>
              <Box
                sx={{ p: 2, display: "flex", justifyContent: "space-between" }}
              >
                <Button variant="outlined" onClick={() => handleOpenModal(llm)}>
                  More
                </Button>
                <Button
                  variant="contained"
                  onClick={() => handleBuildApp(llm.id)}
                >
                  Build App
                </Button>
              </Box>
            </Card>
          </Grid>
        ))}
      </Grid>
      {selectedLLM && (
        <DetailModal
          open={openModal}
          handleClose={handleCloseModal}
          title={selectedLLM.attributes.name}
        >
          <Typography variant="body1" sx={{ mt: 2 }}>
            {selectedLLM.attributes.long_description}
          </Typography>
          <Box sx={{ display: "flex", alignItems: "center", mt: 2 }}>
            <Typography variant="subtitle1">Vendor:</Typography>
            <img
              src={getVendorLogo(selectedLLM.attributes.vendor)}
              alt={getVendorName(selectedLLM.attributes.vendor)}
              style={{
                width: 24,
                height: 24,
                marginLeft: 8,
                marginRight: 8,
                objectFit: "contain",
              }}
            />
            <Typography>
              {getVendorName(selectedLLM.attributes.vendor)}
            </Typography>
          </Box>
        </DetailModal>
      )}
    </Container>
  );
};

export default LLMListView;
