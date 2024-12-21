import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Chip,
  Divider,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import FolderIcon from "@mui/icons-material/Folder";
import { List, ListItem, ListItemText, ListItemIcon } from "@mui/material";
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
  Tooltip,
  Legend,
  TimeScale,
);

const ChatDetails = () => {
  const [chat, setChat] = useState(null);
  const [llm, setLLM] = useState(null);
  const [llmSettings, setLLMSettings] = useState(null);
  const [filters, setFilters] = useState([]);
  const [loading, setLoading] = useState(true);
  const [interactionsData, setInteractionsData] = useState(null);
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
  const [defaultDataSource, setDefaultDataSource] = useState(null);
  const [extraContext, setExtraContext] = useState([]);

  useEffect(() => {
    fetchChatDetails();
  }, [id]);

  useEffect(() => {
    if (chat) {
      fetchChatInteractions();
    }
  }, [chat, startDate, endDate]);

  const fetchChatDetails = async () => {
    try {
      const chatResponse = await apiClient.get(`/chats/${id}`);
      setChat(chatResponse.data.data);
      setFilters(chatResponse.data.data.attributes.filters || []);

      // Fetch default data source if it exists
      if (chatResponse.data.data.attributes.default_data_source_id) {
        const dataSourceResponse = await apiClient.get(
          `/datasources/${chatResponse.data.data.attributes.default_data_source_id}`,
        );
        setDefaultDataSource(dataSourceResponse.data.data);
      }

      // Fetch extra context files
      const extraContextResponse = await apiClient.get(
        `/chats/${id}/extra-context`,
      );
      setExtraContext(extraContextResponse.data.data || []);

      const llmResponse = await apiClient.get(
        `/llms/${chatResponse.data.data.attributes.llm_id}`,
      );
      setLLM(llmResponse.data.data);

      const llmSettingsResponse = await apiClient.get(
        `/llm-settings/${chatResponse.data.data.attributes.llm_settings_id}`,
      );
      setLLMSettings(llmSettingsResponse.data.data);

      setLoading(false);
    } catch (error) {
      console.error("Error fetching chat details", error);
      setLoading(false);
    }
  };

  const fetchChatInteractions = async () => {
    try {
      const response = await apiClient.get(
        "/analytics/chat-interactions-for-chat",
        {
          params: {
            start_date: startDate,
            end_date: endDate,
            chat_id: id,
          },
        },
      );
      setInteractionsData(response.data);
    } catch (error) {
      console.error("Error fetching chat interactions", error);
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
          text: "Interactions",
        },
      },
    },
    plugins: {
      legend: {
        position: "top",
      },
      title: {
        display: true,
        text: "Chat Interactions Over Time",
      },
    },
  };

  const chartData = {
    labels: interactionsData?.labels || [],
    datasets: [
      {
        label: "Interactions",
        data: interactionsData?.data || [],
        borderColor: "rgb(75, 192, 192)",
        tension: 0.1,
      },
    ],
  };

  if (loading) return <CircularProgress />;
  if (!chat) return <Typography>Chat not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5">Chat Room Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/chats")}
          color="inherit"
        >
          Back to Chat Rooms
        </Button>
      </TitleBox>
      <ContentBox>
        <Typography variant="h6" gutterBottom>
          Chat Interactions
        </Typography>
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

        <Typography variant="h6" gutterBottom>
          Chat Information
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{chat.attributes.name}</FieldValue>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>Filters:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
              {filters.map((filter) => (
                <Chip key={filter.id} label={filter.attributes.name} />
              ))}
            </Box>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>LLM Settings:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {llmSettings ? llmSettings.attributes.model_name : "Loading..."}
            </FieldValue>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>LLM:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm ? llm.attributes.name : "Loading..."}</FieldValue>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>Groups:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
              {chat.attributes.groups.map((group) => (
                <Chip key={group.id} label={group.attributes.name} />
              ))}
            </Box>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>System Prompt:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {chat.attributes.system_prompt ||
                "No system prompt set, check LLM Settings for fallback prompt"}
            </FieldValue>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>Tool Support:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {chat.attributes.tool_support ? "Enabled" : "Disabled"}
            </FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <Typography variant="h6" gutterBottom>
          Default Data Source
        </Typography>
        {defaultDataSource ? (
          <Grid container spacing={2}>
            <Grid item xs={3}>
              <FieldLabel>Name:</FieldLabel>
            </Grid>
            <Grid item xs={9}>
              <FieldValue>{defaultDataSource.attributes.name}</FieldValue>
            </Grid>
            <Grid item xs={3}>
              <FieldLabel>Description:</FieldLabel>
            </Grid>
            <Grid item xs={9}>
              <FieldValue>
                {defaultDataSource.attributes.short_description}
              </FieldValue>
            </Grid>
            <Grid item xs={3}>
              <FieldLabel>Privacy Score:</FieldLabel>
            </Grid>
            <Grid item xs={9}>
              <FieldValue>
                {defaultDataSource.attributes.privacy_score}
              </FieldValue>
            </Grid>
          </Grid>
        ) : (
          <Typography color="text.secondary">
            No default data source set
          </Typography>
        )}

        <Divider sx={{ my: 3 }} />

        <Typography variant="h6" gutterBottom>
          Extra Context Files
        </Typography>
        {extraContext.length > 0 ? (
          <List>
            {extraContext.map((file) => (
              <ListItem key={file.id}>
                <ListItemIcon>
                  <FolderIcon />
                </ListItemIcon>
                <ListItemText
                  primary={file.attributes.file_name}
                  secondary={`Size: ${file.attributes.length} bytes${
                    file.attributes.description
                      ? ` • ${file.attributes.description}`
                      : ""
                  }`}
                />
              </ListItem>
            ))}
          </List>
        ) : (
          <Typography color="text.secondary">
            No extra context files added
          </Typography>
        )}

        <Box mt={4}>
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/chats/edit/${id}`)}
          >
            Edit Chat Room
          </StyledButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default ChatDetails;
