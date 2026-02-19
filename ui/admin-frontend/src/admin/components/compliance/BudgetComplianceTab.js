import React from "react";
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Chip,
  Link,
  Grid,
  LinearProgress,
} from "@mui/material";
import { StyledTableHeaderCell, StyledTableCell, StyledTableRow, StyledPaper } from "../../styles/sharedStyles";
import { Link as RouterLink } from "react-router-dom";

const BudgetComplianceTab = ({ data }) => {
  if (!data) {
    return (
      <Box sx={{ p: 3, textAlign: "center" }}>
        <Typography color="text.secondary">No budget compliance data available.</Typography>
      </Box>
    );
  }

  const getProgressColor = (percentage) => {
    if (percentage >= 95) return "error";
    if (percentage >= 80) return "warning";
    return "success";
  };

  const getAlertChipColor = (level) => {
    return level === "critical" ? "error" : "warning";
  };

  return (
    <Box>
      {/* Summary Stats */}
      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Grid item xs={4}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4" color="error.main">
              {data.critical_count || 0}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Critical (&gt;95%)
            </Typography>
          </StyledPaper>
        </Grid>
        <Grid item xs={4}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4" color="warning.main">
              {data.warning_count || 0}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Warning (&gt;80%)
            </Typography>
          </StyledPaper>
        </Grid>
        <Grid item xs={4}>
          <StyledPaper sx={{ p: 2, textAlign: "center" }}>
            <Typography variant="h4">
              {data.alerts?.length || 0}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Total Alerts
            </Typography>
          </StyledPaper>
        </Grid>
      </Grid>

      {/* Budget Alerts Table */}
      <Typography variant="subtitle2" gutterBottom>
        Budget Alerts
      </Typography>
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <StyledTableHeaderCell>Entity</StyledTableHeaderCell>
              <StyledTableHeaderCell>Type</StyledTableHeaderCell>
              <StyledTableHeaderCell align="center">Alert Level</StyledTableHeaderCell>
              <StyledTableHeaderCell>Budget Usage</StyledTableHeaderCell>
              <StyledTableHeaderCell align="right">Spent / Budget</StyledTableHeaderCell>
              <StyledTableHeaderCell align="right">Daily Velocity</StyledTableHeaderCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {data.alerts && data.alerts.length > 0 ? (
              data.alerts.map((alert, idx) => (
                <StyledTableRow key={idx}>
                  <StyledTableCell>
                    {alert.entity_type === "App" ? (
                      <Link
                        component={RouterLink}
                        to={`/admin/apps/${alert.entity_id}`}
                      >
                        {alert.name}
                      </Link>
                    ) : (
                      <Link
                        component={RouterLink}
                        to={`/admin/llms/${alert.entity_id}`}
                      >
                        {alert.name}
                      </Link>
                    )}
                    {alert.owner_email && (
                      <Typography variant="caption" display="block" color="text.secondary">
                        Owner: {alert.owner_email}
                      </Typography>
                    )}
                  </StyledTableCell>
                  <StyledTableCell>
                    <Chip
                      label={alert.entity_type}
                      size="small"
                      variant="outlined"
                    />
                  </StyledTableCell>
                  <StyledTableCell align="center">
                    <Chip
                      label={alert.alert_level}
                      color={getAlertChipColor(alert.alert_level)}
                      size="small"
                    />
                  </StyledTableCell>
                  <StyledTableCell>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <LinearProgress
                        variant="determinate"
                        value={Math.min(alert.percentage, 100)}
                        color={getProgressColor(alert.percentage)}
                        sx={{ flexGrow: 1, height: 8, borderRadius: 1 }}
                      />
                      <Typography variant="body2" sx={{ minWidth: 50 }}>
                        {alert.percentage.toFixed(1)}%
                      </Typography>
                    </Box>
                  </StyledTableCell>
                  <StyledTableCell align="right">
                    <Typography>
                      ${alert.spent.toFixed(2)} / ${alert.budget.toFixed(2)}
                    </Typography>
                  </StyledTableCell>
                  <StyledTableCell align="right">
                    <Typography color={alert.velocity > (alert.budget / 30) ? "error.main" : "text.primary"}>
                      ${alert.velocity.toFixed(2)}/day
                    </Typography>
                  </StyledTableCell>
                </StyledTableRow>
              ))
            ) : (
              <StyledTableRow>
                <StyledTableCell colSpan={6} align="center">
                  <Typography color="text.secondary">
                    No budget alerts - all entities are within budget limits
                  </Typography>
                </StyledTableCell>
              </StyledTableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default BudgetComplianceTab;
