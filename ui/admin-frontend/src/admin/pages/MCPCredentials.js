import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Link as RouterLink, useSearchParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Link,
  MenuItem,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import KeyIcon from "@mui/icons-material/VpnKey";
import apiClient from "../utils/apiClient";
import Can from "../components/rbac/Can";
import { usePermissions } from "../context/PermissionsContext";
import { P } from "../rbac/permissions";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import useListQuery from "../hooks/useListQuery";
import useConfig from "../hooks/useConfig";
import { TitleBox, ContentBox, PrimaryButton, SecondaryOutlineButton } from "../styles/sharedStyles";
import { apiErrorDetail, formatTime } from "./webhookShared";
import { TykUpsell, TykDisabledNotice } from "./tykShared";
import { RevealKeyDialog } from "../../portal/components/AppMCPAccess";

export const CREDENTIAL_STATUSES = ["minting", "active", "suspended", "rotating_out", "revoked", "failed"];
export const DRIFT_STATES = ["none", "pending_widen", "applying", "error"];

const STATUS_LABELS = {
  minting: "Minting",
  active: "Active",
  suspended: "Suspended",
  rotating_out: "Rotating out",
  revoked: "Revoked",
  failed: "Failed",
};
const DRIFT_LABELS = {
  none: "In sync",
  pending_widen: "Widening pending",
  applying: "Applying",
  error: "Error",
};

const statusColor = (s) => ({ active: "success", suspended: "warning", rotating_out: "warning", revoked: "default", failed: "error" }[s] || "default");
const driftColor = (d) => ({ none: "default", pending_widen: "info", applying: "warning", error: "error" }[d] || "default");

export const CredentialStatusChip = ({ status }) => (
  <Chip size="small" label={STATUS_LABELS[status] || status} color={statusColor(status)} data-testid={`status-${status}`} />
);
export const DriftChip = ({ drift, detail }) => (
  <Tooltip title={detail || ""}>
    <Chip size="small" variant={drift === "none" ? "outlined" : "filled"} label={DRIFT_LABELS[drift] || drift} color={driftColor(drift)} data-testid={`drift-${drift}`} />
  </Tooltip>
);

const GRANT_LABELS = { key: "Tyk key", oauth: "OAuth", keyless: "Keyless", external: "External" };

const INTRO =
  "Every Tyk key AI Studio has minted for an App, with the policies it carries and whether it still matches what the App's MCP servers require. The access report lists who reaches which server through those keys.";

const CONFIRM_COPY = {
  rotate: {
    title: "Rotate this key?",
    body: "A new key is minted and shown to you once; the current key is revoked. The App owner must be given the new key.",
  },
  suspend: {
    title: "Suspend this key?",
    body: "The key is switched off on the Dashboard until it is resumed.",
  },
  revoke: {
    title: "Revoke this key?",
    body: "The key is deleted from the Dashboard. The App owner can mint a new one afterwards.",
  },
};

const ConnectionSelect = ({ connections, value, onChange, testId, allLabel = "All connections" }) => (
  <TextField select size="small" label="Connection" value={value} onChange={onChange} sx={{ minWidth: 200 }} inputProps={{ "data-testid": testId }}>
    <MenuItem value="">{allLabel}</MenuItem>
    {connections.map((c) => (
      <MenuItem key={c.id} value={String(c.id)}>
        {c.name}
      </MenuItem>
    ))}
  </TextField>
);

