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
  Tooltip,
  IconButton,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import { Line } from "react-chartjs-2";
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
import DateRangePicker from "../common/DateRangePicker";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";

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

const TooltipLabel = ({ label, tooltip }) => (
  <Box display="flex" alignItems="center">
    <FieldLabel>{label}</FieldLabel>
    <Tooltip title={tooltip} arrow placement="top-start">
      <IconButton size="small" sx={{ ml: 0.5 }}>
        <HelpOutlineIcon fontSize="small" />
      </IconButton>
    </Tooltip>
  </Box>
);

const LLMSettingsDetails = () => {
  const [setting, setSetting] = useState(null);
  const [loading, setLoading] = useState(true);
  const [usageData, setUsageData] = useState(null);
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
    fetchSettingDetails();
  }, [id]);

  useEffect(() => {
    if (setting) {
      fetchUsageData();
    }
  }, [setting, startDate, endDate]);

  const fetchSettingDetails = async () => {
    try {
      const response = await apiClient.get(`/llm-settings/${id}`);
      setSetting(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching LLM Call Settings details", error);
      setLoading(false);
    }
  };

  const fetchUsageData = async () => {
    try {
      const response = await apiClient.get(`/analytics/model-usage`, {
        params: {
          start_date: startDate,
          end_date: endDate,
          model_name: setting.attributes.model_name,
        },
      });
      setUsageData(response.data);
    } catch (error) {
      console.error("Error fetching model usage data", error);
    }
  };

  const chartOptions = {
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
        text: "Model Usage Over Time",
      },
    },
  };

  const chartData = {
    labels: usageData?.labels || [],
    datasets: [
      {
        label: "Token Usage",
        data: usageData?.data || [],
        borderColor: "rgb(75, 192, 192)",
        tension: 0.1,
      },
    ],
  };

  if (loading) return <CircularProgress />;
  if (!setting) return <Typography>LLM Call Settings not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5">LLM Call Settings Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/llm-settings")}
          color="inherit"
        >
          Back to LLM Call Settings
        </Button>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Model Usage Statistics</SectionTitle>
        <Box height={300}>
          <Line options={chartOptions} data={chartData} />
        </Box>
        <Box mt={2}>
          <DateRangePicker
            startDate={startDate}
            endDate={endDate}
            onStartDateChange={setStartDate}
            onEndDateChange={setEndDate}
          />
        </Box>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Basic Information</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={4}>
            <TooltipLabel
              label="Model Name:"
              tooltip="The name of the language model (e.g., 'gpt-3.5-turbo', 'text-davinci-003')"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.model_name}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Temperature:"
              tooltip="Controls randomness: 0 is deterministic, 1 is very random. Range: 0 to 1"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.temperature}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Max Tokens:"
              tooltip="The maximum number of tokens to generate in the response"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.max_tokens}</FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Advanced Settings</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={4}>
            <TooltipLabel
              label="Top P:"
              tooltip="Controls diversity via nucleus sampling: 0.5 means half of all likelihood-weighted options are considered. Range: 0 to 1"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.top_p}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Top K:"
              tooltip="Controls diversity by limiting to k most likely tokens. 0 means no limit"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.top_k}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Min Length:"
              tooltip="The minimum number of tokens to generate in the response"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.min_length}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Max Length:"
              tooltip="The maximum number of overall tokens"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.max_length}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Repetition Penalty:"
              tooltip="Penalizes repetition: 1.0 means no penalty, >1.0 discourages repetition. Typically between 1.0 and 1.5"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.repetition_penalty}</FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>System Prompt</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={12}>
            <TooltipLabel
              label="System Prompt:"
              tooltip="A long-form text prompt that sets the context or behavior for the language model"
            />
          </Grid>
          <Grid item xs={12}>
            <FieldValue
              sx={{
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
                maxHeight: "200px",
                overflowY: "auto",
                padding: "10px",
                border: "1px solid #e0e0e0",
                borderRadius: "4px",
              }}
            >
              {setting.attributes.system_prompt || "No system prompt set"}
            </FieldValue>
          </Grid>
        </Grid>

        <Box mt={4} display="flex" justifyContent="flex-end">
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/llm-settings/edit/${id}`)}
          >
            Edit LLM Call Settings
          </StyledButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default LLMSettingsDetails;
