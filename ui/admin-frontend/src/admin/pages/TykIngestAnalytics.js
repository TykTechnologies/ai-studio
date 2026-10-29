import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  CircularProgress,
  Grid,
  IconButton,
  Tooltip,
  Typography,
} from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import apiClient from "../utils/apiClient";
import { getConfig } from "../../config";
import { copyToClipboard } from "../utils/clipboardUtils";
import ConfirmationDialog from "../components/common/ConfirmationDialog";
import { DangerOutlineButton, SecondaryOutlineButton } from "../styles/sharedStyles";
import { formatTime, apiErrorDetail } from "./webhookShared";

export const INGEST_PATH = "/api/v1/ingest/tyk-analytics";
export const TOKEN_PLACEHOLDER = "<INGEST_TOKEN>";
// A token that has seen nothing for this long gets a warning: the Pump is
// either not configured or cannot reach AI Studio.
export const NO_DATA_WARNING_MS = 60 * 60 * 1000;

// ingestURL is where the Pump sends batches: the API host the UI was served
// with, or the page origin, plus the path the status reports.
export const ingestURL = (path) => {
  const host = (getConfig().api_host || window.location.origin || "").replace(/\/+$/, "");
  return `${host}${path || INGEST_PATH}`;
};

// httpPumpSnippet is the pump.conf entry for the tyk-pump http pump (option
// names from the upstream pump). The token is the one the admin has just
// generated, or a placeholder; nothing else secret is ever embedded.
// dont_purge_uptime_data: without an uptime store Tyk Pump otherwise fails
// at start looking for MongoDB. max_retries 8 keeps a batch retried for up
// to the pump timeout (30 s) when AI Studio is briefly down; Pump has
// already drained Redis, so a batch it gives up on is lost.
export const httpPumpSnippet = (url, token) =>
  JSON.stringify(
    {
      dont_purge_uptime_data: true,
      pumps: {
        http: {
          type: "http",
          timeout: 30,
          omit_detailed_recording: true,
          meta: {
            url,
            headers: { Authorization: `Bearer ${token || TOKEN_PLACEHOLDER}` },
            mcp_only: true,
            gzip: true,
            batch_size: 500,
            max_retries: 8,
          },
        },
      },
    },
    null,
    2,
  );

export const stdoutPumpSnippet = () =>
  JSON.stringify(
    {
      pumps: {
        stdout: {
          type: "stdout",
          omit_detailed_recording: true,
          meta: { format: "json", log_field_name: "tyk-analytics-record" },
        },
      },
    },
    null,
    2,
  );

// vectorSnippet mirrors the Vector config tested end to end in
// tests/tykmcp/oss/vector.toml. URL and token come from the environment, so
// the file itself never holds the token.
export const vectorSnippet = (url) => `# Vector bridge from the Tyk Pump stdout pump to AI Studio's MCP analytics
# ingest endpoint. Works with any Tyk Pump 1.16+, until the http pump ships.
#
# Environment:
#   MCP_INGEST_URL    ${url}
#   MCP_INGEST_TOKEN  the Tyk connection's ingest token

# Pump's container output. On Kubernetes use a kubernetes_logs source with an
# extra_label_selector for the Pump pods instead.
[sources.pump]
type = "docker_logs"
include_labels = ["com.docker.compose.service=pump"]

# Keep only analytics records of MCP traffic. Pump's own log lines (and
# anything else that is not a JSON record) are dropped.
[transforms.mcp_records]
type = "remap"
inputs = ["pump"]
drop_on_error = true
drop_on_abort = true
source = '''
parsed = object!(parse_json!(string!(.message)))
record = object!(parsed."tyk-analytics-record")
if record.mcp_stats.is_mcp != true { abort }
. = record
'''

# A JSON array of records per request, bearer token, gzip.
[sinks.studio]
type = "http"
inputs = ["mcp_records"]
uri = "\${MCP_INGEST_URL}"
method = "post"
compression = "gzip"
encoding.codec = "json"
auth.strategy = "bearer"
auth.token = "\${MCP_INGEST_TOKEN}"
batch.max_events = 500
batch.timeout_secs = 5
request.retry_attempts = 5
`;

// noDataWarning: a token exists and no batch has arrived for an hour (or
// ever, once the token is an hour old).
export const noDataWarning = (status, now = Date.now()) => {
  if (!status?.has_token) return false;
  const last = status.last_batch_at ? new Date(status.last_batch_at).getTime() : null;
  if (last) return now - last > NO_DATA_WARNING_MS;
  const created = status.token_created_at ? new Date(status.token_created_at).getTime() : null;
  return !created || now - created > NO_DATA_WARNING_MS;
};