const CredentialsTab = ({ connections, initial, notify, canExecute, onEmptyMint }) => {
  const [rows, setRows] = useState([]);
  const [filters, setFilters] = useState({ connection_id: "", status: "", drift: "", app_id: "", server_id: "", ...initial });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [confirm, setConfirm] = useState(null); // {action, row}
  const [reason, setReason] = useState("");
  const [minted, setMinted] = useState(null);
  const { queryParams, updatePaginationData, tableProps, pageSize, handlePageChange } = useListQuery({ initialPageSize: 25 });

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const { search, ...params } = queryParams;
      Object.entries(filters).forEach(([k, v]) => {
        if (v !== "") params[k] = v;
      });
      const res = await apiClient.get("/mcp-credentials", { params });
      setRows(res.data?.credentials || []);
      const total = res.data?.total || 0;
      updatePaginationData(total, Math.max(1, Math.ceil(total / pageSize)));
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load credentials"));
    } finally {
      setLoading(false);
    }
  }, [filters, queryParams, pageSize, updatePaginationData]);

  useEffect(() => {
    load();
  }, [load]);

  const setFilter = (k) => (e) => {
    handlePageChange(1);
    setFilters((f) => ({ ...f, [k]: e.target.value }));
  };

  const act = async (action, row, body) => {
    setError(null);
    try {
      const res = await apiClient.post(`/mcp-credentials/${row.id}/${action}`, body || {});
      if (action === "rotate") {
        setMinted(res.data);
        notify(`Rotated the key for ${row.app_name || `app ${row.app_id}`}.`);
      } else {
        notify(`Credential ${row.id.slice(0, 8)}… ${action === "apply-drift" ? "updated" : action + "d"}.`);
      }
      await load();
    } catch (err) {
      notify(apiErrorDetail(err, `Failed to ${action} credential`), "error");
    }
  };

  const columns = useMemo(
    () => [
      {
        field: "app_name",
        headerName: "App",
        renderCell: (row) => (
          <Link component={RouterLink} to={`/admin/apps/${row.app_id}`} onClick={(e) => e.stopPropagation()}>
            {row.app_name || `App ${row.app_id}`}
          </Link>
        ),
      },
      { field: "connection_name", headerName: "Connection", renderCell: (row) => row.connection_name || row.connection_id },
      {
        field: "tyk_key_hash",
        headerName: "Key",
        renderCell: (row) => (
          <Box>
            <Tooltip title={row.tyk_key_hash || ""}>
              <span style={{ fontFamily: "monospace" }}>{row.key_hint ? `…${row.key_hint}` : (row.tyk_key_hash || "").slice(0, 10)}</span>
            </Tooltip>
            {row.alias && (
              <Typography variant="caption" display="block" color="text.secondary">
                {row.alias}
              </Typography>
            )}
          </Box>
        ),
      },
      {
        field: "status",
        headerName: "Status",
        renderCell: (row) => (
          <Box>
            <CredentialStatusChip status={row.status} />
            {row.revoke_reason && (
              <Typography variant="caption" display="block" color="text.secondary">
                {row.revoke_reason}
                {row.revoke_mode === "inactive_only" && " (still present on the Dashboard)"}
              </Typography>
            )}
            {row.last_error && (
              <Typography variant="caption" display="block" color="error">
                {row.last_error}
              </Typography>
            )}
          </Box>
        ),
      },
      {
        field: "applied_policy_ids",
        headerName: "Policies",
        renderCell: (row) => (
          <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
            {(row.applied_policy_ids || []).map((id) => (
              <Chip key={id} size="small" label={id} />
            ))}
            {(row.external_policy_ids || []).map((id) => (
              <Tooltip key={id} title="Added on the Dashboard; not managed by AI Studio">
                <Chip size="small" variant="outlined" color="warning" label={id} />
              </Tooltip>
            ))}
          </Box>
        ),
      },
      { field: "drift", headerName: "Drift", renderCell: (row) => <DriftChip drift={row.drift} detail={row.drift_detail} /> },
      { field: "minted_at", headerName: "Minted", renderCell: (row) => formatTime(row.minted_at) },
      { field: "expires_at", headerName: "Expires", renderCell: (row) => (row.expires_at ? formatTime(row.expires_at) : "Never") },
    ],
    []
  );

  // The action list depends on the row's status and drift, so it is built per row.
  const rowActions = useCallback(
    (row) => {
      const list = [];
      if (row.drift === "pending_widen") {
        list.push({ key: "apply-drift", label: "Apply pending change", onClick: (r) => act("apply-drift", r), "data-testid": `apply-drift-${row.id}` });
      }
      if (row.status === "active") {
        list.push({ key: "suspend", label: "Suspend key", onClick: (r) => setConfirm({ action: "suspend", row: r }), "data-testid": `suspend-${row.id}` });
      }
      if (row.status === "suspended") {
        list.push({ key: "resume", label: "Resume key", onClick: (r) => act("resume", r), "data-testid": `resume-${row.id}` });
      }
      if (["active", "suspended"].includes(row.status)) {
        list.push({ key: "rotate", label: "Rotate key", onClick: (r) => setConfirm({ action: "rotate", row: r }), "data-testid": `rotate-${row.id}` });
        list.push({ key: "revoke", label: "Revoke key", onClick: (r) => setConfirm({ action: "revoke", row: r }), "data-testid": `revoke-${row.id}` });
      }
      return list;
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );

  const filtersApplied = Object.values(filters).some((v) => v !== "");
  const closeConfirm = () => {
    setConfirm(null);
    setReason("");
  };

  return (
    <Box>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="page-error">
          {error}
        </Alert>
      )}
      <Stack direction="row" spacing={2} sx={{ mb: 2, flexWrap: "wrap" }} useFlexGap>
        <ConnectionSelect connections={connections} value={filters.connection_id} onChange={setFilter("connection_id")} testId="filter-connection" />
        <TextField select size="small" label="Status" value={filters.status} onChange={setFilter("status")} sx={{ minWidth: 160 }} inputProps={{ "data-testid": "filter-status" }}>
          <MenuItem value="">Any status</MenuItem>
          {CREDENTIAL_STATUSES.map((s) => (
            <MenuItem key={s} value={s}>
              {STATUS_LABELS[s]}
            </MenuItem>
          ))}
        </TextField>
        <TextField select size="small" label="Drift" value={filters.drift} onChange={setFilter("drift")} sx={{ minWidth: 160 }} inputProps={{ "data-testid": "filter-drift" }}>
          <MenuItem value="">Any drift</MenuItem>
          {DRIFT_STATES.map((d) => (
            <MenuItem key={d} value={d}>
              {DRIFT_LABELS[d]}
            </MenuItem>
          ))}
        </TextField>
        <TextField size="small" label="App ID" value={filters.app_id} onChange={setFilter("app_id")} inputProps={{ "data-testid": "filter-app" }} sx={{ width: 120 }} />
        <TextField size="small" label="Server ID" value={filters.server_id} onChange={setFilter("server_id")} inputProps={{ "data-testid": "filter-server" }} sx={{ width: 120 }} />
      </Stack>

      <DataTable
        {...tableProps}
        enableSearch={false}
        ariaLabel="Minted keys"
        columns={columns}
        data={rows}
        loading={loading}
        actions={canExecute ? rowActions : undefined}
        rowProps={(row) => ({ "data-testid": `credential-${row.id}` })}
        emptyState={
          !filtersApplied ? (
            <EmptyStateWidget
              title="No keys minted yet"
              description="Keys are minted by App owners from the portal once their App is approved and bound to a brokerable MCP server, or here by an administrator with Mint key."
              learnMoreLink={onEmptyMint.docsLink}
              actions={canExecute && connections.length > 0 ? <PrimaryButton variant="contained" startIcon={<KeyIcon />} onClick={onEmptyMint.open} data-testid="empty-mint">Mint key</PrimaryButton> : null}
            />
          ) : undefined
        }
        emptyMessage="No credentials match these filters."
      />

      <Dialog open={!!confirm} onClose={closeConfirm}>
        <DialogTitle>{confirm ? CONFIRM_COPY[confirm.action].title : ""}</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 2 }}>
            {confirm ? CONFIRM_COPY[confirm.action].body : ""}
          </Typography>
          {confirm?.action !== "rotate" && (
            <TextField fullWidth size="small" label="Reason (optional)" value={reason} onChange={(e) => setReason(e.target.value)} inputProps={{ "data-testid": "action-reason", maxLength: 255 }} />
          )}
        </DialogContent>
        <DialogActions>
          <SecondaryOutlineButton onClick={closeConfirm}>Cancel</SecondaryOutlineButton>
          <Button
            variant="contained"
            color={confirm?.action === "revoke" ? "error" : "primary"}
            data-testid="confirm-action"
            onClick={() => {
              const c = confirm;
              const r = reason;
              closeConfirm();
              act(c.action, c.row, c.action === "rotate" ? {} : { reason: r });
            }}
          >
            Confirm
          </Button>
        </DialogActions>
      </Dialog>
      <RevealKeyDialog minted={minted} onClose={() => setMinted(null)} />
    </Box>
  );
};

