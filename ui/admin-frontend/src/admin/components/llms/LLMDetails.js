import React, { useState, useEffect, useMemo } from "react";
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
import { MemoizedLineChart } from "../common/MemoizedChart";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip as ChartTooltip,
  Legend,
  TimeScale,
} from "chart.js";
import "chartjs-adapter-date-fns";
import DateRangePicker from "../../components/common/DateRangePicker";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../../utils/vendorLogos";
import Chip from "@mui/material/Chip";
import { useTheme } from "@mui/material/styles";

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  ChartTooltip,
  Legend,
  TimeScale,
);

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const LLMDetails = () => {
  const [llm, setLLM] = useState(null);
  const [loading, setLoading] = useState(true);
  const [copySuccess, setCopySuccess] = useState("");
  const [vendorUsageData, setVendorUsageData] = useState(null);
  const [startDate, setStartDate] = useState(
    new Date(new Date().getTime() - 30 * 24 * 60 * 60 * 1000)
      .toISOString()
      .split("T")[0],
  );
  const [endDate, setEndDate] = useState(
    new Date().toISOString().split("T")[0],
  );
  const { id } = useParams();
  const navigate = useNavigate();

  const apiEndpointPlaceholder = "API Endpoint not set";
  const apiKeyPlaceholder = "API Key not set";
  const theme = useTheme();

  useEffect(() => {
    fetchLLMDetails();
  }, [id]);

  useEffect(() => {
    if (llm) {
      fetchVendorUsage();
    }
  }, [llm, startDate, endDate]);

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

  const fetchVendorUsage = async () => {
    try {
      const response = await apiClient.get(`/analytics/vendor-usage`, {
        params: {
          start_date: startDate,
          end_date: endDate,
          vendor: llm.attributes.vendor,
        },
      });
      setVendorUsageData(response.data);
    } catch (error) {
      console.error("Error fetching vendor usage data", error);
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

  const chartOptions = useMemo(() => ({
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: {
        type: "time",
        time: {
          unit: "day",
        },
        title: {
          display: true,
          text: "Date",
        },
      },
      y: {
        beginAtZero: true,
        title: {
          display: true,
          text: "Token Usage",
        },
      },
    },
    plugins: {
      legend: {
        position: "top",
      },
      title: {
        display: true,
        text: "Vendor Token Usage Over Time",
      },
    },
  }), []); // Empty dependency array since options never change

  const chartData = useMemo(() => ({
    labels: vendorUsageData?.labels || [],
    datasets: [
      {
        label: "Token Usage",
        data: vendorUsageData?.data || [],
        borderColor: "rgb(75, 192, 192)",
        tension: 0.1,
      },
    ],
  }), [vendorUsageData]);

  if (loading) return <CircularProgress />;
  if (!llm) return <Typography>LLM not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5">LLM Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/llms")}
          color="inherit"
        >
          Back to LLMs
        </Button>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Vendor Usage Statistics</SectionTitle>
        <Box height={300}>
          {" "}
          {/* Reduced height from 400 to 250 */}
          <MemoizedLineChart options={chartOptions} data={chartData} />
        </Box>
          <Box mt={2}>
            <DateRangePicker
              startDate={startDate}
              endDate={endDate}
              onStartDateChange={setStartDate}
              onEndDateChange={setEndDate}
              onUpdate={fetchVendorUsage}
              updateMode="immediate"
            />
          </Box>

        <Divider sx={{ my: 3 }} />

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

        <SectionTitle>Model Configuration</SectionTitle>
        <Typography variant="body2" color="text.secondary" paragraph>
          The following model patterns are allowed for this LLM. These patterns
          are used to validate model requests through the API Gateway.
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Default Model:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {llm.attributes.default_model || "No default model set"}
            </FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Allowed Models:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            {llm.attributes.allowed_models &&
            llm.attributes.allowed_models.length > 0 ? (
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                {llm.attributes.allowed_models.map((model, index) => (
                  <Chip
                    key={index}
                    label={model}
                    color="primary"
                    variant="outlined"
                    sx={{
                      backgroundColor: theme.palette.background.paper,
                      "& .MuiChip-label": {
                        color: theme.palette.text.primary,
                      },
                    }}
                  />
                ))}
              </Box>
            ) : (
              <FieldValue>No model patterns specified</FieldValue>
            )}
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: "block", mt: 1 }}
            >
              These patterns use regex matching to determine which models are
              allowed. For example, "gpt-4.*" allows all GPT-4 models.
            </Typography>
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
            <FieldLabel>Loaded into Gateway:</FieldLabel>
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
            onClick={() => navigate(`/admin/llms/edit/${id}`)}
          >
            Edit LLM
          </StyledButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default LLMDetails;
