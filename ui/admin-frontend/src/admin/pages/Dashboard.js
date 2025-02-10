import React, { useEffect, useState } from "react";

import { useNavigate } from "react-router-dom";

import apiClient from "../utils/apiClient";
import {
  Typography,
  Grid,
  Paper,
  TextField,
  Box,
  Button,
  Stack,
  Divider,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  ToggleButton,
  ToggleButtonGroup,
} from "@mui/material";
import {
  StyledButton,
  StyledPaper,
  TitleBox,
} from "../styles/sharedStyles";
import { Line, Bar } from "react-chartjs-2";
import { styled } from "@mui/material/styles";

import AddIcon from "@mui/icons-material/Add";

import CloseIcon from "@mui/icons-material/Close";

import CircularProgress from "@mui/material/CircularProgress";

import DataUsageIcon from "@mui/icons-material/DataUsage";

import ChatRoomWizard from "../components/wizards/ChatRoomWizard";

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  Tooltip,
  Legend,
} from "chart.js";
import { getVendorName, getVendorLogo } from "../utils/vendorLogos";
import IconButton from "@mui/material/IconButton";

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  Tooltip,
  Legend,
);

const NoDataMessage = ({ message }) => (
  <Box
    display="flex"
    flexDirection="column"
    alignItems="center"
    justifyContent="center"
    height="100%"
  >
    <DataUsageIcon sx={{ fontSize: 60, color: "text.secondary", mb: 2 }} />
    <Typography variant="body1" color="text.secondary">
      {message}
    </Typography>
  </Box>
);

const GetStartedWidget = ({ openChatRoomWizard, onClose }) => (
  <Box
    sx={{
      display: "flex",
      flexDirection: "column",
      alignItems: "center",
      justifyContent: "center",
      height: "100%",
      textAlign: "center",
      p: 4,
      mb: 4,
      backgroundColor: (theme) => theme.palette.custom.lightTeal,
      boxShadow: (theme) => theme.shadows[4],
      borderRadius: (theme) => theme.shape.borderRadius,
      position: "relative",
    }}
  >
    <IconButton
      onClick={onClose}
      sx={{
        position: "absolute",
        top: 8,
        right: 8,
      }}
    >
      <CloseIcon />
    </IconButton>
    <Typography variant="h4" gutterBottom>
      Get started with your first smart chat room
    </Typography>
    <Typography variant="body1" paragraph>
      Chat rooms enable non-technical users to benefit from the full power of AI
      in your organisation, safely and securely.
    </Typography>
    <Button
      variant="contained"
      sx={{
        backgroundColor: (theme) => theme.palette.custom.purpleDark,
        color: "white",
        "&:hover": {
          backgroundColor: (theme) => theme.palette.custom.purpleLight,
        },
      }}
      onClick={openChatRoomWizard}
      startIcon={<AddIcon />}
    >
      Create Chat Room
    </Button>
  </Box>
);