const AccessReportTab = ({ connections, initial }) => {
  const [rows, setRows] = useState([]);
  const [filters, setFilters] = useState({ connection: "", server: "", user: "", app: "", ...initial });
  const [includeRevoked, setIncludeRevoked] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params = {};
      Object.entries(filters).forEach(([k, v]) => {
        if (v !== "") params[k] = v;
      });
      if (includeRevoked) params.include_revoked = "true";
      const res = await apiClient.get("/mcp-access-report", { params });
      setRows(res.data || []);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load the access report"));
    } finally {
      setLoading(false);
    }
  }, [filters, includeRevoked]);

  useEffect(() => {
    load();
  }, [load]);

  const setFilter = (k) => (e) => setFilters((f) => ({ ...f, [k]: e.target.value }));

  const columns = useMemo(
    () => [
      { field: "user_email", headerName: "User", renderCell: (row) => row.user_email || row.user_id },
      {
        field: "app_name",
        headerName: "App",
        renderCell: (row) => (
          <Link component={RouterLink} to={`/admin/apps/${row.app_id}`}>
            {row.app_name}
          </Link>
        ),
      },
      {
        field: "server_name",
        headerName: "MCP server",
        renderCell: (row) => (
          <Link component={RouterLink} to={`/admin/mcp-servers/${row.server_id}`}>
            {row.server_name}
          </Link>
        ),
      },
      { field: "connection_name", headerName: "Connection", renderCell: (row) => row.connection_name || row.connection_id || "—" },
      { field: "grant_kind", headerName: "Access", renderCell: (row) => <Chip size="small" variant="outlined" label={GRANT_LABELS[row.grant_kind] || row.grant_kind} /> },
      {
        field: "credential_id",
        headerName: "Credential",
        renderCell: (row) =>
          row.credential_id ? (
            <Box>
              <CredentialStatusChip status={row.credential_status} />
              <Typography variant="caption" display="block" sx={{ fontFamily: "monospace" }}>
                {(row.credential_hash || "").slice(0, 10)}
              </Typography>
            </Box>
          ) : (
            <Typography variant="caption" color="text.secondary">
              No key
            </Typography>
          ),
      },
      { field: "granted_at", headerName: "Granted", renderCell: (row) => formatTime(row.granted_at) },
      {
        field: "revoked_at",
        headerName: "Revoked",
        renderCell: (row) => (
          <Box>
            {row.revoked_at ? formatTime(row.revoked_at) : "—"}
            {row.revoke_reason && (
              <Typography variant="caption" display="block" color="text.secondary">
                {row.revoke_reason}
              </Typography>
            )}
          </Box>
        ),
      },
    ],
    []
  );

  return (
    <Box>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="report-error">
          {error}
        </Alert>
      )}
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        One row per App and MCP server the App reaches, with the person who owns the App and the key that backs the access. Only key-backed access is
        listed: AI Studio does not broker access to OAuth, mTLS or keyless servers.
      </Typography>
      <Stack direction="row" spacing={2} sx={{ mb: 2, flexWrap: "wrap", alignItems: "center" }} useFlexGap>
        <ConnectionSelect connections={connections} value={filters.connection} onChange={setFilter("connection")} testId="report-connection" />
        <TextField size="small" label="Server ID" value={filters.server} onChange={setFilter("server")} inputProps={{ "data-testid": "report-server" }} sx={{ width: 120 }} />
        <TextField size="small" label="User ID" value={filters.user} onChange={setFilter("user")} inputProps={{ "data-testid": "report-user" }} sx={{ width: 120 }} />
        <TextField size="small" label="App ID" value={filters.app} onChange={setFilter("app")} inputProps={{ "data-testid": "report-app" }} sx={{ width: 120 }} />
        <FormControlLabel
          control={<Checkbox checked={includeRevoked} onChange={(e) => setIncludeRevoked(e.target.checked)} inputProps={{ "data-testid": "report-include-revoked" }} />}
          label="Include closed grants"
        />
      </Stack>
      <DataTable
        ariaLabel="Access report"
        columns={columns}
        data={rows}
        loading={loading}
        rowKey="grant_id"
        rowProps={(row) => ({ "data-testid": `grant-${row.grant_id}` })}
        getRowLabel={(row) => `${row.app_name} on ${row.server_name}`}
        emptyMessage="No grants match."
      />
    </Box>
  );
};

