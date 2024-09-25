import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Divider,
  Chip,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import { Line } from "react-chartjs-2";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  TimeScale,
} from "chart.js";
import "chartjs-adapter-date-fns";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";
import DateRangePicker from "../common/DateRangePicker";

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  TimeScale,
);

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const AppDetails = () => {
  const [app, setApp] = useState(null);
  const [credential, setCredential] = useState(null);
  const [user, setUser] = useState(null);
  const [llms, setLLMs] = useState([]);
  const [datasources, setDatasources] = useState([]);
  const [loading, setLoading] = useState(true);
  const [tokenUsageAndCostData, setTokenUsageAndCostData] = useState(null);
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
    fetchAppDetails();
    fetchTokenUsageAndCost();
  }, [id, startDate, endDate]);

  const fetchAppDetails = async () => {
    try {
      const response = await apiClient.get(`/apps/${id}`);
      setApp(response.data.data);

      if (response.data.data.attributes.credential_id) {
        fetchCredential(response.data.data.attributes.credential_id);
      }

      fetchUser(response.data.data.attributes.user_id);
      fetchLLMs(response.data.data.attributes.llm_ids);
      fetchDatasources(response.data.data.attributes.datasource_ids);

      setLoading(false);
    } catch (error) {
      console.error("Error fetching app details", error);
      setLoading(false);
    }
  };

  const fetchTokenUsageAndCost = async () => {
    try {
      const response = await apiClient.get(
        `/analytics/token-usage-and-cost-for-app`,
        {
          params: { start_date: startDate, end_date: endDate, app_id: id },
        },
      );
      setTokenUsageAndCostData(response.data);
    } catch (error) {
      console.error("Error fetching token usage and cost data", error);
    }
  };

  const fetchCredential = async (credentialId) => {
    try {
      const response = await apiClient.get(`/credentials/${credentialId}`);
      setCredential(response.data.data);
    } catch (error) {
      console.error("Error fetching credential", error);
    }
  };

  const fetchUser = async (userId) => {
    try {
      const response = await apiClient.get(`/users/${userId}`);
      setUser(response.data.data);
    } catch (error) {
      console.error("Error fetching user", error);
    }
  };

  const fetchLLMs = async (llmIds) => {
    try {
      const llmPromises = llmIds.map((id) => apiClient.get(`/llms/${id}`));
      const llmResponses = await Promise.all(llmPromises);
      setLLMs(llmResponses.map((response) => response.data.data));
    } catch (error) {
      console.error("Error fetching LLMs", error);
    }
  };

  const fetchDatasources = async (datasourceIds) => {
    try {
      const datasourcePromises = datasourceIds.map((id) =>
        apiClient.get(`/datasources/${id}`),
      );
      const datasourceResponses = await Promise.all(datasourcePromises);
      setDatasources(datasourceResponses.map((response) => response.data.data));
    } catch (error) {
      console.error("Error fetching datasources", error);
    }
  };

  if (loading) return <CircularProgress />;
  if (!app) return <Typography>App not found</Typography>;

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      mode: "index",
      intersect: false,
    },
    stacked: false,
    plugins: {
      title: {
        display: true,
        text: "Token Usage and Cost Over Time",
      },
    },
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
        type: "linear",
        display: true,
        position: "left",
        title: {
          display: true,
          text: "Token Usage",
        },
      },
      y1: {
        type: "linear",
        display: true,
        position: "right",
        title: {
          display: true,
          text: "Cost",
        },
        grid: {
          drawOnChartArea: false,
        },
      },
    },
  };

  const chartData = {
    labels: tokenUsageAndCostData?.labels || [],
    datasets: [
      {
        label: "Token Usage",
        data: tokenUsageAndCostData?.datasets[0]?.data || [],
        borderColor: "rgb(75, 192, 192)",
        yAxisID: "y",
      },
      {
        label: "Cost",
        data: tokenUsageAndCostData?.datasets[1]?.data || [],
        borderColor: "rgb(255, 99, 132)",
        yAxisID: "y1",
      },
    ],
  };

  const handleStartDateChange = (newDate) => {
    setStartDate(newDate);
  };

  const handleEndDateChange = (newDate) => {
    setEndDate(newDate);
  };

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">App Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/apps")}
          color="white"
        >
          Back to Apps
        </Button>
      </TitleBox>
      <ContentBox>
        <SectionTitle>App Token Usage and Cost</SectionTitle>
        <Box height={300}>
          <Line options={chartOptions} data={chartData} />
        </Box>
        <Box mt={2} mb={4}>
          <DateRangePicker
            startDate={startDate}
            endDate={endDate}
            onStartDateChange={handleStartDateChange}
            onEndDateChange={handleEndDateChange}
          />
        </Box>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>App Information</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{app.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{app.attributes.description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>User:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {user ? user.attributes.name : "Loading..."}
            </FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>LLMs:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" flexWrap="wrap" gap={1}>
              {llms.map((llm) => (
                <Chip key={llm.id} label={llm.attributes.name} />
              ))}
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Datasources:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" flexWrap="wrap" gap={1}>
              {datasources.map((datasource) => (
                <Chip key={datasource.id} label={datasource.attributes.name} />
              ))}
            </Box>
          </Grid>
        </Grid>

        {credential && (
          <>
            <Divider sx={{ my: 3 }} />
            <SectionTitle>Credential Information</SectionTitle>
            <Grid container spacing={2}>
              <Grid item xs={3}>
                <FieldLabel>Key ID:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>{credential.attributes.key_id}</FieldValue>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>Secret:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>********</FieldValue>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>Active:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>
                  {credential.attributes.active ? "Yes" : "No"}
                </FieldValue>
              </Grid>
            </Grid>
          </>
        )}

        <Box
          mt={4}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/apps/edit/${id}`)}
          >
            Edit App
          </StyledButton>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default AppDetails;
