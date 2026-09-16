import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Tooltip,
  Typography,
} from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import KeyIcon from "@mui/icons-material/VpnKey";
import pubClient from "../../admin/utils/pubClient";
import { MCP_AUTH_LABELS } from "../utils/catalog";

const STATUS_LABELS = {
  minting: "Minting",
  active: "Active",
  suspended: "Suspended",
  rotating_out: "Rotating",
  revoked: "Revoked",
  failed: "Failed",
};

const statusColor = (status) => {
  switch (status) {
    case "active":
      return "success";
    case "suspended":
    case "rotating_out":
      return "warning";
    case "revoked":
    case "failed":
      return "error";
    default:
      return "default";
  }
};

export const CredentialStatusChip = ({ status }) => (
  <Chip size="small" label={STATUS_LABELS[status] || status} color={statusColor(status)} data-testid={`credential-status-${status}`} />
);

const errorDetail = (err, fallback) => err?.response?.data?.detail || err?.response?.data?.error || err?.message || fallback;

const copy = (text) => {
  if (navigator.clipboard?.writeText) navigator.clipboard.writeText(text).catch(() => {});
};

const mcpRemoteSnippet = (server, key) =>
  JSON.stringify(
    {
      mcpServers: {
        [server.slug || server.name]: {
          command: "npx",
          args: ["mcp-remote", server.endpoint_url, "--header", `${server.header_name || "Authorization"}: ${key}`],
        },
      },
    },
    null,
    2
  );

/**
 * RevealKeyDialog shows a freshly minted Tyk key exactly once. The platform
 * never stores the plaintext, so closing this dialog is final.
 */