const MintDialog = ({ open, connections, onClose, onMinted }) => {
  const [appId, setAppId] = useState("");
  const [connectionId, setConnectionId] = useState("");
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await apiClient.post("/mcp-credentials", { app_id: Number(appId), connection_id: Number(connectionId) });
      onMinted(res.data);
      setAppId("");
      setConnectionId("");
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to mint the key"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog open={open} onClose={onClose} data-testid="mint-dialog">
      <DialogTitle>Mint a Tyk key for an App</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          The App must be approved and bound to at least one MCP server on the connection. The key is shown to you once; pass it to the App owner
          securely, or ask them to mint it themselves from the portal.
        </Typography>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} data-testid="mint-error">
            {error}
          </Alert>
        )}
        <TextField fullWidth size="small" label="App ID" value={appId} onChange={(e) => setAppId(e.target.value)} inputProps={{ "data-testid": "mint-app" }} sx={{ mb: 2 }} />
        <TextField select fullWidth size="small" label="Connection" value={connectionId} onChange={(e) => setConnectionId(e.target.value)} SelectProps={{ "data-testid": "mint-connection" }}>
          {connections.map((c) => (
            <MenuItem key={c.id} value={String(c.id)}>
              {c.name}
            </MenuItem>
          ))}
        </TextField>
      </DialogContent>
      <DialogActions>
        <SecondaryOutlineButton onClick={onClose}>Cancel</SecondaryOutlineButton>
        <PrimaryButton variant="contained" disabled={busy || !appId || !connectionId} onClick={submit} data-testid="mint-submit">
          Mint key
        </PrimaryButton>
      </DialogActions>
    </Dialog>
  );
};

