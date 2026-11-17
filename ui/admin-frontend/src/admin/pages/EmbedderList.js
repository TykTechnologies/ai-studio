import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { Typography, Alert, Box, Chip } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import DeleteConfirmationDialog from "../components/common/DeleteConfirmationDialog";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import { TitleBox, PrimaryButton } from "../styles/sharedStyles";
import useListQuery from "../hooks/useListQuery";
import { isPermissionDenied } from "../utils/apiErrors";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";
import {
  useWorkspaceColumn,
  useWorkspaceFilter,
  useWorkspaceAccess,
  WorkspaceFilter,
} from "../components/workspaces/workspaceFields";
import { getEmbedderName } from "../utils/vendorUtils";
import { apiErrorDetail } from "../components/embedders/embedderModel";

const EmbedderList = () => {
  const navigate = useNavigate();
  const { canOn, canAnywhere } = useWorkspaceAccess();
  const workspaceColumn = useWorkspaceColumn();
  const { workspaceId, setWorkspaceId, params: workspaceParams } = useWorkspaceFilter();
  const [embedders, setEmbedders] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState(null);
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchEmbedders = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/embedders", { params: { ...queryParams, ...workspaceParams } });
      setEmbedders(response.data.data || []);
      const meta = response.data.meta || {};
      updatePaginationData(parseInt(meta.total_count ?? "0", 10), parseInt(meta.total_pages ?? "0", 10));
      setError("");
    } catch (err) {
      setError(isPermissionDenied(err) ? "Your role does not include access to embedders" : "Failed to load embedders");
    } finally {
      setLoading(false);
    }
  }, [queryParams, workspaceParams, updatePaginationData]);

  useEffect(() => {
    fetchEmbedders();
  }, [fetchEmbedders]);

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/embedders/${id}`);
      notify("Embedder deleted");
      fetchEmbedders();
    } catch (err) {
      // 409 names the data sources that still use it.
      notify(apiErrorDetail(err, "Failed to delete embedder"), "error");
    }
  };

  const columns = useMemo(
    () => [
      { field: "name", headerName: "Name", sortable: true, renderCell: (e) => e.attributes.name },
      { field: "model", headerName: "Model", sortable: true, renderCell: (e) => e.attributes.model },
      {
        field: "connection",
        headerName: "Connection",
        renderCell: (e) =>
          e.attributes.linked ? (
            <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
              <Chip size="small" label="Model provider" variant="outlined" />
              {e.attributes.llm_name || `#${e.attributes.llm_id}`}
            </Box>
          ) : (
            getEmbedderName(e.attributes.vendor)
          ),
      },
      { field: "privacy_score", headerName: "Privacy", renderCell: (e) => e.attributes.privacy_score },
      ...(workspaceColumn ? [workspaceColumn] : []),
    ],
    [workspaceColumn],
  );

  // Edit needs embedders:write and delete embedders:delete, both in the
  // embedder's Workspace: one in another Workspace is read-only.
  const canWrite = canAnywhere(P.EMBEDDERS_WRITE);
  const canDelete = canAnywhere(P.EMBEDDERS_DELETE);
  const writeOn = useCallback((e) => canOn(e?.attributes?.workspace_id, P.EMBEDDERS_WRITE), [canOn]);
  const deleteOn = useCallback((e) => canOn(e?.attributes?.workspace_id, P.EMBEDDERS_DELETE), [canOn]);
  const rowActions = useMemo(
    () => [
      { key: "edit", label: "Edit embedder", onClick: (e) => navigate(`/admin/embedders/edit/${e.id}`), hidden: (e) => !writeOn(e) },
      { key: "delete", label: "Delete embedder", onClick: (e) => setDeleteTarget(e), hidden: (e) => !deleteOn(e) },
    ],
    [navigate, writeOn, deleteOn],
  );

  if (error && embedders.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">Embedders</Typography>
        <Can permission={P.EMBEDDERS_WRITE} anywhere>
          <PrimaryButton variant="contained" startIcon={<AddIcon />} onClick={() => navigate("/admin/embedders/new")}>
            Add embedder
          </PrimaryButton>
        </Can>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          Embedders turn text into vectors for data sources. Most are created from the data source form; this page is
          where you review, fix and clean them up. An embedder either uses a model provider&apos;s connection or has its
          own. Its model cannot change while data sources use it, because their stored vectors came from that model.
        </Typography>
      </Box>
      <Box sx={{ p: 3 }}>
        <WorkspaceFilter value={workspaceId} onChange={setWorkspaceId} sx={{ mb: 2 }} />
        <DataTable
          {...tableProps}
          ariaLabel="Embedders"
          searchPlaceholder="Search embedders by name or model..."
          columns={columns}
          data={embedders}
          loading={loading}
          onRowClick={(e) => navigate(`/admin/embedders/${e.id}`)}
          actions={canWrite || canDelete ? rowActions : undefined}
          emptyState={
            !searchTerm ? (
              <EmptyStateWidget
                title="No embedders yet"
                description="Embedders are created when you set up a data source, or here."
                buttonText="Add embedder"
                buttonIcon={<AddIcon />}
                onButtonClick={canWrite ? () => navigate("/admin/embedders/new") : undefined}
              />
            ) : undefined
          }
        />
      </Box>

      <DeleteConfirmationDialog
        open={Boolean(deleteTarget)}
        resourcePath="embedders"
        objectLabel="embedder"
        item={deleteTarget ? { id: deleteTarget.id, name: deleteTarget.attributes?.name } : null}
        consequence="An embedder in use cannot be deleted: point those data sources at another embedder first."
        onConfirm={() => {
          const id = deleteTarget?.id;
          setDeleteTarget(null);
          handleDelete(id);
        }}
        onCancel={() => setDeleteTarget(null)}
      />
      <FeedbackSnackbar {...snackbarProps} />
    </Box>
  );
};

export default EmbedderList;
