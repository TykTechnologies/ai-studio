import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
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
// ReportDialog reports a block as a false positive: a proposed guardrail
// exception for this app that an administrator reviews. Owners cannot
// approve it.
const ReportDialog = ({ client, appId, block, onClose, onReported }) => {
  const [why, setWhy] = useState("");
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    setWhy("");
    setError(null);
  }, [block]);
  if (!block) return null;
  const send = async () => {
    setSaving(true);
    setError(null);
    try {
      await client.post(`/common/apps/${appId}/guardrail-blocks/${block.id}/report`, { justification: why.trim() });
      onReported(block);
    } catch (e) {
      setError(e?.response?.data?.errors?.[0]?.detail || "Could not send the report");
    } finally {
      setSaving(false);
    }
  };
  return (
    <Dialog open onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Report a false positive</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          Tell an administrator why this request was legitimate. They can let the guardrail log this kind of finding
          for your app instead of stopping it. Nothing changes until they approve.
        </Typography>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <TextField
          fullWidth
          required
          label="Why is this not a threat?"
          value={why}
          onChange={(e) => setWhy(e.target.value)}
          inputProps={{ maxLength: 450, "data-testid": "report-justification" }}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="contained" onClick={send} disabled={!why.trim() || saving} data-testid="report-send">
          Send report
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const AppGuardrailBlocks = ({ client, appId, days = 7 }) => {
  const [rows, setRows] = useState(null);
  const [reporting, setReporting] = useState(null);
  const [reported, setReported] = useState({});

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
        Requests of this app that a guardrail stopped or changed. If a block is wrong, report it as a false positive
        (or quote the request ID to an administrator).
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
              <TableCell />
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
                <TableCell sx={{ whiteSpace: "nowrap" }}>
                  {reported[r.id] ? (
                    <Typography variant="caption" color="text.secondary" data-testid="report-sent">
                      Reported
                    </Typography>
                  ) : (
                    r.reportable && (
                      <Button size="small" onClick={() => setReporting(r)} data-testid="report-false-positive">
                        Report false positive
                      </Button>
                    )
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Box>
      <ReportDialog
        client={client}
        appId={appId}
        block={reporting}
        onClose={() => setReporting(null)}
        onReported={(b) => {
          setReporting(null);
          setReported((prev) => ({ ...prev, [b.id]: true }));
        }}
      />
    </Paper>
  );
};

export default AppGuardrailBlocks;
