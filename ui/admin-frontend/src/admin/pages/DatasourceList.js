import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { Typography, Alert, Box, Chip } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DataTable from "../components/common/DataTable";
import ActiveStatusDot from "../components/common/ActiveStatusDot";
import PrivacyLevelChip from "../components/common/privacy/PrivacyLevelChip";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import DeleteConfirmationDialog from "../components/common/DeleteConfirmationDialog";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../styles/sharedStyles";
import {
  getVectorStoreName,
  getVectorStoreLogo,
  getEmbedderName,
  getEmbedderLogo,
  fetchVendors,
} from "../utils/vendorUtils";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";
import WorkspacePickDialog from "../components/workspaces/WorkspacePickDialog";
import useDatasourceClone from "../components/datasources/useDatasourceClone";
import {
  useWorkspaceColumn,
  useWorkspaceFilter,
  useWorkspaceAccess,
  WorkspaceFilter,
} from "../components/workspaces/workspaceFields";

const DatasourceList = () => {
  const navigate = useNavigate();
  const { canOn, canAnywhere } = useWorkspaceAccess();
  const workspaceColumn = useWorkspaceColumn();
  const { workspaceId, setWorkspaceId, params: workspaceParams } = useWorkspaceFilter();
  const [datasources, setDatasources] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState(null);
  // Populated once so the vendor name/logo helpers can resolve display names.
  const [, setVendors] = useState({ embedders: [], vectorStores: [] });
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchDatasources = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/datasources", { params: { ...queryParams, ...workspaceParams } });
      setDatasources(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching datasources", error);
      setError("Failed to load data sources");
    } finally {
      setLoading(false);
    }
  }, [queryParams, workspaceParams, updatePaginationData]);

  useEffect(() => {
    const initializePage = async () => {
      const fetchedVendors = await fetchVendors();
      setVendors(fetchedVendors);
      fetchDatasources();
    };
    initializePage();
  }, [fetchDatasources]);

  const bulk = useBulkActions({
    items: datasources,
    resource: "datasources",
    singular: "data source",
    plural: "data sources",
    notify,
    refresh: fetchDatasources,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/datasources/${id}`);
      notify("Data source deleted successfully");
      fetchDatasources();
    } catch (error) {
      console.error("Error deleting datasource", error);
      notify("Failed to delete data source", "error");
    }
  };

  const handleToggleActive = useCallback(async (datasource) => {
    try {
      const updatedDatasource = {
        data: {
          type: "Datasource",
          id: datasource.id,
          attributes: {
            ...datasource.attributes,
            active: !datasource.attributes.active,
            tags: (datasource.attributes.tags || []).map((tag) => tag.attributes.name),
          },
        },
      };
      await apiClient.patch(`/datasources/${datasource.id}`, updatedDatasource);
      notify(
        `Data source ${updatedDatasource.data.attributes.active ? "activated" : "deactivated"} successfully`,
      );
      fetchDatasources();
    } catch (error) {
      console.error("Error toggling datasource active state", error);
      notify("Failed to update data source active state", "error");
    }
  }, [fetchDatasources, notify]);

  // Server-side clone (keeps API keys); a caller who may write in several
  // Workspaces picks where the copy goes.
  const onCloned = useCallback((newDatasourceId) => {
    notify("Data source cloned successfully");
    navigate(`/admin/datasources/edit/${newDatasourceId}`);
  }, [navigate, notify]);
  const onCloneError = useCallback((message) => notify(message, "error"), [notify]);
  const { startClone, dialogProps: cloneDialogProps } = useDatasourceClone({ onCloned, onError: onCloneError });
  const handleCloneDatasource = useCallback((datasource) => startClone(datasource.id), [startClone]);

  const handleDatasourceClick = (datasource) => {
    navigate(`/admin/datasources/${datasource.id}`);
  };

  const handleAddDatasource = () => {
    navigate("/admin/datasources/new");
  };

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (d) => d.attributes.name },
    { field: "short_description", headerName: "Short Description", renderCell: (d) => d.attributes.short_description },
    {
      field: "db_source_type",
      headerName: "DB Source Type",
      renderCell: (datasource) => (
        <Box sx={{ display: "flex", alignItems: "center" }}>
          <img
            src={getVectorStoreLogo(datasource.attributes.db_source_type)}
            alt={getVectorStoreName(datasource.attributes.db_source_type)}
            style={{
              width: 24,
              height: 24,
              marginRight: 8,
              objectFit: "contain",
            }}
          />
          {getVectorStoreName(datasource.attributes.db_source_type)}
        </Box>
      ),
    },
    {
      field: "embedder",
      headerName: "Embedder",
      renderCell: (datasource) => (
        <Box sx={{ display: "flex", alignItems: "center" }}>
          {datasource.attributes.embed_vendor && (
            <img
              src={getEmbedderLogo(datasource.attributes.embed_vendor)}
              alt={getEmbedderName(datasource.attributes.embed_vendor)}
              style={{
                width: 24,
                height: 24,
                marginRight: 8,
                objectFit: "contain",
              }}
            />
          )}
          {datasource.attributes.embedder_name ||
            getEmbedderName(datasource.attributes.embed_vendor) ||
            "Not set"}
        </Box>
      ),
    },
    {
      field: "privacy_score",
      headerName: "Privacy Level",
      sortable: true,
      renderCell: (d) => <PrivacyLevelChip score={d.attributes.privacy_score} />,
    },
    {
      field: "tags",
      headerName: "Tags",
      renderCell: (datasource) =>
        (datasource.attributes.tags || []).map((tag) => (
          <Chip
            key={tag.id}
            label={tag.attributes.name}
            size="small"
            sx={{ mr: 0.5, mb: 0.5 }}
          />
        )),
    },
    {
      field: "active",
      headerName: "Active",
      sortable: true,
      renderCell: (d) => <ActiveStatusDot active={d.attributes.active} />,
    },
    ...(workspaceColumn ? [workspaceColumn] : []),
  ], [workspaceColumn]);

  // Edit/clone/delete need datasources:write; the active toggle needs
  // datasources:publish, which never implies write. Either permission earns
  // the actions column. Both are checked in each data source's Workspace:
  // one in another Workspace is read-only.
  const canWrite = canAnywhere(P.DATASOURCES_WRITE);
  const canPublish = canAnywhere(P.DATASOURCES_PUBLISH);
  const canManage = canWrite || canPublish;
  const writeOn = useCallback((d) => canOn(d?.attributes?.workspace_id, P.DATASOURCES_WRITE), [canOn]);
  const publishOn = useCallback((d) => canOn(d?.attributes?.workspace_id, P.DATASOURCES_PUBLISH), [canOn]);

  const rowActions = useMemo(() => [
    { key: "edit", label: "Edit data source", onClick: (d) => navigate(`/admin/datasources/edit/${d.id}`), hidden: (d) => !writeOn(d) },
    { key: "clone", label: "Clone data source", onClick: handleCloneDatasource, hidden: (d) => !writeOn(d) },
    { key: "delete", label: "Delete data source", onClick: (d) => setDeleteTarget(d), hidden: (d) => !writeOn(d) },
    {
      key: "toggle",
      label: (d) => `${d?.attributes?.active ? "Deactivate" : "Activate"} data source`,
      onClick: handleToggleActive,
      hidden: (d) => !publishOn(d),
    },
  ], [navigate, handleCloneDatasource, handleToggleActive, writeOn, publishOn]);

  const bulkActions = useMemo(
    () =>
      standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete, canToggle: canPublish }).filter(
        (action) => canWrite || action.key !== "delete",
      ),
    [bulk.run, bulk.requestDelete, canWrite, canPublish],
  );

  if (error && datasources.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="var(--studio-header-height)">
          <Typography variant="headingXLarge">Data sources</Typography>
          <Can permission={P.DATASOURCES_WRITE} anywhere>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddDatasource}
            >
              Add data source
            </PrimaryButton>
          </Can>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Data sources let you store and access information to enhance AI conversations using Retrieval Augmented Generation (RAG). By using embedding providers to convert content into searchable vectors, your AI can deliver more accurate, informed, and engaging responses.</Typography>
        </Box>
        <ContentBox>
          <WorkspaceFilter value={workspaceId} onChange={setWorkspaceId} sx={{ mb: 2 }} />
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <DataTable
            {...tableProps}
            ariaLabel="Data sources"
            searchPlaceholder="Search data sources by name..."
            columns={columns}
            data={datasources}
            loading={loading}
            onRowClick={handleDatasourceClick}
            actions={canManage ? rowActions : undefined}
            {...(canManage ? bulk.selectionProps : {})}
            bulkActions={canManage ? bulkActions : undefined}
            emptyState={
              !searchTerm ? (
                <EmptyStateWidget
                  title="No vector DBs yet"
                  description="Vector data sources are used to store and retrieve data to enhance LLM response effectiveness. These can be created using embedding providers that vectorize the content you wish to search, and make for an excellent way to enhance your chat room value for your users, or to better inform responses in your AI Applications."
                  buttonText="Add data source"
                  buttonIcon={<AddIcon />}
                  onButtonClick={canWrite ? handleAddDatasource : undefined}
                />
              ) : undefined
            }
          />
        </ContentBox>
      </>

      <DeleteConfirmationDialog
        open={Boolean(deleteTarget)}
        resourcePath="datasources"
        objectLabel="data source"
        item={deleteTarget ? { id: deleteTarget.id, name: deleteTarget.attributes?.name } : null}
        consequence="Deleting it removes it from all of them; apps and chats that use it lose access to its data."
        onConfirm={() => {
          const id = deleteTarget?.id;
          setDeleteTarget(null);
          handleDelete(id);
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath="datasources"
        objectLabel="data source"
        objectLabelPlural="data sources"
        items={bulk.deleteDialogItems}
        consequence="Deleting them removes them from all of those; apps and chats that use them lose access to their data."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <WorkspacePickDialog {...cloneDialogProps} />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default DatasourceList;