const MCPCredentials = () => {
  const [searchParams] = useSearchParams();
  const { can } = usePermissions();
  const { getDocsLink } = useConfig();
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const canExecute = can(P.MCP_CREDENTIALS_EXECUTE);
  const [status, setStatus] = useState(null);
  const [connections, setConnections] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [tab, setTab] = useState(searchParams.get("tab") === "report" ? 1 : 0);
  const [mintOpen, setMintOpen] = useState(false);
  const [minted, setMinted] = useState(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const load = async () => {
      try {
        const st = await apiClient.get("/tyk-mcp/status");
        setStatus(st.data);
        if (st.data?.available && st.data?.enabled) {
          const conns = await apiClient.get("/tyk-connections");
          setConnections(conns.data || []);
        }
      } catch (err) {
        setError(apiErrorDetail(err, "Failed to load"));
      } finally {
        setLoading(false);
      }
    };
    load();
  }, []);

  if (loading) {
    return (
      <Box sx={{ p: 3, display: "flex", justifyContent: "center" }}>
        <CircularProgress />
      </Box>
    );
  }
  if (status && !status.available) return <TykUpsell />;
  if (status && !status.enabled) return <TykDisabledNotice status={status} />;

  const initialCredentialFilters = {};
  ["connection_id", "status", "drift", "app_id", "server_id"].forEach((k) => {
    if (searchParams.get(k)) initialCredentialFilters[k] = searchParams.get(k);
  });
  const initialReportFilters = {};
  ["connection", "server", "user", "app"].forEach((k) => {
    if (searchParams.get(k)) initialReportFilters[k] = searchParams.get(k);
  });

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">MCP credentials</Typography>
        <Can permission={P.MCP_CREDENTIALS_EXECUTE}>
          <PrimaryButton variant="contained" startIcon={<KeyIcon />} onClick={() => setMintOpen(true)} data-testid="open-mint" disabled={connections.length === 0}>
            Mint key
          </PrimaryButton>
        </Can>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          {INTRO}
        </Typography>
      </Box>
      <ContentBox>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}
        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2 }}>
          <Tab label="Minted keys" data-testid="tab-credentials" />
          <Tab label="Access report" data-testid="tab-report" />
        </Tabs>
        {tab === 0 ? (
          <CredentialsTab
            key={reloadKey}
            connections={connections}
            initial={initialCredentialFilters}
            notify={notify}
            canExecute={canExecute}
            onEmptyMint={{ open: () => setMintOpen(true), docsLink: getDocsLink("mcp_servers") }}
          />
        ) : (
          <AccessReportTab connections={connections} initial={initialReportFilters} />
        )}
      </ContentBox>
      <MintDialog
        open={mintOpen}
        connections={connections}
        onClose={() => setMintOpen(false)}
        onMinted={(m) => {
          setMintOpen(false);
          setMinted(m);
          setReloadKey((k) => k + 1);
        }}
      />
      <RevealKeyDialog minted={minted} onClose={() => setMinted(null)} />
      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default MCPCredentials;
