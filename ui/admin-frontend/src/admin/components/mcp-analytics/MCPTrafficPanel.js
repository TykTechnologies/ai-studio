import React, { useEffect, useState } from "react";
import { Alert, Box, CircularProgress, Grid, Link, Table, TableBody, TableHead, TableRow, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import { StyledTableCell, StyledTableHeaderCell, StyledTableRow } from "../../styles/sharedStyles";
import { apiErrorDetail } from "../../pages/webhookShared";
import MCPAnalyticsEmptyState from "./MCPAnalyticsEmptyState";
import { StatTile } from "./MCPServerUsage";
import { fetchOverview, formatCount, formatPercent } from "./mcpAnalytics";

/**
 * MCPTrafficPanel is the Dashboard's MCP traffic summary: calls through Tyk
 * to MCP servers in the Dashboard's period, the error rate and the busiest
 * tools.
 */
const MCPTrafficPanel = ({ startDate, endDate }) => {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;
    setError(null);
    fetchOverview(apiClient, { startDate, endDate })
      .then((d) => !cancelled && setData(d))
      .catch((err) => !cancelled && setError(apiErrorDetail(err, "Failed to load MCP traffic")));
    return () => {
      cancelled = true;
    };
  }, [startDate, endDate]);

  if (error) {
    return (
      <Alert severity="error" data-testid="mcp-traffic-error">
        {error}
      </Alert>
    );
  }
  if (!data) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", p: 2 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }
  if (data.calls === 0) return <MCPAnalyticsEmptyState compact />;

  return (
    <Box data-testid="mcp-traffic">
      <Grid container spacing={2} sx={{ mb: 2 }}>
        <StatTile label="Calls" value={formatCount(data.calls)} testId="mcp-traffic-calls" />
        <StatTile label="Errors" value={formatCount(data.errors)} testId="mcp-traffic-errors" />
        <StatTile label="Error rate" value={formatPercent(data.errorRate)} testId="mcp-traffic-error-rate" />
      </Grid>
      {data.topTools.length > 0 && (
        <>
          <Typography variant="bodyMediumSemiBold" component="h3" sx={{ mb: 1 }}>
            Top tools
          </Typography>
          <Table size="small" aria-label="Top MCP tools">
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>Tool</StyledTableHeaderCell>
                <StyledTableHeaderCell>MCP server</StyledTableHeaderCell>
                <StyledTableHeaderCell align="right">Calls</StyledTableHeaderCell>
                <StyledTableHeaderCell align="right">Error rate</StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {data.topTools.map((t) => (
                <StyledTableRow key={t.id} data-testid="mcp-traffic-tool">
                  <StyledTableCell>{t.name}</StyledTableCell>
                  <StyledTableCell>
                    <Link component={RouterLink} to={`/admin/mcp-servers/${t.serverId}`}>
                      {t.serverName}
                    </Link>
                  </StyledTableCell>
                  <StyledTableCell align="right">{formatCount(t.calls)}</StyledTableCell>
                  <StyledTableCell align="right">{formatPercent(t.errorRate)}</StyledTableCell>
                </StyledTableRow>
              ))}
            </TableBody>
          </Table>
        </>
      )}
    </Box>
  );
};

export default MCPTrafficPanel;