export const RevealKeyDialog = ({ minted, onClose }) => {
  if (!minted) return null;
  const { key, servers = [], skipped = [], credential } = minted;
  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth data-testid="reveal-key-dialog">
      <DialogTitle>Your Tyk access key</DialogTitle>
      <DialogContent>
        <Alert severity="warning" sx={{ mb: 2 }}>
          Copy this key now. It is shown only once and cannot be recovered; if you lose it, rotate the key to get a new one.
        </Alert>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2 }}>
          <Typography component="code" sx={{ fontFamily: "monospace", wordBreak: "break-all", flexGrow: 1 }} data-testid="revealed-key">
            {key}
          </Typography>
          <Tooltip title="Copy key">
            <IconButton size="small" onClick={() => copy(key)} aria-label="Copy key">
              <ContentCopyIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        </Box>
        {credential?.expires_at && (
          <Typography variant="body2" sx={{ mb: 2 }}>
            Expires {new Date(credential.expires_at).toLocaleString()}.
          </Typography>
        )}
        {servers.map((server) => (
          <Box key={server.id} sx={{ mb: 2 }} data-testid={`reveal-server-${server.id}`}>
            <Typography variant="subtitle2">{server.name}</Typography>
            <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
              {server.endpoint_url || "Endpoint URL not configured; ask your platform team."}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Send the key in the <code>{server.header_name || "Authorization"}</code> header.
            </Typography>
            {server.endpoint_url && (
              <Box component="pre" sx={{ fontSize: "0.8rem", overflowX: "auto", bgcolor: "action.hover", p: 1, mt: 1 }}>
                {mcpRemoteSnippet(server, key)}
              </Box>
            )}
          </Box>
        ))}
        {skipped.length > 0 && (
          <Alert severity="info" data-testid="reveal-skipped">
            Not covered by this key: {skipped.map((s) => `${s.name} (${s.reason})`).join(", ")}
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} variant="contained" data-testid="reveal-close">
          I have copied the key
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const ServerInstructions = ({ server }) => {
  const auth = MCP_AUTH_LABELS[server.auth_mode] || server.auth_mode;
  return (
    <Box sx={{ mb: 1 }} data-testid={`mcp-access-server-${server.id}`}>
      <Typography variant="subtitle2">{server.name}</Typography>
      <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
        {server.endpoint_url || "The gateway URL has not been configured for this server yet."}
      </Typography>
      <Typography variant="body2" color="text.secondary">
        Authentication: {auth}.
        {server.auth_mode === "keyless" && " No credential is needed."}
        {server.prm && server.prm.authorization_servers?.length > 0 && (
          <>
            {" "}
            Obtain a token from {server.prm.authorization_servers.join(", ")}
            {server.prm.scopes_supported?.length > 0 && ` with scopes ${server.prm.scopes_supported.join(", ")}`}.
          </>
        )}
        {!server.brokerable && !["keyless"].includes(server.auth_mode) && !server.prm && " Credentials for this server are issued by your platform team."}
      </Typography>
    </Box>
  );
};

/**
 * AppMCPAccess is the "MCP access" section of the portal App page: the Tyk
 * MCP servers the App reaches, grouped by Dashboard connection, with the
 * access key controls for the connections that broker keys.
 */
const AppMCPAccess = ({ appId, credentialActive }) => {
  const [summary, setSummary] = useState(null);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [minted, setMinted] = useState(null);
  const [confirm, setConfirm] = useState(null); // {kind, credential}

  const load = useCallback(async () => {
    try {
      const res = await pubClient.get(`/common/apps/${appId}/mcp`);
      setSummary(res.data || { servers: [], credentials: [], connections: [] });
    } catch (err) {
      setError(errorDetail(err, "Failed to load MCP access"));
    }
  }, [appId]);

  useEffect(() => {
    load();
  }, [load]);

  const run = async (fn) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await load();
    } catch (err) {
      setError(errorDetail(err, "The request failed"));
    } finally {
      setBusy(false);
    }
  };

  const mint = (connectionId) =>
    run(async () => {
      const res = await pubClient.post(`/common/apps/${appId}/mcp/credentials`, { connection_id: connectionId });
      setMinted(res.data);
    });
  const rotate = (credential) =>
    run(async () => {
      const res = await pubClient.post(`/common/apps/${appId}/mcp/credentials/${credential.id}/rotate`, {});
      setMinted(res.data);
    });
  const revoke = (credential) => run(() => pubClient.post(`/common/apps/${appId}/mcp/credentials/${credential.id}/revoke`, {}));

  if (!summary && !error) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", p: 2 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }
  const servers = summary?.servers || [];
  const credentials = summary?.credentials || [];
  const connections = summary?.connections || [];
  const liveFor = (connectionId) =>
    credentials.find((c) => c.connection_id === connectionId && ["active", "suspended", "minting"].includes(c.status));
  const groups = connections.length
    ? connections.map((conn) => ({ conn, servers: servers.filter((s) => s.connection_id === conn.connection_id) }))
    : [{ conn: null, servers }];

  return (
    <Box data-testid="app-mcp-access">
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="mcp-access-error">
          {error}
        </Alert>
      )}
      {groups.map(({ conn, servers: group }) => {
        const live = conn ? liveFor(conn.connection_id) : null;
        return (
          <Card key={conn ? conn.connection_id : "none"} sx={{ mb: 2 }} data-testid={conn ? `mcp-connection-${conn.connection_id}` : "mcp-connection-none"}>
            <CardContent>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1, flexWrap: "wrap" }}>
                <Typography variant="h6">{conn ? conn.connection_name : "MCP servers"}</Typography>
                {live && <CredentialStatusChip status={live.status} />}
                {live?.drift === "pending_widen" && (
                  <Tooltip title="A change that widens this key's access is waiting for an administrator.">
                    <Chip size="small" color="info" label="Change pending" />
                  </Tooltip>
                )}
              </Box>
              {group.map((server) => (
                <ServerInstructions key={server.id} server={server} />
              ))}
              {conn && (
                <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mt: 1 }}>
                  {live ? (
                    <>
                      <Typography variant="body2" color="text.secondary">
                        Key {live.key_hint ? `…${live.key_hint}` : live.tyk_key_hash?.slice(0, 8)} minted{" "}
                        {live.minted_at ? new Date(live.minted_at).toLocaleString() : ""}
                        {live.expires_at ? `, expires ${new Date(live.expires_at).toLocaleString()}` : ""}
                      </Typography>
                      <Button size="small" disabled={busy || !credentialActive} onClick={() => setConfirm({ kind: "rotate", credential: live })} data-testid={`rotate-${conn.connection_id}`}>
                        Rotate key
                      </Button>
                      <Button size="small" color="error" disabled={busy} onClick={() => setConfirm({ kind: "revoke", credential: live })} data-testid={`revoke-${conn.connection_id}`}>
                        Revoke key
                      </Button>
                    </>
                  ) : conn.can_mint ? (
                    <Button size="small" variant="contained" startIcon={<KeyIcon />} disabled={busy} onClick={() => mint(conn.connection_id)} data-testid={`mint-${conn.connection_id}`}>
                      Get access key
                    </Button>
                  ) : (
                    conn.reason && (
                      <Typography variant="body2" color="text.secondary" data-testid={`mint-reason-${conn.connection_id}`}>
                        {conn.reason}
                      </Typography>
                    )
                  )}
                </Box>
              )}
            </CardContent>
          </Card>
        );
      })}
      <RevealKeyDialog minted={minted} onClose={() => setMinted(null)} />
      <Dialog open={!!confirm} onClose={() => setConfirm(null)}>
        <DialogTitle>{confirm?.kind === "rotate" ? "Rotate this key?" : "Revoke this key?"}</DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            {confirm?.kind === "rotate"
              ? "A new key is issued and the current one stops working immediately. Update every client that uses it."
              : "The key stops working immediately. You can request a new one afterwards."}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirm(null)}>Cancel</Button>
          <Button
            variant="contained"
            color={confirm?.kind === "rotate" ? "primary" : "error"}
            data-testid="confirm-action"
            onClick={() => {
              const c = confirm;
              setConfirm(null);
              if (c.kind === "rotate") rotate(c.credential);
              else revoke(c.credential);
            }}
          >
            {confirm?.kind === "rotate" ? "Rotate" : "Revoke"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default AppMCPAccess;
