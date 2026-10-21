import React, { useEffect, useState } from "react";
import {
  Box,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";

const ACTION_LABELS = {
  block: "Blocked",
  quarantine: "Content replaced",
  remove: "Tool removed",
  redact: "Redacted",
};

/**
 * The requests of this App that guardrails acted on, for its owner: when,
 * what happened, why (the reason the caller got) and the request id to quote
 * to an administrator. Only filters an administrator shares with App owners
 * appear (evidence.share_with_app_owners); nothing renders when there are
 * none, so the section stays out of the way for apps without guardrails.
 */
const AppGuardrailBlocks = ({ client, appId, days = 7 }) => {
  const [rows, setRows] = useState(null);

  useEffect(() => {
    let cancelled = false;
    client
      .get(`/common/apps/${appId}/guardrail-blocks`, { params: { days } })
      .then((resp) => {
        if (!cancelled) setRows(Array.isArray(resp.data?.data) ? resp.data.data : []);
      })
      .catch(() => {
        if (!cancelled) setRows([]);
      });
    return () => {
      cancelled = true;
    };
  }, [client, appId, days]);

  if (!rows || rows.length === 0) return null;
  return (
    <Paper sx={{ p: 3, mb: 3 }} data-testid="app-guardrail-blocks">
      <Typography variant="h6" gutterBottom>
        Guardrail blocks (last {days} days)
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Requests of this app that a guardrail stopped or changed. Quote the request ID to an administrator if you
        think a block is wrong.
      </Typography>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Time</TableCell>
              <TableCell>What happened</TableCell>
              <TableCell>Why</TableCell>
              <TableCell>Filter</TableCell>
              <TableCell>Request ID</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((r, i) => (
              <TableRow key={`${r.request_id || ""}-${r.timestamp}-${i}`} data-testid="app-guardrail-block">
                <TableCell sx={{ whiteSpace: "nowrap" }}>{new Date(r.timestamp).toLocaleString()}</TableCell>
                <TableCell>{ACTION_LABELS[r.action] || r.action || "-"}</TableCell>
                <TableCell>
                  {r.reason}
                  {r.reason_code && (
                    <Typography variant="caption" color="text.secondary" component="div" sx={{ fontFamily: "monospace" }}>
                      {r.reason_code}
                    </Typography>
                  )}
                </TableCell>
                <TableCell>{r.filter}</TableCell>
                <TableCell sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>{r.request_id || "-"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Box>
    </Paper>
  );
};

export default AppGuardrailBlocks;
