import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Table,
  TableBody,
  TableCell,
  TableRow,
  Typography,
} from "@mui/material";
import agentTracingService from "../../services/agentTracingService";
import { CONTENT_MODES } from "../common/TracingSettingsSection";

// The install-wide content capture settings (jyx.172): a ceiling nothing
// may exceed, and the default for Apps without their own mode. They are
// set by environment variables in this release, so the dialog shows them
// read-only from GET /agent-tracing/config.

const modeLabel = (mode) => CONTENT_MODES.find((m) => m.value === mode)?.label || mode || "Not set";

const ROWS = [
  {
    field: "content_ceiling",
    label: "Ceiling",
    env: "AGENT_TRACING_CONTENT_CEILING",
    help: "The most any App, request header or default may capture.",
  },
  {
    field: "content_default_mode",
    label: "Default",
    env: "AGENT_TRACING_CONTENT_DEFAULT_MODE",
    help: "What Apps without their own content mode capture.",
  },
];

const CaptureSettingsDialog = ({ open, onClose }) => {
  const [config, setConfig] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return undefined;
    let live = true;
    setConfig(null);
    setError("");
    agentTracingService
      .getConfig()
      .then((c) => live && setConfig(c || {}))
      .catch(() => live && setError("The capture settings could not be loaded."));
    return () => {
      live = false;
    };
  }, [open]);

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth aria-labelledby="capture-settings-title">
      <DialogTitle id="capture-settings-title">Content capture</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" paragraph>
          An App can set its own content mode up to the ceiling; Apps that do not use the default. The default is never
          above the ceiling.
        </Typography>
        {error && <Alert severity="error">{error}</Alert>}
        {!config && !error && (
          <Box sx={{ display: "flex", justifyContent: "center", py: 3 }}>
            <CircularProgress size={24} aria-label="Loading capture settings" />
          </Box>
        )}
        {config && (
          <Table size="small" data-testid="capture-settings">
            <TableBody>
              {ROWS.map((r) => (
                <TableRow key={r.field}>
                  <TableCell sx={{ verticalAlign: "top", whiteSpace: "nowrap" }}>{r.label}</TableCell>
                  <TableCell>
                    <Typography variant="body2" data-testid={`capture-${r.field}`}>
                      {modeLabel(config[r.field])}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" display="block">
                      {r.help} Set with <code>{r.env}</code>.
                    </Typography>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 2 }}>
          These settings come from the environment variables named above and change when AI Studio restarts with new
          values.
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
};

export default CaptureSettingsDialog;
