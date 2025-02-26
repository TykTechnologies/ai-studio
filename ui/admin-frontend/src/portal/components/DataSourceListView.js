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
import {
  getVectorStoreName,
  getVectorStoreLogo,
  getEmbedderName,
  getEmbedderLogo,
  fetchVendors,
} from "../../admin/utils/vendorUtils";
import DetailModal from "./DetailModal";
import { PrimaryButton } from "../../admin/styles/sharedStyles";

const defaultIcon = "/generic-datasource-icon.png";

const DataSourceListView = () => {
  const [dataSources, setDataSources] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const { catalogueId } = useParams();
  const [openModal, setOpenModal] = useState(false);
  const [selectedDataSource, setSelectedDataSource] = useState(null);
  const [vendors, setVendors] = useState({ embedders: [], vectorStores: [] });
  const navigate = useNavigate();

  useEffect(() => {
    const initializePage = async () => {
      const fetchedVendors = await fetchVendors();
      setVendors(fetchedVendors);
      fetchDataSources();
    };
    initializePage();
  }, [catalogueId]);

  const fetchDataSources = async () => {
    try {
      const response = await pubClient.get(
        `/common/data-catalogues/${catalogueId}/datasources`,
      );
      setDataSources(response.data);
      setLoading(false);
    } catch (err) {
      console.error("Error fetching data sources:", err);
      setError("Failed to fetch data sources. Please try again later.");
      setLoading(false);
    }
  };

  const handleOpenModal = (dataSource) => {
    setSelectedDataSource(dataSource);
    setOpenModal(true);
  };

  const handleCloseModal = () => {
    setOpenModal(false);
    setSelectedDataSource(null);
  };

  const handleUseDataSource = (dataSourceId) => {
    navigate(`/portal/app/new?datasource=${dataSourceId}`);
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
        Available Data Sources
      </Typography>
      <Grid container spacing={3}>
        {dataSources.map((dataSource) => (
          <Grid item xs={12} sm={6} md={4} key={dataSource.id}>
            <Card
              sx={{ height: "100%", display: "flex", flexDirection: "column" }}
            >
              <CardMedia
                component="img"
                sx={{ height: 140, objectFit: "contain" }}
                image={dataSource.attributes.icon || defaultIcon}
                alt={`${dataSource.attributes.name} icon`}
                onError={(e) => {
                  e.target.onerror = null;
                  e.target.src = defaultIcon;
                }}
              />
              <CardContent sx={{ flexGrow: 1 }}>
                <Typography gutterBottom variant="h6" component="div">
                  {dataSource.attributes.name}
                </Typography>
                <Typography variant="body2" color="text.defaultSubdued">
                  {dataSource.attributes.short_description}
                </Typography>
              </CardContent>
              <Box
                sx={{ p: 2, display: "flex", justifyContent: "space-between" }}
              >
                <Button
                  variant="outlined"
                  onClick={() => handleOpenModal(dataSource)}
                >
                  More
                </Button>
                <PrimaryButton
                  variant="contained"
                  onClick={() => handleUseDataSource(dataSource.id)}
                >
                  Get Access
                </PrimaryButton>
              </Box>
            </Card>
          </Grid>
        ))}
      </Grid>
      {selectedDataSource && (
        <DetailModal
          open={openModal}
          handleClose={handleCloseModal}
          title={selectedDataSource.attributes.name}
        >
          <Box sx={{ mt: 2 }}>
            <Typography variant="subtitle1">Database Type:</Typography>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <img
                src={getVectorStoreLogo(
                  selectedDataSource.attributes.db_source_type,
                )}
                alt={getVectorStoreName(
                  selectedDataSource.attributes.db_source_type,
                )}
                style={{
                  width: 24,
                  height: 24,
                  marginRight: 8,
                  objectFit: "contain",
                }}
              />
              <Typography>
                {getVectorStoreName(
                  selectedDataSource.attributes.db_source_type,
                )}
              </Typography>
            </Box>
          </Box>
          <Box sx={{ mt: 2 }}>
            <Typography variant="subtitle1">Embed Vendor:</Typography>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <img
                src={getEmbedderLogo(
                  selectedDataSource.attributes.embed_vendor,
                )}
                alt={getEmbedderName(
                  selectedDataSource.attributes.embed_vendor,
                )}
                style={{
                  width: 24,
                  height: 24,
                  marginRight: 8,
                  objectFit: "contain",
                }}
              />
              <Typography>
                {getEmbedderName(selectedDataSource.attributes.embed_vendor)}
              </Typography>
            </Box>
          </Box>
          <Typography variant="subtitle1" sx={{ mt: 2 }}>
            Embedding Model: {selectedDataSource.attributes.embed_model}
          </Typography>
          <Typography variant="body1" sx={{ mt: 2 }}>
            {selectedDataSource.attributes.long_description}
          </Typography>
        </DetailModal>
      )}
    </Container>
  );
};

export default DataSourceListView;
