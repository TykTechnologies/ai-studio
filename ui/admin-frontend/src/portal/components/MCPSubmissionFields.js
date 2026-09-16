import React, { useMemo } from "react";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Autocomplete,
  Checkbox,
  Chip,
  FormControlLabel,
  Grid,
  MenuItem,
  TextField,
  Typography,
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";

// The MCP server submission payload (resource_type "mcp_server"): what a
// portal user needs to tell the platform team, or AI Studio itself on a
// full-mode connection, to put an MCP server behind the Tyk Gateway.

const listToText = (v) => (Array.isArray(v) ? v.join(", ") : v || "");
const textToList = (s) =>
  (s || "")
    .split(/[,\n]/)
    .map((x) => x.trim())
    .filter(Boolean);

/**
 * validateMCPPayload returns the field errors for an mcp_server payload. It
 * mirrors the server's checks that need no Dashboard call so the message
 * lands next to the field.
 */
export const validateMCPPayload = (payload, connections = []) => {
  const errors = {};
  const conn = connections.find((c) => String(c.id) === String(payload.connection_id));
  if (!payload.connection_id) errors.connection_id = "Choose the Tyk Dashboard connection";
  if (!payload.description?.trim()) errors.description = "A description is required";
  const kind = payload.kind || "remote";
  if (kind === "remote") {
    if (!payload.upstream_url?.trim()) errors.upstream_url = "The upstream MCP URL is required";
    else if (!/^https?:\/\/\S+$/i.test(payload.upstream_url.trim())) errors.upstream_url = "Enter an absolute http(s) URL";
    if (payload.upstream_auth_header_name && !payload.upstream_auth_token) errors.upstream_auth_token = "The header needs a value";
  } else {
    if (!payload.source_api_id?.trim()) errors.source_api_id = "The Tyk API id of the REST API is required";
    if (conn && !conn.rest_to_mcp_supported) errors.kind = "This connection's Dashboard does not support REST API to MCP (Tyk 5.15+)";
  }
  if (payload.suggested_listen_path && !/^\/[a-z0-9][a-z0-9\-/]*\/$/.test(payload.suggested_listen_path)) {
    errors.suggested_listen_path = "Lowercase letters, digits and dashes, wrapped in slashes, e.g. /orders-mcp/";
  }
  if (payload.consumer_auth === "keyless" && !payload.confirm_keyless) errors.confirm_keyless = "Confirm that the server may be keyless";
  if (payload.consumer_auth === "oauth21" && textToList(listToText(payload.authorization_servers)).length === 0) {
    errors.authorization_servers = "Enter at least one authorization server URL";
  }
  const tagOptions = conn?.gateway_tags || [];
  if (tagOptions.length > 0 && (payload.gateway_tags || []).length === 0 && !payload.confirm_no_gateway_tags) {
    errors.gateway_tags = "Choose a deployment target or confirm that none is wanted";
  }
  return errors;
};