const Dashboard = () => {
  const [isWizardOpen, setIsWizardOpen] = useState(false);

  const [isChatRoomWizardOpen, setIsChatRoomWizardOpen] = useState(false);

  const openChatRoomWizard = () => {
    setIsChatRoomWizardOpen(true);
  };

  const closeChatRoomWizard = () => {
    setIsChatRoomWizardOpen(false);
    fetchData();
  };

  const [chatData, setChatData] = useState(null);
  const [costData, setCostData] = useState({});
  const [llmModelData, setLLMModelData] = useState(null);
  const [toolUsageData, setToolUsageData] = useState(null);
  const [userActivityData, setUserActivityData] = useState(null);
  const [vendorModelCostData, setVendorModelCostData] = useState([]);
  const [isTableExpanded, setIsTableExpanded] = useState(false);

  const [llms, setLLMs] = useState([]);
  const [llmsLoading, setLLMsLoading] = useState(true);
  const [chats, setChats] = useState([]);
  const [chatsLoading, setChatsLoading] = useState(true);

  const [showGetStartedWidget, setShowGetStartedWidget] = useState(true);

  const hideGetStartedWidget = () => {
    setShowGetStartedWidget(false);
    localStorage.setItem("hideGetStartedWidget", "true");
  };

  const [startDate, setStartDate] = useState(
    new Date(new Date().getTime() - 7 * 24 * 60 * 60 * 1000)
      .toISOString()
      .split("T")[0],
  );
  const [endDate, setEndDate] = useState(
    new Date().toISOString().split("T")[0],
  );
  const [interactionType, setInteractionType] = useState(null);

  const handleInteractionTypeChange = (event, newValue) => {
    setInteractionType(newValue);
  };

  useEffect(() => {
    fetchData();
  }, []);

  useEffect(() => {
    const hideGetStarted =
      localStorage.getItem("hideGetStartedWidget") === "true";
    const shouldShowWidget = !hideGetStarted && chats.length === 0;
    setShowGetStartedWidget(shouldShowWidget);
  }, [chats.length]);

  const fetchData = async () => {
    try {
      const [llmResponse, chatResponse] = await Promise.all([
        apiClient.get("/llms"),
        apiClient.get("/chats"),
      ]);
      setLLMs(llmResponse.data.data || []);
      setChats(chatResponse.data.data || []);

      const [
        chatDataResponse,
        costResponse,
        llmModelResponse,
        toolUsageResponse,
        userActivityResponse,
        vendorModelCostResponse,
      ] = await Promise.all([
        apiClient.get("/analytics/chat-records-per-day", {
          params: { 
            start_date: startDate, 
            end_date: endDate,
            ...(interactionType && { interaction_type: interactionType }),
          },
        }),
        apiClient.get("/analytics/cost-analysis", {
          params: { 
            start_date: startDate, 
            end_date: endDate,
            ...(interactionType && { interaction_type: interactionType }),
          },
        }),
        apiClient.get("/analytics/most-used-llm-models", {
          params: { 
            start_date: startDate, 
            end_date: endDate,
            ...(interactionType && { interaction_type: interactionType }),
          },
        }),
        apiClient.get("/analytics/tool-usage-statistics", {
          params: { 
            start_date: startDate, 
            end_date: endDate,
            ...(interactionType && { interaction_type: interactionType }),
          },
        }),
        apiClient.get("/analytics/unique-users-per-day", {
          params: { 
            start_date: startDate, 
            end_date: endDate,
            ...(interactionType && { interaction_type: interactionType }),
          },
        }),
        apiClient.get("/analytics/total-cost-per-vendor-and-model", {
          params: { 
            start_date: startDate, 
            end_date: endDate,
            ...(interactionType && { interaction_type: interactionType }),
          },
        }),
      ]);

      setChatData(chatDataResponse.data);
      setCostData(costResponse.data);
      setLLMModelData(llmModelResponse.data);
      setToolUsageData(toolUsageResponse.data);
      setUserActivityData(userActivityResponse.data);
      setVendorModelCostData(vendorModelCostResponse.data);
    } catch (error) {
      console.error("Error fetching data:", error);
      if (error.response) {
        console.error("Response data:", error.response.data);
        console.error("Response status:", error.response.status);
        console.error("Response headers:", error.response.headers);
      } else if (error.request) {
        console.error("No response received:", error.request);
      } else {
        console.error("Error setting up request:", error.message);
      }
      // Set default values for all state variables
      setLLMs([]);
      setChats([]);
      setChatData(null);
      setCostData({});
      setLLMModelData(null);
      setToolUsageData(null);
      setUserActivityData(null);
      setVendorModelCostData([]);
    } finally {
      setLLMsLoading(false);
      setChatsLoading(false);
    }
  };

  const handleDateChange = () => {
    fetchData();
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: "top",
      },
    },
    scales: {
      y: {
        beginAtZero: true,
      },
    },
  };

  const createLineChartData = (data, label) => ({
    labels: data.labels,
    datasets: [
      {
        label: label,
        data: data.data,
        borderColor: "rgb(75, 192, 192)",
        tension: 0.1,
      },
    ],
  });

  const createBarChartData = (data, label) => ({
    labels: data.labels,
    datasets: [
      {
        label: label,
        data: data.data,
        backgroundColor: "rgba(75, 192, 192, 0.6)",
      },
    ],
  });

  const createMultiLineChartData = (data) => ({
    labels: Object.values(data)[0].labels,
    datasets: Object.entries(data).map(([currency, chartData]) => ({
      label: `Cost (${currency})`,
      data: chartData.data,
      borderColor: getRandomColor(),
      tension: 0.1,
    })),
  });

  const getRandomColor = () => {
    const r = Math.floor(Math.random() * 255);
    const g = Math.floor(Math.random() * 255);
    const b = Math.floor(Math.random() * 255);
    return `rgb(${r}, ${g}, ${b})`;
  };

  const StyledSectionTitle = styled(Box)(({ theme }) => ({
    marginBottom: theme.spacing(3),
    padding: theme.spacing(2),
    backgroundColor: theme.palette.custom.lightTeal,
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.custom.teal}`,
  }));

  const StyledTitle = styled(Typography)(({ theme }) => ({
    fontWeight: "bold",
    color: theme.palette.text.primary,
    marginBottom: theme.spacing(1),
  }));

  const StyledHelpText = styled(Typography)(({ theme }) => ({
    color: theme.palette.text.secondary,
  }));

  const toggleTableExpansion = () => {
    setIsTableExpanded(!isTableExpanded);
  };

  const SectionTitle = ({ title, helpText }) => (
    <StyledSectionTitle>
      <StyledTitle variant="h5" gutterBottom>
        {title}
      </StyledTitle>
      <StyledHelpText variant="body2">{helpText}</StyledHelpText>
    </StyledSectionTitle>
  );

  const ChartPaper = styled(Paper)(({ theme }) => ({
    backgroundColor: theme.palette.background.paper,
    borderRadius: theme.shape.borderRadius * 3,
    border: `1px solid rgba(0, 0, 0, 0.12)`,
    boxShadow: "none",
    overflow: "hidden",
    padding: theme.spacing(3),
    paddingBottom: theme.spacing(6), // Increased bottom padding
    height: 450, // Increased height to accommodate the extra padding
  }));

  const StyledTableCell = styled(TableCell)(({ theme }) => ({
    "&.MuiTableCell-head": {
      backgroundColor: theme.palette.custom.purpleLight,
      color: theme.palette.common.white,
    },
  }));

  const StyledTableRow = styled(TableRow)(({ theme }) => ({
    "&:nth-of-type(odd)": {
      backgroundColor: theme.palette.custom.lightTeal,
    },
    "&:nth-of-type(even)": {
      backgroundColor: theme.palette.common.white,
    },
    "&:hover": {
      backgroundColor: theme.palette.custom.hoverTeal,
    },
    // Remove last border
    "&:last-child td, &:last-child th": {
      border: 0,
    },
  }));

  return (
    <div>
      {llmsLoading || chatsLoading ? (
        <CircularProgress />
      ) : (
        <>
          {chats.length === 0 && showGetStartedWidget && (
            <GetStartedWidget
              openChatRoomWizard={openChatRoomWizard}
              onClose={hideGetStartedWidget}
            />
          )}

          <TitleBox top="64px">
            <Typography variant="h4">Dashboard</Typography>
            <Stack direction="row" spacing={2} alignItems="center">
              <TextField
                label="Start Date"
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                InputLabelProps={{ shrink: true }}
                size="small"
              />
              <TextField
                label="End Date"
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                InputLabelProps={{ shrink: true }}
                size="small"
              />
              <ToggleButtonGroup
                value={interactionType}
                exclusive
                onChange={handleInteractionTypeChange}
                size="small"
                sx={{ mr: 2 }}
              >
                <ToggleButton value={null}>All</ToggleButton>
                <ToggleButton value="chat">Chat</ToggleButton>
                <ToggleButton value="proxy">Proxy</ToggleButton>
              </ToggleButtonGroup>
              <StyledButton
                variant="contained"
                onClick={handleDateChange}
              >
                Update
              </StyledButton>
            </Stack>
          </TitleBox>

          <Box sx={{ p: 3 }}>
            <SectionTitle
              title="Conversations"
              helpText="Overview of user engagement and chat activity"
            />
            <Grid container spacing={3}>
              <Grid item xs={12} md={6}>
                <ChartPaper elevation={3}>
                  <Typography variant="h6" gutterBottom>
                    Unique Users per Day
                  </Typography>
                  {userActivityData ? (
                    <Line
                      options={chartOptions}
                      data={createLineChartData(
                        userActivityData,
                        "Unique Users",
                      )}
                    />
                  ) : (
                    <NoDataMessage message="No user activity data available for the selected period." />
                  )}
                </ChartPaper>
              </Grid>
              <Grid item xs={12} md={6}>
                <ChartPaper elevation={3}>
                  <Typography variant="h6" gutterBottom>
                    Chat Interactions per Day
                  </Typography>
                  {chatData ? (
                    <Line
                      options={chartOptions}
                      data={createLineChartData(chatData, "Chat Interactions")}
                    />
                  ) : (
                    <NoDataMessage message="No chat interaction data available for the selected period." />
                  )}
                </ChartPaper>
              </Grid>
            </Grid>
          </Box>

          <Divider sx={{ my: 4 }} />

          <Box sx={{ p: 3 }}>
            <SectionTitle
              title="Cost Analysis"
              helpText="Breakdown of costs for different currencies and usage of LLM models"
            />
            <Grid container spacing={3}>
              <Grid item xs={12}>
                <ChartPaper elevation={3}>
                  <Typography variant="h6" gutterBottom>
                    Cost Analysis by Currency {interactionType ? `(${interactionType})` : '(All)'}
                  </Typography>
                  {Object.keys(costData).length > 0 ? (
                    <Line
                      options={chartOptions}
                      data={createMultiLineChartData(costData)}
                    />
                  ) : (
                    <NoDataMessage message="No cost analysis data available for the selected period." />
                  )}
                </ChartPaper>
              </Grid>
              <Grid item xs={12}>
                <StyledPaper elevation={3} style={{ padding: "20px" }}>
                  <Typography variant="h6" gutterBottom>
                    Total Cost per Vendor and Model {interactionType ? `(${interactionType})` : '(All)'}
                  </Typography>
                  {vendorModelCostData.length > 0 ? (
                    <>
                      <TableContainer>
                        <Table>
                          <TableHead>
                            <TableRow>
                              <StyledTableCell>Vendor</StyledTableCell>
                              <StyledTableCell>Model</StyledTableCell>
                              <StyledTableCell align="right">
                                Total Cost
                              </StyledTableCell>
                              <StyledTableCell>Currency</StyledTableCell>
                            </TableRow>
                          </TableHead>
                          <TableBody>
                            {vendorModelCostData
                              .slice(0, isTableExpanded ? undefined : 5)
                              .map((row, index) => (
                                <StyledTableRow key={index}>
                                  <StyledTableCell>
                                    <Box
                                      sx={{
                                        display: "flex",
                                        alignItems: "center",
                                      }}
                                    >
                                      <img
                                        src={getVendorLogo(row.vendor)}
                                        alt={getVendorName(row.vendor)}
                                        style={{
                                          width: 24,
                                          height: 24,
                                          marginRight: 8,
                                          objectFit: "contain",
                                        }}
                                        onError={(e) => {
                                          e.target.onerror = null;
                                          e.target.src =
                                            process.env.PUBLIC_URL +
                                            "/images/placeholder-logo.png";
                                        }}
                                      />
                                      {getVendorName(row.vendor)}
                                    </Box>
                                  </StyledTableCell>
                                  <StyledTableCell>{row.model}</StyledTableCell>
                                  <StyledTableCell align="right">
                                    {row.totalCost.toFixed(2)}
                                  </StyledTableCell>
                                  <StyledTableCell>
                                    {row.currency}
                                  </StyledTableCell>
                                </StyledTableRow>
                              ))}
                          </TableBody>
                        </Table>
                      </TableContainer>
                      {vendorModelCostData.length > 5 && (
                        <Box mt={2} textAlign="center">
                          <Button onClick={toggleTableExpansion}>
                            {isTableExpanded ? "Collapse" : "Expand"}
                          </Button>
                        </Box>
                      )}
                    </>
                  ) : (
                    <NoDataMessage message="No vendor and model cost data available for the selected period." />
                  )}
                </StyledPaper>
              </Grid>
            </Grid>
          </Box>

          <Divider sx={{ my: 4 }} />

          <Box sx={{ p: 3 }}>
            <SectionTitle
              title="Model and Tool Usage"
              helpText="Analysis of most used LLM models and tools"
            />
            <Grid container spacing={3}>
              <Grid item xs={12} md={6}>
                <ChartPaper elevation={3}>
                  <Typography variant="h6" gutterBottom>
                    Most Used LLM Models {interactionType ? `(${interactionType})` : '(All)'}
                  </Typography>
                  {llmModelData ? (
                    <Bar
                      options={chartOptions}
                      data={createBarChartData(llmModelData, "LLM Models")}
                    />
                  ) : (
                    <NoDataMessage message="No LLM model usage data available for the selected period." />
                  )}
                </ChartPaper>
              </Grid>
              <Grid item xs={12} md={6}>
                <ChartPaper elevation={3}>
                  <Typography variant="h6" gutterBottom>
                    Tool Usage Statistics
                  </Typography>
                  {toolUsageData ? (
                    <Bar
                      options={chartOptions}
                      data={createBarChartData(toolUsageData, "Tool Usage")}
                    />
                  ) : (
                    <NoDataMessage message="No tool usage data available for the selected period." />
                  )}
                </ChartPaper>
              </Grid>
            </Grid>
          </Box>
        </>
      )}

      <ChatRoomWizard
        open={isChatRoomWizardOpen}
        onClose={closeChatRoomWizard}
        fetchData={fetchData}
      />
    </div>
  );
};

export default Dashboard;