const Snippet = ({ title, text, onCopy, testId }) => (
  <Box sx={{ mb: 2 }}>
    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
      <Typography variant="bodyMediumSemiBold">{title}</Typography>
      <Tooltip title="Copy">
        <IconButton size="small" onClick={() => onCopy(text, title)} aria-label={`Copy ${title}`} data-testid={`${testId}-copy`}>
          <ContentCopyIcon fontSize="small" />
        </IconButton>
      </Tooltip>
    </Box>
    <Box
      component="pre"
      data-testid={testId}
      sx={{
        m: 0,
        mt: 0.5,
        p: 1.5,
        bgcolor: "background.surfaceNeutralDisabled",
        borderRadius: 1,
        fontSize: 12,
        overflowX: "auto",
        whiteSpace: "pre",
      }}
    >
      {text}
    </Box>
  </Box>
);

const Stat = ({ label, value, testId }) => (
  <Grid item xs={12} sm={4}>
    <Typography variant="body2" color="text.secondary">
      {label}
    </Typography>
    <Typography variant="bodyLargeBold" data-testid={testId}>
      {value}
    </Typography>
  </Grid>
);

/**
 * TykIngestAnalytics is the Analytics part of a Tyk connection: the ingest
 * token Tyk Pump authenticates with, what has arrived, and the Pump config to
 * send MCP analytics to AI Studio. The token's plaintext is returned once by
 * rotate and kept only in this component's state; the status endpoint never
 * returns it, so a reload cannot show it again.
 */
