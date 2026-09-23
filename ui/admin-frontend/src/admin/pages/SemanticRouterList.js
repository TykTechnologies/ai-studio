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

const SemanticRouterList = () => {
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
      const response = await apiClient.get("/semantic-routers", { params: queryParams });
      setRouters(response.data.data || []);
      // Paging comes back in meta; headers are honoured too for parity with
      // the other list endpoints.
      const meta = response.data.meta || {};
      const totalCount = parseInt(meta.total_count ?? response.headers?.["x-total-count"] ?? "0", 10);
      const totalPages = parseInt(meta.total_pages ?? response.headers?.["x-total-pages"] ?? "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching Semantic Routers", error);
      if (isPermissionDenied(error)) {
        setError("Your role does not include access to Semantic Routers");
      } else if (isEnterpriseFeature(error)) {
        setError("Semantic Routers require Enterprise Edition");
      } else {
        setError("Failed to load Semantic Routers");
      }
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchRouters();
  }, [fetchRouters]);

  // There is no bulk endpoint for semantic routers: bulk delete is done one
  // request per router, and bulk activate/deactivate is not offered.
  const bulk = useBulkActions({
    items: routers,
    resource: "semantic-routers",
    singular: "semantic router",
    plural: "semantic routers",
    notify,
    refresh: fetchRouters,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/semantic-routers/${id}`);
      notify("Semantic Router deleted successfully");
      fetchRouters();
    } catch (error) {
      console.error("Error deleting Semantic Router", error);
      notify("Failed to delete Semantic Router", "error");
    }
  };

  const handleToggleActive = useCallback(async (router) => {
    const active = !router.attributes.active;
    try {
      await apiClient.patch(`/semantic-routers/${router.id}/toggle`, { active });
      notify(`Semantic Router ${active ? "activated" : "deactivated"} successfully`);
      fetchRouters();
    } catch (error) {
      console.error("Error toggling Semantic Router active state", error);
      notify("Failed to update Semantic Router active state", "error");
    }
  }, [fetchRouters, notify]);

  const handleRouterClick = (router) => {
    navigate(`/admin/semantic-routers/${router.id}`);
  };

  const handleAddRouter = () => {
    navigate("/admin/semantic-routers/new");
  };

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (router) => router.attributes.name },
    {
      field: "slug",
      headerName: "Slug",
      renderCell: (router) => <Chip label={router.attributes.slug} size="small" variant="outlined" />,
    },
    {
      field: "models",
      headerName: "Model",
      renderCell: (router) => {
        const models = router.attributes.models || [];
        const first = models[0] || `${router.attributes.slug}/auto`;
        return (
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <Typography variant="body2" sx={{ fontFamily: "monospace" }}>{first}</Typography>
            {models.length > 1 && (
              <Typography variant="caption" color="text.secondary">{`+${models.length - 1}`}</Typography>
            )}
          </Box>
        );
      },
    },
    { field: "routes", headerName: "Routes", renderCell: (router) => `${router.attributes.routes?.length || 0} route(s)` },
    {
      field: "mode",
      headerName: "Mode",
      renderCell: (router) =>
        router.attributes.settings?.mode === "shadow" ? (
          <Chip label="Shadow" size="small" color="warning" variant="outlined" />
        ) : (
          "Enforce"
        ),
    },
    {
      field: "active",
      headerName: "Status",
      sortable: true,
      renderCell: (router) => <ActiveStatusDot active={router.attributes.active} showLabel />,
    },
  ], []);

  // Edit needs semantic-routers:write, delete semantic-routers:delete, and
  // the active toggle semantic-routers:publish (which never implies write).
  // Any of them earns the actions column.
  const canWrite = can(P.SEMANTIC_ROUTERS_WRITE);
  const canDelete = can(P.SEMANTIC_ROUTERS_DELETE);
  const canPublish = can(P.SEMANTIC_ROUTERS_PUBLISH);
  const canManage = canWrite || canDelete || canPublish;
  const rowActions = useMemo(() => [
    { key: "edit", label: "Edit Router", onClick: (router) => navigate(`/admin/semantic-routers/edit/${router.id}`), hidden: () => !canWrite },
    { key: "delete", label: "Delete Router", onClick: (router) => setDeleteTarget(router), hidden: () => !canDelete },
    {
      key: "toggle",
      label: (router) => `${router?.attributes?.active ? "Deactivate" : "Activate"} Router`,
      onClick: handleToggleActive,
      hidden: () => !canPublish,
    },
  ], [navigate, handleToggleActive, canWrite, canDelete, canPublish]);

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete, canToggle: false }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && routers.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Semantic Routers</Typography>
          <Can permission={P.SEMANTIC_ROUTERS_WRITE}>
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
            Semantic Routers pick a route for each request by classifying the prompt: keywords
            first, then similarity to example prompts, then an optional LLM judge, falling back to
            a default route. Each route sends the request to an LLM and model, or to a Model Router.
            Clients call a router with the model written as <code>&lt;slug&gt;/auto</code>.
          </Typography>
        </Box>
        <Box sx={{ p: 3 }}>
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <DataTable
            {...tableProps}
            ariaLabel="Semantic routers"
            searchPlaceholder="Search semantic routers by name..."
            columns={columns}
            data={routers}
            loading={loading}
            onRowClick={handleRouterClick}
            actions={canManage ? rowActions : undefined}
            {...(canDelete ? bulk.selectionProps : {})}
            bulkActions={canDelete ? bulkActions : undefined}
            emptyState={
              !searchTerm ? (
                <EmptyStateWidget
                  title="Create your first Semantic Router"
                  description="Semantic Routers send each prompt to the right model: hard questions to a strong model, simple ones to a cheap one, code to a code model."
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
        resourcePath="semantic-routers"
        objectLabel="semantic router"
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
        resourcePath="semantic-routers"
        objectLabel="semantic router"
        objectLabelPlural="semantic routers"
        items={bulk.deleteDialogItems}
        consequence="Deleting them removes them from all of those; requests routed through them will fail."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </Box>
  );
};

export default SemanticRouterList;
