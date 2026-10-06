import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from "@mui/material";
import apiClient from "../utils/apiClient";
import { SecondaryOutlineButton } from "../styles/sharedStyles";
import { formatTime, apiErrorDetail } from "./webhookShared";

const STATE_LABELS = {
  in_sync: "In sync",
  pending: "Pending",
  unreachable: "Unreachable",
  gone: "Gone",
};

const stateColor = (state) => {
  switch (state) {
    case "in_sync":
      return "success";
    case "pending":
      return "warning";
    case "unreachable":
      return "error";
    default:
      return "default";
  }
};

/**
 * TykGatewayNodes lists the nodes of a Tyk Gateway connection: an OSS
 * Gateway keeps its MCP proxies on each node's own disk, so AI Studio writes
 * the proxies it owns to every node it finds and shows here which nodes
 * serve them. Nodes are refreshed by every sync.
 */
const TykGatewayNodes = ({ connectionId, refreshKey }) => {
  const [nodes, setNodes] = useState(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await apiClient.get(`/tyk-connections/${connectionId}/nodes`);
      setNodes(res.data || []);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load the gateway nodes"));
    } finally {
      setLoading(false);
    }
  }, [connectionId]);

  useEffect(() => {
    load();
  }, [load, refreshKey]);

  if (error) {
    return (
      <Alert severity="error" data-testid="gateway-nodes-error">
        {error}
      </Alert>
    );
  }
  if (nodes === null) {
    return <CircularProgress size={24} />;
  }
  if (nodes.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary" data-testid="gateway-nodes-empty">
        No nodes seen yet. They are discovered by the first sync after activation.
      </Typography>
    );
  }
  return (
    <Box data-testid="gateway-nodes">
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Node</TableCell>
            <TableCell>State</TableCell>
            <TableCell>Version</TableCell>
            <TableCell>AI Studio proxies</TableCell>
            <TableCell>All MCP proxies</TableCell>
            <TableCell>Last reconciled</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {nodes.map((n) => (
            <TableRow key={n.id} data-testid={`gateway-node-${n.id}`}>
              <TableCell>
                <Typography variant="body2">{n.address}</Typography>
                {n.source === "dns" && (
                  <Typography variant="caption" color="text.secondary">
                    via DNS
                  </Typography>
                )}
              </TableCell>
              <TableCell>
                <Tooltip title={n.last_error || ""}>
                  <Chip size="small" label={STATE_LABELS[n.state] || n.state} color={stateColor(n.state)} />
                </Tooltip>
              </TableCell>
              <TableCell>{n.version || "–"}</TableCell>
              <TableCell>
                {n.studio_count}/{n.expected_count}
              </TableCell>
              <TableCell>{n.mcp_count}</TableCell>
              <TableCell>{n.last_reconcile_at ? formatTime(n.last_reconcile_at) : "–"}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <Box sx={{ mt: 1 }}>
        <SecondaryOutlineButton size="small" onClick={load} disabled={loading} data-testid="gateway-nodes-refresh">
          {loading ? "Loading…" : "Refresh"}
        </SecondaryOutlineButton>
      </Box>
    </Box>
  );
};

export default TykGatewayNodes;
