import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { Typography, Alert, Box, Chip } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DataTable from "../components/common/DataTable";
import ActiveStatusDot from "../components/common/ActiveStatusDot";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import DeleteConfirmationDialog from "../components/common/DeleteConfirmationDialog";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import {
  TitleBox,
  PrimaryButton,
} from "../styles/sharedStyles";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import { isEnterpriseFeature, isPermissionDenied } from "../utils/apiErrors";
import Can from "../components/rbac/Can";
import { usePermissions } from "../context/PermissionsContext";
import { P } from "../rbac/permissions";

const ModelRouterList = () => {
  const navigate = useNavigate();
  const { can } = usePermissions();
  const [routers, setRouters] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState(null);
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchRouters = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/model-routers", { params: queryParams });
      setRouters(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching Model Routers", error);
      if (isPermissionDenied(error)) {
        setError("Your role does not include access to Model Routers");
      } else if (isEnterpriseFeature(error)) {
        setError("Model Routers require Enterprise Edition");
      } else {
        setError("Failed to load Model Routers");
      }
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchRouters();
  }, [fetchRouters]);

  const bulk = useBulkActions({
    items: routers,
    resource: "model-routers",
    singular: "model router",
    plural: "model routers",
    notify,
    refresh: fetchRouters,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/model-routers/${id}`);
      notify("Model Router deleted successfully");
      fetchRouters();
    } catch (error) {
      console.error("Error deleting Model Router", error);
      notify("Failed to delete Model Router", "error");
    }
  };

  const handleToggleActive = useCallback(async (router) => {
    try {
      await apiClient.patch(`/model-routers/${router.id}/toggle`);
      notify(`Model Router ${!router.attributes.active ? "activated" : "deactivated"} successfully`);
      fetchRouters();
    } catch (error) {
      console.error("Error toggling Model Router active state", error);
      notify("Failed to update Model Router active state", "error");
    }
  }, [fetchRouters, notify]);

  const handleRouterClick = (router) => {
    navigate(`/admin/model-routers/${router.id}`);
  };

  const handleAddRouter = () => {
    navigate("/admin/model-routers/new");
  };

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (router) => router.attributes.name },
    {
      field: "slug",
      headerName: "Slug",
      renderCell: (router) => <Chip label={router.attributes.slug} size="small" variant="outlined" />,
    },
    { field: "description", headerName: "Description", renderCell: (router) => router.attributes.description || "-" },
    { field: "pools", headerName: "Pools", renderCell: (router) => `${router.attributes.pools?.length || 0} pool(s)` },
    {
      field: "active",
      headerName: "Status",
      sortable: true,
      renderCell: (router) => <ActiveStatusDot active={router.attributes.active} showLabel />,
    },
  ], []);

  // Edit/delete need model-routers:write; the active toggle needs
  // model-routers:publish, which never implies write. Either permission
  // earns the actions column and bulk selection.
  const canWrite = can(P.MODEL_ROUTERS_WRITE);
  const canPublish = can(P.MODEL_ROUTERS_PUBLISH);
  const canManage = canWrite || canPublish;
  const rowActions = useMemo(() => [
    { key: "edit", label: "Edit Router", onClick: (router) => navigate(`/admin/model-routers/edit/${router.id}`), hidden: () => !canWrite },
    { key: "delete", label: "Delete Router", onClick: (router) => setDeleteTarget(router), hidden: () => !canWrite },
    {
      key: "toggle",
      label: (router) => `${router?.attributes?.active ? "Deactivate" : "Activate"} Router`,
      onClick: handleToggleActive,
      hidden: () => !canPublish,
    },
  ], [navigate, handleToggleActive, canWrite, canPublish]);

  const bulkActions = useMemo(
    () =>
      standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete, canToggle: canPublish }).filter(
        (action) => canWrite || action.key !== "delete",
      ),
    [bulk.run, bulk.requestDelete, canWrite, canPublish],
  );

  if (error && routers.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Model Routers</Typography>
          <Can permission={P.MODEL_ROUTERS_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddRouter}
            >
              Add Router
            </PrimaryButton>
          </Can>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            Model Routers enable intelligent request routing to LLM vendors based on model name patterns.
            Configure pools with glob patterns (e.g., "claude-*", "gpt-4*") to route requests to multiple vendors
            with round-robin or weighted selection algorithms.
          </Typography>
        </Box>
        <Box sx={{ p: 3 }}>
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <DataTable
            {...tableProps}
            ariaLabel="Model routers"
            searchPlaceholder="Search model routers by name..."
            columns={columns}
            data={routers}
            loading={loading}
            onRowClick={handleRouterClick}
            actions={canManage ? rowActions : undefined}
            {...(canManage ? bulk.selectionProps : {})}
            bulkActions={canManage ? bulkActions : undefined}
            emptyState={
              !searchTerm ? (
                <EmptyStateWidget
                  title="Create your first Model Router"
                  description="Model Routers let you define pools of LLM vendors and route requests based on model name patterns. Great for load balancing and failover."
                  buttonText="Add Router"
                  buttonIcon={<AddIcon />}
                  onButtonClick={canWrite ? handleAddRouter : undefined}
                />
              ) : undefined
            }
          />
        </Box>
      </>

      <DeleteConfirmationDialog
        open={Boolean(deleteTarget)}
        resourcePath="model-routers"
        objectLabel="model router"
        item={deleteTarget ? { id: deleteTarget.id, name: deleteTarget.attributes?.name } : null}
        consequence="Deleting it removes it from all of them; requests routed through it will fail."
        onConfirm={() => {
          const id = deleteTarget?.id;
          setDeleteTarget(null);
          handleDelete(id);
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath="model-routers"
        objectLabel="model router"
        objectLabelPlural="model routers"
        items={bulk.deleteDialogItems}
        consequence="Deleting them removes them from all of those; requests routed through them will fail."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </Box>
  );
};

export default ModelRouterList;
