import React from "react";
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Grid,
} from "@mui/material";
import { StyledTableHeaderCell, StyledTableCell, StyledTableRow, StyledPaper } from "../../styles/sharedStyles";
import { MemoizedLineChart, MemoizedBarChart } from "../common/MemoizedChart";

const ErrorsTab = ({ data }) => {
  if (!data) {
    return (
      <Box sx={{ p: 3, textAlign: "center" }}>
        <Typography color="text.secondary">No error data available.</Typography>
      </Box>
    );
  }

  const timelineChartData = {
    labels: data.timeline?.map((t) => t.date) || [],
    datasets: [
      {
        label: "5xx Errors",
        data: data.timeline?.map((t) => t.count) || [],
        borderColor: "rgb(255, 99, 132)",
        backgroundColor: "rgba(255, 99, 132, 0.5)",
        tension: 0.1,
      },
    ],
  };

  const vendorLabels = Object.keys(data.by_vendor || {});
  const vendorData = Object.values(data.by_vendor || {});

  const vendorChartData = {
    labels: vendorLabels,
    datasets: [
      {
        label: "Errors by Vendor",
        data: vendorData,
        backgroundColor: [
          "rgba(255, 99, 132, 0.6)",
          "rgba(54, 162, 235, 0.6)",
          "rgba(255, 206, 86, 0.6)",
          "rgba(75, 192, 192, 0.6)",
          "rgba(153, 102, 255, 0.6)",
        ],
      },
    ],
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

  return (
    <Box>
      {/* Summary Stats */}
      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Grid item xs={6}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4" color="error.main">
              {data.total_5xx || 0}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Total 5xx Errors
            </Typography>
          </StyledPaper>
        </Grid>
        <Grid item xs={6}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography
              variant="h4"
              color={data.error_rate > 5 ? "error.main" : data.error_rate > 1 ? "warning.main" : "success.main"}
            >
              {data.error_rate?.toFixed(2) || 0}%
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Error Rate
            </Typography>
          </StyledPaper>
        </Grid>
      </Grid>

      <Grid container spacing={3}>
        {/* Timeline Chart */}
        <Grid item xs={12} md={8}>
          <StyledPaper sx={{ p: 2, height: 350 }}>
            <Typography variant="subtitle2" gutterBottom>
              Errors Over Time
            </Typography>
            {data.timeline && data.timeline.length > 0 ? (
              <MemoizedLineChart options={chartOptions} data={timelineChartData} />
            ) : (
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", height: "80%" }}>
                <Typography color="text.secondary">No timeline data available</Typography>
              </Box>
            )}
          </StyledPaper>
        </Grid>

        {/* Vendor Breakdown Chart */}
        <Grid item xs={12} md={4}>
          <StyledPaper sx={{ p: 2, height: 350 }}>
            <Typography variant="subtitle2" gutterBottom>
              Errors by Vendor
            </Typography>
            {vendorLabels.length > 0 ? (
              <MemoizedBarChart options={chartOptions} data={vendorChartData} />
            ) : (
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", height: "80%" }}>
                <Typography color="text.secondary">No vendor data available</Typography>
              </Box>
            )}
          </StyledPaper>
        </Grid>
      </Grid>

      {/* Vendor Table */}
      <Box sx={{ mt: 3 }}>
        <Typography variant="subtitle2" gutterBottom>
          Errors by Vendor
        </Typography>
        <TableContainer>
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>Vendor</StyledTableHeaderCell>
                <StyledTableHeaderCell align="right">Error Count</StyledTableHeaderCell>
                <StyledTableHeaderCell align="right">% of Total</StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {vendorLabels.length > 0 ? (
                vendorLabels.map((vendor, idx) => {
                  const count = data.by_vendor[vendor];
                  const percentage = data.total_5xx > 0 ? (count / data.total_5xx) * 100 : 0;
                  return (
                    <StyledTableRow key={vendor}>
                      <StyledTableCell>
                        <Typography fontWeight="medium">{vendor}</Typography>
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <Typography fontWeight="bold" color="error.main">
                          {count}
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <Typography>{percentage.toFixed(1)}%</Typography>
                      </StyledTableCell>
                    </StyledTableRow>
                  );
                })
              ) : (
                <StyledTableRow>
                  <StyledTableCell colSpan={3} align="center">
                    <Typography color="text.secondary">No errors recorded</Typography>
                  </StyledTableCell>
                </StyledTableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
      </Box>
    </Box>
  );
};

export default ErrorsTab;
