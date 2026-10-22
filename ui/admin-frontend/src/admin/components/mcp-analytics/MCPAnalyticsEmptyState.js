import React from "react";
import { Box, Link, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";

// connectionAnalyticsPath is the Analytics section of a Tyk connection,
// where the ingest token and the Pump configuration live.
export const connectionAnalyticsPath = (connectionId) =>
  connectionId ? `/admin/tyk-connections/edit/${connectionId}#analytics` : "/admin/tyk-connections";

/**
 * MCPAnalyticsEmptyState explains why there is no MCP data: Tyk Gateways
 * record MCP calls, and Tyk Pump has to send them to AI Studio. Admin views
 * link to the connection's Analytics section; the portal passes no link.
 */
const MCPAnalyticsEmptyState = ({ connectionId, showSetupLink = true, compact = false }) => (
  <Box
    data-testid="mcp-analytics-empty"
    sx={{
      p: compact ? 1.5 : 3,
      border: (theme) => `1px dashed ${theme.palette.border?.neutralDefault || theme.palette.divider}`,
      borderRadius: 1,
      textAlign: compact ? "left" : "center",
    }}
  >
    <Typography variant="bodyLargeBold" component="p" sx={{ mb: 0.5 }}>
      No MCP calls recorded in this period
    </Typography>
    <Typography variant="body2" color="text.secondary">
      MCP analytics come from the Tyk Gateways that proxy these servers. Tyk Pump sends them to AI Studio with
      the connection's ingest token; the Gateways need Tyk 5.14 or later with analytics enabled.
      {showSetupLink && (
        <>
          {" "}
          <Link component={RouterLink} to={connectionAnalyticsPath(connectionId)} data-testid="mcp-analytics-setup-link">
            Set up analytics on the Tyk connection
          </Link>
          .
        </>
      )}
    </Typography>
  </Box>
);

export default MCPAnalyticsEmptyState;
