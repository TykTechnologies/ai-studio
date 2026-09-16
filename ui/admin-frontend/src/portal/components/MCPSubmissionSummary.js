import React from "react";
import { Box, Chip, Grid, Typography } from "@mui/material";
import { MCP_AUTH_LABELS, MCP_KIND_LABELS } from "../utils/catalog";

const CONSUMER_AUTH_LABELS = { auth_token: "API key", oauth21: "OAuth 2.1", keyless: "Keyless" };

const Field = ({ label, children, xs = 6 }) => (
  <Grid item xs={xs}>
    <Typography variant="body2" color="text.secondary">
      {label}
    </Typography>
    <Typography component="div">{children || "—"}</Typography>
  </Grid>
);

/**
 * MCPSubmissionSummary renders an mcp_server submission payload read-only,
 * for the submitter's detail page and the reviewer's page alike. Credentials
 * arrive redacted from the API and are shown as such.
 */
const MCPSubmissionSummary = ({ payload = {}, connectionName }) => {
  const kind = payload.kind || "remote";
  return (
    <Grid container spacing={2} data-testid="mcp-submission-summary">
      <Field label="Tyk Dashboard connection">{connectionName || (payload.connection_id ? `#${payload.connection_id}` : "")}</Field>
      <Field label="Kind">{MCP_KIND_LABELS?.[kind] || (kind === "rest_to_mcp" ? "REST API to MCP" : "Remote MCP server")}</Field>
      {kind === "remote" ? (
        <>
          <Field label="Upstream MCP URL" xs={12}>
            <span style={{ fontFamily: "monospace", wordBreak: "break-all" }}>{payload.upstream_url}</span>
          </Field>
          <Field label="Upstream auth">
            {payload.upstream_auth_token ? `${payload.upstream_auth_header_name || "Authorization"}: [redacted]` : "none"}
          </Field>
          <Field label="Allowed tools">
            {(payload.allowed_tools || []).length > 0 ? (
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                {payload.allowed_tools.map((t) => (
                  <Chip key={t} size="small" label={t} />
                ))}
              </Box>
            ) : (
              "every tool the server offers"
            )}
          </Field>
        </>
      ) : (
        <Field label="Source API id" xs={12}>
          <span style={{ fontFamily: "monospace" }}>{payload.source_api_id}</span>
        </Field>
      )}
      <Field label="Consumer authentication">
        {CONSUMER_AUTH_LABELS[payload.consumer_auth || "auth_token"] || MCP_AUTH_LABELS?.[payload.consumer_auth] || payload.consumer_auth}
        {payload.consumer_auth === "oauth21" && (payload.authorization_servers || []).length > 0 ? ` via ${payload.authorization_servers.join(", ")}` : ""}
      </Field>
      <Field label="Suggested listen path">{payload.suggested_listen_path}</Field>
      <Field label="Deployment target">
        {(payload.gateway_tags || []).length > 0 ? (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
            {payload.gateway_tags.map((t) => (
              <Chip key={t} size="small" label={t} />
            ))}
          </Box>
        ) : (
          "any non-segmented gateway"
        )}
      </Field>
      {payload.transport_notes && (
        <Field label="Transport notes" xs={12}>
          {payload.transport_notes}
        </Field>
      )}
      {payload.description && (
        <Field label="Description" xs={12}>
          {payload.description}
        </Field>
      )}
    </Grid>
  );
};

export default MCPSubmissionSummary;
