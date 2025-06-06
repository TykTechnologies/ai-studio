import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Divider,
  Tooltip,
  List,
  ListItem,
  ListItemText,
  Paper
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import DataUsageIcon from "@mui/icons-material/DataUsage";
import { Line, Bar } from "react-chartjs-2";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  Tooltip as ChartTooltip,
  Legend,
  TimeScale,
  Filler,
} from "chart.js";
import "chartjs-adapter-date-fns";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
} from "../../styles/sharedStyles";
import DateRangePicker from "../common/DateRangePicker";
import { styled } from "@mui/material/styles";

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  ChartTooltip,
  Legend,
  TimeScale,
  Filler,
);

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const NoDataMessage = ({ message }) => (
  <Box
    display="flex"
    flexDirection="column"
    alignItems="center"
    justifyContent="center"
    height="100%"
    minHeight="200px"
  >
    <DataUsageIcon sx={{ fontSize: 60, color: "text.secondary", mb: 2 }} />
    <Typography variant="body1" color="text.secondary">
      {message}
    </Typography>
  </Box>
);

const ChartPaper = styled(Paper)(({ theme }) => ({
  backgroundColor: theme.palette.background.paper,
  borderRadius: theme.shape.borderRadius * 3,
  border: `1px solid rgba(0, 0, 0, 0.12)`,
  boxShadow: "none",
  overflow: "hidden",
  padding: theme.spacing(3),
  paddingBottom: theme.spacing(6),
  height: 450,
}));

const ToolDetails = () => {
  const [tool, setTool] = useState(null);
  const [operations, setOperations] = useState([]);
  const [loading, setLoading] = useState(true);
  const [toolUsageData, setToolUsageData] = useState(null);
  const [toolOperationsUsageData, setToolOperationsUsageData] = useState(null);
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

  useEffect(() => {
    fetchToolDetails();
    fetchToolOperations();
    fetchToolAnalytics();
  }, [id, startDate, endDate]);

  const fetchToolDetails = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}`);
      setTool(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching tool details", error);
      setLoading(false);
    }
  };

  const fetchToolOperations = async () => {
    try {
      const response = await apiClient.get(`/tools/${id}/operations`);
      setOperations(response.data.data.operations);
    } catch (error) {
      console.error("Error fetching tool operations", error);
    }
  };

  const fetchToolAnalytics = async () => {
    try {
      const response = await apiClient.get(`/analytics/tool-operations-usage-over-time`, {
        params: { 
          start_date: startDate, 
          end_date: endDate, 
          tool_id: id 
        },
      });
      setToolOperationsUsageData(response.data);
      setToolUsageData(null); // We no longer need the old chart data
    } catch (error) {
      console.error("Error fetching tool analytics", error);
      setToolUsageData(null);
      setToolOperationsUsageData(null);
    }
  };

  const toolOperationsChartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: {
        title: {
          display: true,
          text: "Date",
        },
        stacked: true,
      },
      y: {
        beginAtZero: true,
        title: {
          display: true,
          text: "Usage Count",
        },
        stacked: true,
      },
    },
    plugins: {
      legend: {
        position: "top",
      },
      title: {
        display: true,
        text: "Tool Operations Usage Over Time",
      },
      tooltip: {
        mode: 'index',
      },
    },
  };

  const toolOperationsChartData = {
    labels: toolOperationsUsageData?.labels || [],
    datasets: toolOperationsUsageData?.datasets?.map((dataset, index) => ({
      label: dataset.label,
      data: dataset.data,
      borderColor: `hsl(${(index * 60) % 360}, 70%, 50%)`,
      backgroundColor: `hsla(${(index * 60) % 360}, 70%, 50%, 0.5)`,
      fill: true,
    })) || [],
  };

  const handleStartDateChange = (newDate) => {
    setStartDate(newDate);
  };

  const handleEndDateChange = (newDate) => {
    setEndDate(newDate);
  };

  if (loading) return <CircularProgress />;
  if (!tool) return <Typography>Tool not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Tool details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/tools")}
          color="inherit"
        >
          Back to tools
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Tool Operations Usage Analytics</SectionTitle>
        <Grid container spacing={3}>
          <Grid item xs={12}>
            <ChartPaper elevation={3}>
              <Typography variant="h6" gutterBottom>
                Tool Operations Usage Over Time
              </Typography>
              {toolOperationsUsageData ? (
                <Line options={toolOperationsChartOptions} data={toolOperationsChartData} />
              ) : (
                <NoDataMessage message="No tool operations usage data available for the selected period." />
              )}
            </ChartPaper>
          </Grid>
        </Grid>
        <Box mt={2} mb={4}>
          <DateRangePicker
            startDate={startDate}
            endDate={endDate}
            onStartDateChange={handleStartDateChange}
            onEndDateChange={handleEndDateChange}
          />
        </Box>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Tool Information</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{tool.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{tool.attributes.description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Privacy Level:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>{tool.attributes.privacy_score}</FieldValue>
              <Tooltip
                title="Privacy level is a value between 0 and 100, where 0 is the lowest and 100 is the highest. This determines the privacy level of the tool."
                placement="top"
              >
                <HelpOutlineIcon
                  sx={{ ml: 1, fontSize: 20, color: "text.secondary" }}
                />
              </Tooltip>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Tool Type:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>REST</FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Authentication Details</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Auth Schema Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{tool.attributes.auth_schema_name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Auth Key:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {tool.attributes.auth_key ? "*".repeat(20) : "Not set"}
            </FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>OpenAPI Specification</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={12}>
            <FieldValue>
              {tool.attributes.oas_spec
                ? "OpenAPI Specification is set"
                : "OpenAPI Specification is not set"}
            </FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Operations</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={12}>
            {operations.length > 0 ? (
              <List sx={{ listStyleType: "decimal", pl: 4 }}>
                {operations.map((operation, index) => (
                  <ListItem key={index} sx={{ display: "list-item" }}>
                    <ListItemText
                      primary={
                        <Typography sx={{ fontFamily: "monospace" }}>
                          {operation}
                        </Typography>
                      }
                    />
                  </ListItem>
                ))}
              </List>
            ) : (
              <FieldValue>No operations set for this tool.</FieldValue>
            )}
          </Grid>
        </Grid>

        <Box mt={4} display="flex" justifyContent="flex-end">
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/tools/edit/${id}`)}
          >
            Edit tool
          </PrimaryButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default ToolDetails;
