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
  Divider,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../../utils/vendorLogos";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const LLMDetails = () => {
  const [llm, setLLM] = useState(null);
  const [loading, setLoading] = useState(true);
  const [copySuccess, setCopySuccess] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  const apiEndpointPlaceholder = "API Endpoint not set";
  const apiKeyPlaceholder = "API Key not set";

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
        <SectionTitle>LLM Description</SectionTitle>
        <Grid container spacing={2}>
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
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>{llm.attributes.privacy_score}</FieldValue>
              <Tooltip
                title="Privacy score is a value between 0 and 100, where 0 is the lowest and 100 is the highest. This determines the privacy level of the LLM for Data Source sharing."
                placement="top"
              >
                <HelpOutlineIcon
                  sx={{ ml: 1, fontSize: 20, color: "text.secondary" }}
                />
              </Tooltip>
            </Box>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Access Details</SectionTitle>
        <Typography variant="body2" color="text.secondary" paragraph>
          Some LLMs do not require an API Key for access, or have a default URL
          (for example Anthropic and OopenAI). If you have an LLM provider that
          is not on the list, but provides an OpenAPI compatible API, you can
          use the compatible vendor setting and override the default URL.
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>API Endpoint:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>
                {llm.attributes.api_endpoint || apiEndpointPlaceholder}
              </FieldValue>
              {llm.attributes.api_endpoint && (
                <Tooltip title="Copy to clipboard" placement="top">
                  <IconButton
                    onClick={() =>
                      copyToClipboard(
                        llm.attributes.api_endpoint,
                        "API Endpoint",
                      )
                    }
                  >
                    <ContentCopyIcon />
                  </IconButton>
                </Tooltip>
              )}
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>API Key:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>
                {llm.attributes.api_key ? "*".repeat(20) : apiKeyPlaceholder}
              </FieldValue>
              {llm.attributes.api_key && (
                <Tooltip title="Copy to clipboard" placement="top">
                  <IconButton
                    onClick={() =>
                      copyToClipboard(llm.attributes.api_key, "API Key")
                    }
                  >
                    <ContentCopyIcon />
                  </IconButton>
                </Tooltip>
              )}
            </Box>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Portal Display Information</SectionTitle>
        <Typography variant="body2" color="text.secondary" paragraph>
          The following settings will be used in the Portal UI that your
          end-users / developers will see when browsing for LLMs to use.
        </Typography>
        <Grid container spacing={2}>
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
          mt={4}
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
