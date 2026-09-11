import React from "react";
import { Alert, Box, Chip, Typography } from "@mui/material";

export const TARGET_STATUSES = ["pending", "approved", "rejected", "revoked"];
export const DELIVERY_STATUSES = [
  "queued",
  "in_flight",
  "retrying",
  "succeeded",
  "dead_lettered",
  "cancelled",
];

export const targetStatusColor = (status) => {
  switch (status) {
    case "approved":
      return "success";
    case "pending":
      return "warning";
    case "rejected":
    case "revoked":
      return "error";
    default:
      return "default";
  }
};

export const deliveryStatusColor = (status) => {
  switch (status) {
    case "succeeded":
      return "success";
    case "queued":
    case "in_flight":
      return "info";
    case "retrying":
      return "warning";
    case "dead_lettered":
      return "error";
    default:
      return "default";
  }
};

export const formatTime = (iso) => {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
};

export const apiErrorDetail = (err, fallback) =>
  err?.response?.data?.errors?.[0]?.detail || err?.message || fallback;

export const humanStatus = (s) => (s || "").replace(/_/g, " ");

// StatusChip renders a target or delivery status with a consistent colour.
export const StatusChip = ({ status, kind = "delivery" }) => (
  <Chip
    label={humanStatus(status)}
    size="small"
    color={kind === "target" ? targetStatusColor(status) : deliveryStatusColor(status)}
    data-testid={`status-${status}`}
  />
);

// HealthPill summarises the feature status returned by /webhooks/status.
export const HealthPill = ({ status }) => {
  if (!status?.enabled) return <Chip label="Disabled" size="small" color="default" />;
  if (!status.bus_connected) return <Chip label="Event bus not connected" size="small" color="error" />;
  if (status.dropped_events > 0) return <Chip label={`${status.dropped_events} events dropped`} size="small" color="warning" />;
  if (!status.worker_enabled) return <Chip label="Ingesting only (no worker)" size="small" color="warning" />;
  return <Chip label="Healthy" size="small" color="success" />;
};

// EnterpriseUpsell is shown when the API reports the feature is unavailable.
export const EnterpriseUpsell = () => (
  <Box sx={{ p: 3 }}>
    <Alert severity="info">
      <Typography variant="h6" gutterBottom>
        Enterprise Feature
      </Typography>
      <Typography>
        Webhooks are an Enterprise Edition feature that push AI Studio events (LLM, app, user,
        tool and datasource changes and more) to your own systems:
      </Typography>
      <ul>
        <li>Administrator approval of every target URL before anything is sent</li>
        <li>Per-target payload templates with automatic redaction of secrets</li>
        <li>Signed deliveries, retries with backoff, dead-letter queue and replay</li>
        <li>A searchable log of every delivery attempt, exportable to CSV or JSON</li>
        <li>Approvals and dead letters recorded in the audit trail</li>
      </ul>
      <Typography sx={{ mt: 2 }}>
        Visit{" "}
        <a href="https://tyk.io/ai-studio/pricing" target="_blank" rel="noopener noreferrer">
          tyk.io/ai-studio/pricing
        </a>{" "}
        for more information.
      </Typography>
    </Alert>
  </Box>
);

export const DisabledNotice = () => (
  <Box sx={{ p: 3 }}>
    <Alert severity="warning">
      Webhooks are switched off. Set <code>WEBHOOKS_ENABLED=true</code> on the AI Studio server to
      enable them.
    </Alert>
  </Box>
);

export const BusWarning = ({ status }) =>
  status?.enabled && !status.bus_connected ? (
    <Alert severity="warning" sx={{ mb: 2 }}>
      This node is not connected to the event bus, so it will not ingest events. Targets can still be
      managed here and other nodes keep delivering.
    </Alert>
  ) : null;

export const preStyle = {
  m: 0,
  fontSize: "0.75rem",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  maxHeight: 320,
  overflow: "auto",
  p: 1,
  bgcolor: "background.paper",
};