const MCPSubmissionFields = ({ payload, onChange, connections = [], errors = {} }) => {
  const conn = useMemo(() => connections.find((c) => String(c.id) === String(payload.connection_id)), [connections, payload.connection_id]);
  const kind = payload.kind || "remote";
  const consumerAuth = payload.consumer_auth || "auth_token";
  const tagOptions = conn?.gateway_tags || [];
  const set = (field) => (e) => onChange(field, e.target.type === "checkbox" ? e.target.checked : e.target.value);

  return (
    <Accordion defaultExpanded data-testid="mcp-submission-fields">
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography variant="h6">MCP Server Details</Typography>
      </AccordionSummary>
      <AccordionDetails>
        {connections.length === 0 && (
          <Alert severity="info" sx={{ mb: 2 }}>
            No Tyk Dashboard connection accepts MCP server submissions right now. Ask an administrator to enable one.
          </Alert>
        )}
        <Grid container spacing={2}>
          <Grid item xs={12} md={6}>
            <TextField select fullWidth label="Tyk Dashboard connection" value={payload.connection_id ? String(payload.connection_id) : ""} onChange={(e) => onChange("connection_id", Number(e.target.value))} error={!!errors.connection_id} helperText={errors.connection_id || (conn ? (conn.direct_create ? "AI Studio creates the proxy on approval." : "The platform team creates the proxy from a handoff package on approval.") : "")} inputProps={{ "data-testid": "mcp-connection" }} required>
              {connections.map((c) => (
                <MenuItem key={c.id} value={String(c.id)}>
                  {c.name}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid item xs={12} md={6}>
            <TextField select fullWidth label="Kind" value={kind} onChange={set("kind")} error={!!errors.kind} helperText={errors.kind} inputProps={{ "data-testid": "mcp-kind" }}>
              <MenuItem value="remote">Remote MCP server</MenuItem>
              <MenuItem value="rest_to_mcp" disabled={conn ? !conn.rest_to_mcp_supported : false}>
                REST API to MCP {conn && !conn.rest_to_mcp_supported ? "(not supported by this Dashboard)" : ""}
              </MenuItem>
            </TextField>
          </Grid>
          {kind === "remote" ? (
            <>
              <Grid item xs={12}>
                <TextField fullWidth label="Upstream MCP URL" value={payload.upstream_url || ""} onChange={set("upstream_url")} error={!!errors.upstream_url} helperText={errors.upstream_url || "The MCP server's Streamable HTTP endpoint. AI Studio never calls it; the Tyk Gateway does."} inputProps={{ "data-testid": "mcp-upstream-url" }} required />
              </Grid>
              <Grid item xs={12} md={4}>
                <TextField fullWidth label="Upstream auth header (optional)" value={payload.upstream_auth_header_name || ""} onChange={set("upstream_auth_header_name")} inputProps={{ "data-testid": "mcp-upstream-header" }} placeholder="Authorization" />
              </Grid>
              <Grid item xs={12} md={8}>
                <TextField fullWidth type="password" label="Upstream auth value (optional)" value={payload.upstream_auth_token || ""} onChange={set("upstream_auth_token")} error={!!errors.upstream_auth_token} helperText={errors.upstream_auth_token || "Stored encrypted, shown to nobody, sent to the Tyk Dashboard on approval."} inputProps={{ "data-testid": "mcp-upstream-token" }} autoComplete="new-password" />
              </Grid>
              <Grid item xs={12}>
                <TextField fullWidth label="Allowed tools (optional, comma separated)" value={listToText(payload.allowed_tools)} onChange={(e) => onChange("allowed_tools", textToList(e.target.value))} helperText="When set, only these tools are exposed; everything else the server offers is blocked." />
              </Grid>
            </>
          ) : (
            <Grid item xs={12}>
              <TextField fullWidth label="Source API id (Tyk OAS API)" value={payload.source_api_id || ""} onChange={set("source_api_id")} error={!!errors.source_api_id} helperText={errors.source_api_id || "The api id of the REST API on the Tyk Dashboard; the reviewer picks the operations to expose."} inputProps={{ "data-testid": "mcp-source-api" }} required />
            </Grid>
          )}
          <Grid item xs={12} md={4}>
            <TextField select fullWidth label="Consumer authentication" value={consumerAuth} onChange={set("consumer_auth")} inputProps={{ "data-testid": "mcp-consumer-auth" }}>
              <MenuItem value="auth_token">API key (AI Studio mints keys)</MenuItem>
              <MenuItem value="oauth21">OAuth 2.1 (clients bring a token)</MenuItem>
              <MenuItem value="keyless">Keyless</MenuItem>
            </TextField>
          </Grid>
          {consumerAuth === "oauth21" && (
            <>
              <Grid item xs={12} md={4}>
                <TextField fullWidth label="Authorization servers (comma separated)" value={listToText(payload.authorization_servers)} onChange={(e) => onChange("authorization_servers", textToList(e.target.value))} error={!!errors.authorization_servers} helperText={errors.authorization_servers} inputProps={{ "data-testid": "mcp-authorization-servers" }} />
              </Grid>
              <Grid item xs={12} md={4}>
                <TextField fullWidth label="Scopes (optional)" value={listToText(payload.scopes_supported)} onChange={(e) => onChange("scopes_supported", textToList(e.target.value))} />
              </Grid>
            </>
          )}
          {consumerAuth === "keyless" && (
            <Grid item xs={12} md={8}>
              <Alert severity="warning">
                Anyone who can reach the gateway can call a keyless server.
                <FormControlLabel sx={{ ml: 1 }} control={<Checkbox checked={!!payload.confirm_keyless} onChange={set("confirm_keyless")} inputProps={{ "data-testid": "mcp-confirm-keyless" }} />} label="I understand" />
              </Alert>
              {errors.confirm_keyless && (
                <Typography variant="caption" color="error">
                  {errors.confirm_keyless}
                </Typography>
              )}
            </Grid>
          )}
          <Grid item xs={12} md={6}>
            <TextField fullWidth label="Suggested listen path (optional)" value={payload.suggested_listen_path || ""} onChange={set("suggested_listen_path")} error={!!errors.suggested_listen_path} helperText={errors.suggested_listen_path || "e.g. /weather-mcp/; the reviewer may change it"} inputProps={{ "data-testid": "mcp-listen-path" }} />
          </Grid>
          <Grid item xs={12} md={6}>
            <TextField fullWidth label="Transport notes (optional)" value={payload.transport_notes || ""} onChange={set("transport_notes")} helperText="Anything the platform team should know about the server's transport or quirks." />
          </Grid>
          {tagOptions.length > 0 && (
            <Grid item xs={12}>
              <Autocomplete
                multiple
                options={tagOptions.map((o) => o.tag)}
                value={payload.gateway_tags || []}
                onChange={(_, v) => onChange("gateway_tags", v)}
                renderTags={(value, getTagProps) => value.map((tag, index) => <Chip {...getTagProps({ index })} key={tag} size="small" label={tag} />)}
                renderInput={(params) => <TextField {...params} label="Deployment target (gateway tags)" error={!!errors.gateway_tags} helperText={errors.gateway_tags || "Which gateways should serve this server."} inputProps={{ ...params.inputProps, "data-testid": "mcp-gateway-tags" }} />}
              />
              {(payload.gateway_tags || []).length === 0 && (
                <FormControlLabel control={<Checkbox checked={!!payload.confirm_no_gateway_tags} onChange={set("confirm_no_gateway_tags")} inputProps={{ "data-testid": "mcp-confirm-no-tags" }} />} label="No specific target: any non-segmented gateway" />
              )}
            </Grid>
          )}
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
};

export default MCPSubmissionFields;
