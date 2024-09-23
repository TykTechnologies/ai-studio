import React, { useEffect, useState } from "react";
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
} from "@mui/material";
import { Line, Bar } from "react-chartjs-2";
import { styled } from "@mui/material/styles";
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

const Dashboard = () => {
  const [chatData, setChatData] = useState(null);
  const [costData, setCostData] = useState({});
  const [llmModelData, setLLMModelData] = useState(null);
  const [toolUsageData, setToolUsageData] = useState(null);
  const [userActivityData, setUserActivityData] = useState(null);
  const [vendorModelCostData, setVendorModelCostData] = useState([]);
  const [isTableExpanded, setIsTableExpanded] = useState(false);
  const [startDate, setStartDate] = useState(
    new Date(new Date().getTime() - 7 * 24 * 60 * 60 * 1000)
      .toISOString()
      .split("T")[0],
  );
  const [endDate, setEndDate] = useState(
    new Date().toISOString().split("T")[0],
  );

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      const [
        chatResponse,
        costResponse,
        llmModelResponse,
        toolUsageResponse,
        userActivityResponse,
        vendorModelCostResponse,
      ] = await Promise.all([
        apiClient.get("/analytics/chat-records-per-day", {
          params: { start_date: startDate, end_date: endDate },
        }),
        apiClient.get("/analytics/cost-analysis", {
          params: { start_date: startDate, end_date: endDate },
        }),
        apiClient.get("/analytics/most-used-llm-models", {
          params: { start_date: startDate, end_date: endDate },
        }),
        apiClient.get("/analytics/tool-usage-statistics", {
          params: { start_date: startDate, end_date: endDate },
        }),
        apiClient.get("/analytics/unique-users-per-day", {
          params: { start_date: startDate, end_date: endDate },
        }),
        apiClient.get("/analytics/total-cost-per-vendor-and-model", {
          params: { start_date: startDate, end_date: endDate },
        }),
      ]);

      setChatData(chatResponse.data);
      setCostData(costResponse.data);
      setLLMModelData(llmModelResponse.data);
      setToolUsageData(toolUsageResponse.data);
      setUserActivityData(userActivityResponse.data);
      setVendorModelCostData(vendorModelCostResponse.data);
    } catch (error) {
      console.error("Error fetching dashboard data", error);
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
      <Box
        display="flex"
        justifyContent="space-between"
        alignItems="center"
        mb={3}
      >
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
          <Button variant="contained" onClick={handleDateChange} size="small">
            Update
          </Button>
        </Stack>
      </Box>

      <Box mb={4}>
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
              {userActivityData && (
                <Line
                  options={chartOptions}
                  data={createLineChartData(userActivityData, "Unique Users")}
                />
              )}
            </ChartPaper>
          </Grid>
          <Grid item xs={12} md={6}>
            <ChartPaper elevation={3}>
              <Typography variant="h6" gutterBottom>
                Chat Interactions per Day
              </Typography>
              {chatData && (
                <Line
                  options={chartOptions}
                  data={createLineChartData(chatData, "Chat Interactions")}
                />
              )}
            </ChartPaper>
          </Grid>
        </Grid>
      </Box>

      <Divider sx={{ my: 4 }} />

      <Box mb={4}>
        <SectionTitle
          title="Cost Analysis"
          helpText="Breakdown of costs for different currencies and usage of LLM models"
        />
        <Grid container spacing={3}>
          <Grid item xs={12}>
            <ChartPaper elevation={3}>
              <Typography variant="h6" gutterBottom>
                Cost Analysis by Currency
              </Typography>
              {Object.keys(costData).length > 0 && (
                <Line
                  options={chartOptions}
                  data={createMultiLineChartData(costData)}
                />
              )}
            </ChartPaper>
          </Grid>
          <Grid item xs={12}>
            <Paper elevation={3} style={{ padding: "20px" }}>
              <Typography variant="h6" gutterBottom>
                Total Cost per Vendor and Model
              </Typography>
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
                            <Box sx={{ display: "flex", alignItems: "center" }}>
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
                          <StyledTableCell>{row.currency}</StyledTableCell>
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
            </Paper>
          </Grid>
        </Grid>
      </Box>

      <Divider sx={{ my: 4 }} />

      <Box mb={4}>
        <SectionTitle
          title="Model and Tool Usage"
          helpText="Analysis of most used LLM models and tools"
        />
        <Grid container spacing={3}>
          <Grid item xs={12} md={6}>
            <ChartPaper elevation={3}>
              <Typography variant="h6" gutterBottom>
                Most Used LLM Models
              </Typography>
              {llmModelData && (
                <Bar
                  options={chartOptions}
                  data={createBarChartData(llmModelData, "LLM Models")}
                />
              )}
            </ChartPaper>
          </Grid>
          <Grid item xs={12} md={6}>
            <ChartPaper elevation={3}>
              <Typography variant="h6" gutterBottom>
                Tool Usage Statistics
              </Typography>
              {toolUsageData && (
                <Bar
                  options={chartOptions}
                  data={createBarChartData(toolUsageData, "Tool Usage")}
                />
              )}
            </ChartPaper>
          </Grid>
        </Grid>
      </Box>
    </div>
  );
};

export default Dashboard;