const TykIngestAnalytics = ({ connectionId, canWrite, notify }) => {
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [plaintext, setPlaintext] = useState(null);
  const [confirm, setConfirm] = useState(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const res = await apiClient.get(`/tyk-connections/${connectionId}/ingest-status`);
      setStatus(res.data || {});
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load the ingest status"));
    } finally {
      setLoading(false);
    }
  }, [connectionId]);

  useEffect(() => {
    load();
  }, [load]);

  const copy = (text, what) =>
    copyToClipboard(
      text,
      what,
      () => notify?.(`${what} copied`),
      () => notify?.(`Could not copy ${what.toLowerCase()}`, "error"),
    );

  const rotate = async () => {
    setConfirm(null);
    setBusy(true);
    setError(null);
    try {
      const res = await apiClient.post(`/tyk-connections/${connectionId}/ingest-token`, {});
      setPlaintext(res.data?.token || null);
      if (res.data?.status) setStatus(res.data.status);
      else await load();
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to generate the ingest token"));
    } finally {
      setBusy(false);
    }
  };

  const revoke = async () => {
    setConfirm(null);
    setBusy(true);
    setError(null);
    try {
      await apiClient.delete(`/tyk-connections/${connectionId}/ingest-token`);
      setPlaintext(null);
      await load();
      notify?.("Ingest token revoked");
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to revoke the ingest token"));
    } finally {
      setBusy(false);
    }
  };

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", p: 2 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  const hasToken = Boolean(status?.has_token);
  const url = ingestURL(status?.ingest_path);

  return (
    <Box data-testid="ingest-analytics">
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="ingest-error">
          {error}
        </Alert>
      )}

      <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2, flexWrap: "wrap" }}>
        <Typography variant="body2" sx={{ flex: 1, minWidth: 200 }} data-testid="ingest-token-state">
          {hasToken
            ? `An ingest token is set${status.token_prefix ? ` (${status.token_prefix}…)` : ""}${
                status.token_created_at ? `, created ${formatTime(status.token_created_at)}` : ""
              }.`
            : "No ingest token. Generate one, then configure Tyk Pump with it."}
        </Typography>
        {canWrite && !hasToken && (
          <SecondaryOutlineButton size="small" onClick={rotate} disabled={busy} data-testid="ingest-generate">
            Generate token
          </SecondaryOutlineButton>
        )}
        {canWrite && hasToken && (
          <>
            <SecondaryOutlineButton size="small" onClick={() => setConfirm("rotate")} disabled={busy} data-testid="ingest-rotate">
              Rotate token
            </SecondaryOutlineButton>
            <DangerOutlineButton size="small" onClick={() => setConfirm("revoke")} disabled={busy} data-testid="ingest-revoke">
              Revoke
            </DangerOutlineButton>
          </>
        )}
      </Box>

      {plaintext && (
        <Alert
          severity="warning"
          sx={{ mb: 2 }}
          onClose={() => setPlaintext(null)}
          data-testid="ingest-plaintext"
        >
          <Typography variant="body2" sx={{ mb: 1 }}>
            Copy this token now. It is shown only once and cannot be retrieved later.
          </Typography>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Box
              component="code"
              data-testid="ingest-plaintext-value"
              sx={{ fontFamily: "monospace", wordBreak: "break-all", fontSize: 13 }}
            >
              {plaintext}
            </Box>
            <IconButton size="small" onClick={() => copy(plaintext, "Token")} aria-label="Copy token" data-testid="ingest-plaintext-copy">
              <ContentCopyIcon fontSize="small" />
            </IconButton>
          </Box>
        </Alert>
      )}

      {hasToken && noDataWarning(status) && (
        <Alert severity="warning" sx={{ mb: 2 }} data-testid="ingest-no-data">
          {status.last_batch_at
            ? "No analytics have arrived for over an hour."
            : "No analytics have arrived since the token was created."}{" "}
          Check that Tyk Pump runs with the configuration below and can reach AI Studio, and that the Gateways have
          analytics enabled.
        </Alert>
      )}

      {hasToken && (
        <Grid container spacing={2} sx={{ mb: 3 }} data-testid="ingest-status">
          <Stat
            label="Last batch received"
            value={status.last_batch_at ? formatTime(status.last_batch_at) : "Never"}
            testId="ingest-last-batch"
          />
          <Stat label="MCP records, last 24 h" value={(status.records_24h ?? 0).toLocaleString()} testId="ingest-records-24h" />
          <Stat
            label="Records for unknown APIs, last 24 h"
            value={(status.unknown_api_24h ?? 0).toLocaleString()}
            testId="ingest-unknown-api"
          />
          <Stat
            label="MCP records without an App, last 24 h"
            value={(status.unattributed_24h ?? 0).toLocaleString()}
            testId="ingest-unattributed"
          />
          <Stat
            label="Non-MCP records dropped, last 24 h"
            value={(status.skipped_non_mcp_24h ?? 0).toLocaleString()}
            testId="ingest-skipped-non-mcp"
          />
        </Grid>
      )}

      {hasToken && status.unknown_api_24h > 0 && status.last_unknown_api_id && (
        <Alert severity="info" sx={{ mb: 2 }} data-testid="ingest-unknown-api-hint">
          Records arrive for Tyk APIs AI Studio does not know, most recently API {status.last_unknown_api_id}. Sync
          the connection if it is a new MCP proxy, or filter the Pump with mcp_only or filters.api_ids to send less.
        </Alert>
      )}

      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        Tyk Pump sends the Gateways' MCP analytics records to {url}. Use the http pump once your Pump release has
        it, or the stdout pump with Vector, which works with Tyk Pump 1.16 and later. Records for APIs AI Studio
        does not know are counted and dropped.
      </Typography>
      <Box component="ul" sx={{ mt: 0, mb: 2, pl: 3, typography: "body2", color: "text.secondary" }} data-testid="ingest-traps">
        <li>
          Tyk Pump stops at start unless <code>dont_purge_uptime_data</code> is true (env{" "}
          <code>TYK_PMP_DONTPURGEUPTIMEDATA=true</code>): its uptime pump defaults to MongoDB.
        </li>
        <li>
          An API only records analytics when its OAS sets{" "}
          <code>x-tyk-api-gateway.middleware.global.trafficLogs.enabled</code> to true. Proxies AI Studio registers
          set it; proxies created on the Tyk side may not.
        </li>
      </Box>
      <Snippet
        title="Tyk Pump: http pump (pending the upstream tyk-pump release)"
        text={httpPumpSnippet(url, plaintext)}
        onCopy={copy}
        testId="snippet-http-pump"
      />
      <Snippet title="Tyk Pump: stdout pump" text={stdoutPumpSnippet()} onCopy={copy} testId="snippet-stdout-pump" />
      <Snippet title="Vector (vector.toml)" text={vectorSnippet(url)} onCopy={copy} testId="snippet-vector" />

      <ConfirmationDialog
        open={confirm === "rotate"}
        title="Rotate the ingest token?"
        message="The current token stops working immediately. Tyk Pump gets 401 responses and drops batches until it is configured with the new token."
        buttonLabel="Rotate token"
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
        onConfirm={rotate}
        onCancel={() => setConfirm(null)}
        data-testid="ingest-rotate-dialog"
      />
      <ConfirmationDialog
        open={confirm === "revoke"}
        title="Revoke the ingest token?"
        message="Tyk Pump can no longer send analytics for this connection. Analytics already received are kept."
        buttonLabel="Revoke"
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
        onConfirm={revoke}
        onCancel={() => setConfirm(null)}
        data-testid="ingest-revoke-dialog"
      />
    </Box>
  );
};

export default TykIngestAnalytics;
