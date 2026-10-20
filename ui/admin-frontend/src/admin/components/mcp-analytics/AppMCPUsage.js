import React, { useEffect, useState } from "react";
import { Alert, Box, CircularProgress, Link, Table, TableBody, TableHead, TableRow, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import { StyledTableCell, StyledTableHeaderCell, StyledTableRow } from "../../styles/sharedStyles";
import Section from "../common/Section";
import { formatTime, apiErrorDetail } from "../../pages/webhookShared";
import MCPAnalyticsEmptyState from "./MCPAnalyticsEmptyState";
import { fetchAppServers, formatCount, formatMs, formatPercent } from "./mcpAnalytics";

/**
 * AppMCPUsage lists, per MCP server, the calls an App made through Tyk in a
 * period. The admin App page and the portal App page both render it, each
 * with its own client and path: the portal path only answers for Apps the
 * caller owns, so the portal never reaches an admin endpoint. The response
 * has a row per MCP server the App called in the period. boundServers are
 * the App's own MCP servers (app.attributes.mcp_servers): bound but silent
 * shows the Pump empty state; no calls and nothing bound shows nothing.
 */
const AppMCPUsage = ({ client, path, startDate, endDate, boundServers = [], admin = false }) => {
  const [rows, setRows] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;
    setError(null);
    fetchAppServers(client, path, { startDate, endDate })
      .then((r) => !cancelled && setRows(r))
      .catch((err) => {
        if (cancelled) return;
        setRows([]);
        // 404: no MCP analytics on this edition or for this App; no section.
        if (err?.response?.status !== 404) setError(apiErrorDetail(err, "Failed to load MCP usage"));
      });
    return () => {
      cancelled = true;
    };
  }, [client, path, startDate, endDate]);

  if (rows !== null && !error && rows.length === 0 && boundServers.length === 0) return null;

  return (
    <Section
      title="MCP usage"
      description="Calls this App made to MCP servers through Tyk, per server."
      data-testid="app-mcp-usage-section"
    >
      <AppMCPUsageBody rows={rows} error={error} admin={admin} connectionId={boundServers[0]?.connection_id} />
    </Section>
  );
};

const AppMCPUsageBody = ({ rows, error, admin, connectionId }) => {
  if (rows === null) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", p: 2 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }
  if (error) {
    return (
      <Alert severity="error" data-testid="app-mcp-usage-error">
        {error}
      </Alert>
    );
  }
  if (rows.every((r) => r.calls === 0)) {
    return <MCPAnalyticsEmptyState compact showSetupLink={admin} connectionId={rows[0]?.connectionId ?? connectionId} />;
  }

  return (
    <Table size="small" data-testid="app-mcp-usage" aria-label="MCP usage per server">
      <TableHead>
        <TableRow>
          <StyledTableHeaderCell>MCP server</StyledTableHeaderCell>
          <StyledTableHeaderCell align="right">Calls</StyledTableHeaderCell>
          <StyledTableHeaderCell align="right">Error rate</StyledTableHeaderCell>
          <StyledTableHeaderCell align="right">p95 latency</StyledTableHeaderCell>
          <StyledTableHeaderCell>Last call</StyledTableHeaderCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {rows.map((r) => (
          <StyledTableRow key={r.id} data-testid={`app-mcp-usage-row-${r.id}`}>
            <StyledTableCell>
              {admin ? (
                <Link component={RouterLink} to={`/admin/mcp-servers/${r.serverId}`}>
                  {r.name}
                </Link>
              ) : (
                <Typography variant="body2">{r.name}</Typography>
              )}
            </StyledTableCell>
            <StyledTableCell align="right">{formatCount(r.calls)}</StyledTableCell>
            <StyledTableCell align="right">{formatPercent(r.errorRate)}</StyledTableCell>
            <StyledTableCell align="right">{formatMs(r.p95Ms)}</StyledTableCell>
            <StyledTableCell>{r.lastSeenAt ? formatTime(r.lastSeenAt) : "Never"}</StyledTableCell>
          </StyledTableRow>
        ))}
      </TableBody>
    </Table>
  );
};

export default AppMCPUsage;
