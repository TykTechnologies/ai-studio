import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  IconButton,
  Tooltip,
  Link,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../../utils/vendorLogos";

const LLMDetails = () => {
  const [llm, setLLM] = useState(null);
  const [loading, setLoading] = useState(true);
  const [copySuccess, setCopySuccess] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchLLMDetails();
  }, [id]);

  const fetchLLMDetails = async () => {
    try {
      const response = await apiClient.get(`/llms/${id}`);
      setLLM(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching LLM details", error);
      setLoading(false);
    }
  };

  const copyToClipboard = (text, field) => {
    navigator.clipboard.writeText(text).then(
      () => {
        setCopySuccess(`${field} copied!`);
        setTimeout(() => setCopySuccess(""), 2000);
      },
      (err) => {
        console.error("Could not copy text: ", err);
      },
    );
  };

  if (loading) return <CircularProgress />;
  if (!llm) return <Typography>LLM not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">LLM Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/llms")}
          color="white"
        >
          Back to LLMs
        </Button>
      </TitleBox>
      <ContentBox>
        <Grid container spacing={2} mb={4}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Short Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm.attributes.short_description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Long Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm.attributes.long_description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Vendor:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <img
                src={getVendorLogo(llm.attributes.vendor)}
                alt={getVendorName(llm.attributes.vendor)}
                style={{
                  width: 24,
                  height: 24,
                  marginRight: 8,
                  objectFit: "contain",
                }}
              />
              <FieldValue>{getVendorName(llm.attributes.vendor)}</FieldValue>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Privacy Score:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm.attributes.privacy_score}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>API Endpoint:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>{llm.attributes.api_endpoint}</FieldValue>
              <Tooltip title="Copy to clipboard" placement="top">
                <IconButton
                  onClick={() =>
                    copyToClipboard(llm.attributes.api_endpoint, "API Endpoint")
                  }
                >
                  <ContentCopyIcon />
                </IconButton>
              </Tooltip>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>API Key:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>{"*".repeat(20)}</FieldValue>
              <Tooltip title="Copy to clipboard" placement="top">
                <IconButton
                  onClick={() =>
                    copyToClipboard(llm.attributes.api_key, "API Key")
                  }
                >
                  <ContentCopyIcon />
                </IconButton>
              </Tooltip>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Logo URL:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <img
                src={llm.attributes.logo_url}
                alt="LLM Logo"
                style={{
                  width: 50,
                  height: 50,
                  marginRight: 8,
                  objectFit: "contain",
                }}
              />
              <Link
                href={llm.attributes.logo_url}
                target="_blank"
                rel="noopener noreferrer"
                sx={{
                  maxWidth: "300px",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                }}
              >
                {llm.attributes.logo_url}
              </Link>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Active:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              <FiberManualRecordIcon
                sx={{
                  color: llm.attributes.active ? "green" : "red",
                  verticalAlign: "middle",
                  marginRight: 1,
                }}
              />
              {llm.attributes.active ? "Active" : "Inactive"}
            </FieldValue>
          </Grid>
        </Grid>
        <Box
          mb={2}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography color="success.main">{copySuccess}</Typography>
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/llms/edit/${id}`)}
          >
            Edit LLM
          </StyledButton>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default LLMDetails;
