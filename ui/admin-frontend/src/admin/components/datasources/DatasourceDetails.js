import React, { useState, useEffect } from "react";
import { useParams, useNavigate, NavLink } from "react-router-dom";
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
  Chip,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import {
  StyledButtonPrimaryOutlined,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
  StyledAccordion,
} from "../../styles/sharedStyles";
import {
  getVectorStoreName,
  getVectorStoreLogo,
  getEmbedderName,
  getEmbedderLogo,
} from "../../utils/vendorUtils";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const DatasourceDetails = () => {
  const [datasource, setDatasource] = useState(null);
  const [owner, setOwner] = useState(null);
  const [loading, setLoading] = useState(true);
  const [copySuccess, setCopySuccess] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchDatasourceDetails();
  }, [id]);

  const fetchDatasourceDetails = async () => {
    try {
      const response = await apiClient.get(`/datasources/${id}`);
      const processedData = {
        ...response.data.data,
        attributes: {
          ...response.data.data.attributes,
          tags: response.data.data.attributes.tags.map((tag) =>
            typeof tag === "object" ? tag.attributes.name : tag,
          ),
        },
      };
      setDatasource(processedData);

      // Fetch owner details
      if (processedData.attributes.user_id) {
        const ownerResponse = await apiClient.get(
          `/users/${processedData.attributes.user_id}`,
        );
        setOwner(ownerResponse.data.data);
      }

      setLoading(false);
    } catch (error) {
      console.error("Error fetching datasource details", error);
      setLoading(false);
    }
  };

  const handleCloneDataSource = async () => {
    try {
      // Create new datasource data with modified name
      const cloneData = {
        data: {
          type: "datasources",
          attributes: {
            ...datasource.attributes,
            name: `Copy of ${datasource.attributes.name}`,
            active: false,
            // Reset files array if it exists
            files: [],
          },
        },
      };

      // Create new datasource
      const response = await apiClient.post("/datasources", cloneData);
      const newDatasourceId = response.data.data.id;

      // Redirect to edit page of new datasource
      navigate(`/admin/datasources/edit/${newDatasourceId}`);
    } catch (error) {
      console.error("Error cloning datasource:", error);
      // You might want to add some error handling here, like showing a snackbar
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
  if (!datasource) return <Typography>Datasource not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5">Datasource Details</Typography>
        <Link component={NavLink} to="/admin/datasources">
          <ArrowBackIcon sx={{ mr: 1 }} />
          Back to Datasources
        </Link>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Basic Information</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{datasource.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Short Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{datasource.attributes.short_description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Owner:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{owner ? owner.attributes.name : "Unknown"}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Vector Database Type:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <img
                src={getVectorStoreLogo(datasource.attributes.db_source_type)}
                alt={getVectorStoreName(datasource.attributes.db_source_type)}
                style={{
                  width: 24,
                  height: 24,
                  marginRight: 8,
                  objectFit: "contain",
                }}
              />
              <FieldValue>
                {getVectorStoreName(datasource.attributes.db_source_type)}
              </FieldValue>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Embedding Service Vendor:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <img
                src={getEmbedderLogo(datasource.attributes.embed_vendor)}
                alt={getEmbedderName(datasource.attributes.embed_vendor)}
                style={{
                  width: 24,
                  height: 24,
                  marginRight: 8,
                  objectFit: "contain",
                }}
              />
              <FieldValue>
                {getEmbedderName(datasource.attributes.embed_vendor)}
              </FieldValue>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Privacy Score:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>{datasource.attributes.privacy_score}</FieldValue>
              <Tooltip
                title="Privacy score is a value between 0 and 100, where 0 is the lowest and 100 is the highest. This determines the privacy level of the datasource."
                placement="top"
              >
                <HelpOutlineIcon
                  sx={{ ml: 1, fontSize: 20, color: "text.secondary" }}
                />
              </Tooltip>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Active:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              <FiberManualRecordIcon
                sx={{
                  color: datasource.attributes.active ? "green" : "red",
                  verticalAlign: "middle",
                  marginRight: 1,
                }}
              />
              {datasource.attributes.active ? "Active" : "Inactive"}
            </FieldValue>
          </Grid>
        </Grid>

        <StyledAccordion>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Typography>Vector Database Access Details</Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Grid container spacing={2}>
              <Grid item xs={3}>
                <FieldLabel>Database / Namespace Name:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>
                  {datasource.attributes.db_name || "Not set"}
                </FieldValue>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>Connection String:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  <FieldValue>
                    {datasource.attributes.db_conn_string
                      ? "*".repeat(20)
                      : "Not set"}
                  </FieldValue>
                  {datasource.attributes.db_conn_string && (
                    <Tooltip title="Copy to clipboard" placement="top">
                      <IconButton
                        onClick={() =>
                          copyToClipboard(
                            datasource.attributes.db_conn_string,
                            "DB Connection String",
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
                    {datasource.attributes.db_conn_api_key
                      ? "*".repeat(20)
                      : "Not set"}
                  </FieldValue>
                  {datasource.attributes.db_conn_api_key && (
                    <Tooltip title="Copy to clipboard" placement="top">
                      <IconButton
                        onClick={() =>
                          copyToClipboard(
                            datasource.attributes.db_conn_api_key,
                            "DB Connection API Key",
                          )
                        }
                      >
                        <ContentCopyIcon />
                      </IconButton>
                    </Tooltip>
                  )}
                </Box>
              </Grid>
            </Grid>
          </AccordionDetails>
        </StyledAccordion>

        <StyledAccordion>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Typography>Embedding Service Details</Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Grid container spacing={2}>
              <Grid item xs={3}>
                <FieldLabel>Model:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>
                  {datasource.attributes.embed_model || "Not set"}
                </FieldValue>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>Service URL:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>
                  {datasource.attributes.embed_url || "Not set"}
                </FieldValue>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>API Key:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  <FieldValue>
                    {datasource.attributes.embed_api_key
                      ? "*".repeat(20)
                      : "Not set"}
                  </FieldValue>
                  {datasource.attributes.embed_api_key && (
                    <Tooltip title="Copy to clipboard" placement="top">
                      <IconButton
                        onClick={() =>
                          copyToClipboard(
                            datasource.attributes.embed_api_key,
                            "Embed API Key",
                          )
                        }
                      >
                        <ContentCopyIcon />
                      </IconButton>
                    </Tooltip>
                  )}
                </Box>
              </Grid>
            </Grid>
          </AccordionDetails>
        </StyledAccordion>

        <StyledAccordion>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Typography>Additional Information</Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Grid container spacing={2}>
              <Grid item xs={3}>
                <FieldLabel>Long Description:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>
                  {datasource.attributes.long_description}
                </FieldValue>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>Icon:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  <img
                    src={datasource.attributes.icon}
                    alt="Datasource Icon"
                    style={{
                      width: 50,
                      height: 50,
                      marginRight: 8,
                      objectFit: "contain",
                    }}
                  />
                  <Link
                    href={datasource.attributes.icon}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{
                      maxWidth: "300px",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {datasource.attributes.icon}
                  </Link>
                </Box>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>Tags:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <Box>
                  {datasource.attributes.tags.map((tag) => (
                    <Chip
                      key={tag}
                      label={tag}
                      size="small"
                      sx={{ mr: 0.5, mb: 0.5 }}
                    />
                  ))}
                </Box>
              </Grid>
            </Grid>
          </AccordionDetails>
        </StyledAccordion>

        <Box
          mt={4}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography color="success.main">{copySuccess}</Typography>
          <Box display="flex" gap={2}>
            <StyledButtonPrimaryOutlined
              variant="contained"
              color="secondary"
              onClick={handleCloneDataSource}
              startIcon={<ContentCopyIcon />}
            >
              Clone This Data Source
            </StyledButtonPrimaryOutlined>
            <StyledButton
              variant="contained"
              startIcon={<EditIcon />}
              onClick={() => navigate(`/admin/datasources/edit/${id}`)}
            >
              Edit Datasource
            </StyledButton>
          </Box>
        </Box>
      </ContentBox>
    </>
  );
};

export default DatasourceDetails;
