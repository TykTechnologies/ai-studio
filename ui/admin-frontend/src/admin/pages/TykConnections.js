import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, Box, Chip, CircularProgress, TextField, Tooltip, Typography } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import apiClient from "../utils/apiClient";
import { usePermissions } from "../context/PermissionsContext";
import { P } from "../rbac/permissions";
import Can from "../components/rbac/Can";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import ConfirmationDialog from "../components/common/ConfirmationDialog";
import DeleteConfirmationDialog from "../components/common/DeleteConfirmationDialog";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import { TitleBox, ContentBox, PrimaryButton } from "../styles/sharedStyles";
import { formatTime, apiErrorDetail } from "./webhookShared";
import { ConnectionStatusChip, ModeChip, TykUpsell, TykDisabledNotice } from "./tykShared";

const INTRO =
  "Tyk Connections link AI Studio to the Tyk Dashboards that run your MCP proxies. Each connection imports the Dashboard's MCP servers into the AI Portal, brokers Tyk access keys for Apps, and, in full mode, publishes new proxies there.";

/**
 * TykConnections lists the connected Tyk Dashboards. Creating and editing
 * happen on the TykConnectionForm page; activation, sync, probe, disable and
 * delete run from the row menu.
 */
const TykConnections = () => {
  const navigate = useNavigate();
  const { can } = usePermissions();
  const canWrite = can(P.TYK_CONNECTIONS_WRITE);
  const canExecute = can(P.TYK_CONNECTIONS_EXECUTE);
  const canDelete = can(P.TYK_CONNECTIONS_DELETE);
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const [status, setStatus] = useState(null);
  const [connections, setConnections] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState("");
  const [confirm, setConfirm] = useState(null); // { action: "activate"|"disable", conn }
  const [reason, setReason] = useState("");
  const [deleteTarget, setDeleteTarget] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const st = await apiClient.get("/tyk-mcp/status");
      setStatus(st.data);
      if (st.data?.available && st.data?.enabled) {
        const res = await apiClient.get("/tyk-connections");
        setConnections(res.data || []);
      }
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load connections"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const run = useCallback(
    async (fn, success) => {
      try {
        await fn();
        if (success) notify(success);
        await load();
      } catch (err) {
        notify(apiErrorDetail(err, "Action failed"), "error");
      }
    },
    [load, notify],
  );

  const closeConfirm = () => {
    setConfirm(null);
    setReason("");
  };

  const onConfirm = async () => {
    const { action, conn } = confirm;
    const text = reason;
    closeConfirm();
    if (action === "activate") {
      await run(() => apiClient.post(`/tyk-connections/${conn.id}/activate`, {}), `${conn.name} activated`);
    } else if (action === "disable") {
      await run(() => apiClient.post(`/tyk-connections/${conn.id}/disable`, { reason: text }), `${conn.name} disabled`);
    }
  };

  const handleDelete = async (conn) => {
    await run(() => apiClient.delete(`/tyk-connections/${conn.id}`), `${conn.name} deleted`);
  };

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return connections;
    return connections.filter(
      (c) => (c.name || "").toLowerCase().includes(term) || (c.dashboard_url || "").toLowerCase().includes(term),
    );
  }, [connections, search]);

  const columns = useMemo(
    () => [
      {
        field: "name",
        headerName: "Name",
        renderCell: (conn) => (
          <Box>
            <Typography variant="body2" fontWeight={600}>
              {conn.name}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {conn.dashboard_url}
            </Typography>
          </Box>
        ),
      },
      {
        field: "mode",
        headerName: "Mode",
        renderCell: (conn) => <ModeChip declared={conn.declared_mode} effective={conn.effective_mode} />,
      },
      {
        field: "status",
        headerName: "Status",
        renderCell: (conn) => <ConnectionStatusChip status={conn.status} degraded={conn.degraded} />,
      },
      {
        field: "last_sync",
        headerName: "Last sync",
        renderCell: (conn) =>
          conn.last_sync_at ? (
            <Tooltip title={conn.last_sync_error || ""}>
              <span>
                {formatTime(conn.last_sync_at)} {conn.last_sync_status ? `(${conn.last_sync_status})` : ""}
              </span>
            </Tooltip>
          ) : (
            <Typography variant="caption" color="text.secondary">
              never
            </Typography>
          ),
      },
      {
        field: "gateway_tags",
        headerName: "Gateway tags",
        renderCell: (conn) =>
          (conn.gateway_tags || []).length > 0 ? (
            <Chip size="small" label={`${conn.gateway_tags.length} tag(s)`} variant="outlined" />
          ) : (
            <Typography variant="caption" color="text.secondary">
              none
            </Typography>
          ),
      },
    ],
    [],
  );

  const rowActions = useMemo(() => {
    const actions = [];
    if (canWrite) {
      actions.push({ key: "edit", label: "Edit connection", onClick: (conn) => navigate(`/admin/tyk-connections/edit/${conn.id}`) });
    }
    if (canExecute) {
      actions.push(
        {
          key: "activate",
          label: "Activate",
          hidden: (conn) => conn.status === "active",
          onClick: (conn) => setConfirm({ action: "activate", conn }),
          "data-testid": "menu-activate",
        },
        {
          key: "sync",
          label: "Sync now",
          hidden: (conn) => conn.status !== "active",
          onClick: (conn) => run(() => apiClient.post(`/tyk-connections/${conn.id}/sync`, {}), "Sync requested"),
          "data-testid": "menu-sync",
        },
        {
          key: "probe",
          label: "Probe",
          onClick: (conn) => run(() => apiClient.post(`/tyk-connections/${conn.id}/probe`, {}), "Probe complete"),
          "data-testid": "menu-probe",
        },
        {
          key: "disable",
          label: "Disable",
          hidden: (conn) => conn.status !== "active",
          onClick: (conn) => setConfirm({ action: "disable", conn }),
          "data-testid": "menu-disable",
        },
      );
    }
    if (canDelete) {
      actions.push({ key: "delete", label: "Delete connection", onClick: (conn) => setDeleteTarget(conn), "data-testid": "menu-delete" });
    }
    return actions;
  }, [canWrite, canExecute, canDelete, navigate, run]);

  if (loading && !status) {
    return (
      <Box sx={{ p: 3, display: "flex", justifyContent: "center" }}>
        <CircularProgress />
      </Box>
    );
  }
  if (status && !status.available) return <TykUpsell />;
  if (status && !status.enabled) return <TykDisabledNotice status={status} />;

  const addButton = (
    <PrimaryButton variant="contained" startIcon={<AddIcon />} onClick={() => navigate("/admin/tyk-connections/new")} data-testid="add-connection">
      Connect Dashboard
    </PrimaryButton>
  );

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Tyk Connections</Typography>
        <Can permission={P.TYK_CONNECTIONS_WRITE}>{addButton}</Can>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          {INTRO}
        </Typography>
      </Box>
      <ContentBox>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="page-error">
            {error}
          </Alert>
        )}
        {connections.length > 0 && (
          <Box sx={{ mb: 2, maxWidth: 400 }}>
            <TextField
              fullWidth
              size="small"
              placeholder="Search connections by name or URL..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              inputProps={{ "data-testid": "connection-search", "aria-label": "Search connections" }}
            />
          </Box>
        )}
        <DataTable
          ariaLabel="Tyk connections"
          columns={columns}
          data={rows}
          loading={loading}
          onRowClick={canWrite ? (conn) => navigate(`/admin/tyk-connections/edit/${conn.id}`) : undefined}
          actions={rowActions.length > 0 ? rowActions : undefined}
          rowProps={(conn) => ({ "data-testid": `connection-row-${conn.id}` })}
          getRowLabel={(conn) => conn.name}
          emptyMessage={search ? `No matches for “${search}”` : "No connections"}
          emptyState={
            connections.length === 0 ? (
              <EmptyStateWidget
                title="No Tyk Dashboard connected yet"
                description="Connect a Tyk Dashboard to import its MCP proxies into the AI Portal, broker access keys for Apps and, in full mode, publish new proxies from AI Studio."
                actions={canWrite ? addButton : null}
              />
            ) : undefined
          }
        />
      </ContentBox>

      <ConfirmationDialog
        open={Boolean(confirm)}
        title={confirm?.action === "activate" ? `Activate ${confirm?.conn?.name}?` : `Disable ${confirm?.conn?.name}?`}
        message={
          confirm?.action === "activate"
            ? `Activating probes ${confirm?.conn?.dashboard_url} and lets AI Studio import MCP proxies${
                confirm?.conn?.declared_mode !== "catalogue" ? " and write to the Dashboard" : ""
              }.`
            : "Syncing stops and every key minted on this connection is suspended on the Dashboard."
        }
        confirmText={
          confirm?.action === "disable" ? (
            <TextField
              fullWidth
              size="small"
              label="Reason (optional)"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              inputProps={{ "data-testid": "action-reason" }}
            />
          ) : (
            ""
          )
        }
        buttonLabel={confirm?.action === "activate" ? "Activate" : "Disable"}
        primaryButtonComponent={confirm?.action === "disable" ? "danger" : "primary"}
        iconName={confirm?.action === "disable" ? "hexagon-exclamation" : "circle-info"}
        iconColor={confirm?.action === "disable" ? "background.buttonCritical" : undefined}
        titleColor={confirm?.action === "disable" ? "text.criticalDefault" : undefined}
        backgroundColor={confirm?.action === "disable" ? "background.surfaceCriticalDefault" : undefined}
        borderColor={confirm?.action === "disable" ? "border.criticalDefaultSubdue" : undefined}
        onConfirm={onConfirm}
        onCancel={closeConfirm}
        data-testid="confirm-dialog"
      />

      <DeleteConfirmationDialog
        open={Boolean(deleteTarget)}
        resourcePath="tyk-connections"
        objectLabel="connection"
        item={deleteTarget ? { id: deleteTarget.id, name: deleteTarget.name } : null}
        consequence="Deleting it removes the connection and every MCP server imported from it from AI Studio. Live credentials must be revoked first."
        onConfirm={() => {
          const target = deleteTarget;
          setDeleteTarget(null);
          handleDelete(target);
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default TykConnections;
