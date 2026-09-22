import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { Typography, Alert, Box } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import { CredentialStatusDot } from "../components/llms/CredentialStatusIndicator";
import PrivacyLevelChip from "../components/common/privacy/PrivacyLevelChip";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import DeleteConfirmationDialog from "../components/common/DeleteConfirmationDialog";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import { TitleBox, PrimaryButton } from "../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../utils/vendorLogos";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import Can from "../components/rbac/Can";
import { usePermissions } from "../context/PermissionsContext";
import { P } from "../rbac/permissions";

const LLMList = () => {
  const navigate = useNavigate();
  const { can } = usePermissions();
  const [llms, setLLMs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState(null);
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchLLMs = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/llms", { params: queryParams });
      setLLMs(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching LLMs", error);
      setError("Failed to load LLMs");
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchLLMs();
  }, [fetchLLMs]);

  const bulk = useBulkActions({
    items: llms,
    resource: "llms",
    singular: "LLM provider",
    plural: "LLM providers",
    notify,
    refresh: fetchLLMs,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/llms/${id}`);
      notify("LLM provider deleted successfully");
      fetchLLMs();
    } catch (error) {
      console.error("Error deleting LLM", error);
      notify("Failed to delete LLM", "error");
    }
  };

  // Activate/deactivate are publish-gated endpoints, so a publish-only role
  // can flip the switch without the write permission PATCH /llms/:id needs.
  const handleToggleActive = useCallback(async (llm) => {
    const activating = !llm.attributes.active;
    try {
      await apiClient.post(`/llms/${llm.id}/${activating ? "activate" : "deactivate"}`);
      notify(`LLM ${activating ? "activated" : "deactivated"} successfully`);
      fetchLLMs();
    } catch (error) {
      console.error("Error toggling LLM active state", error);
      notify("Failed to update LLM active state", "error");
    }
  }, [fetchLLMs, notify]);

  const handleLLMClick = (llm) => {
    navigate(`/admin/llms/${llm.id}`);
  };

  const handleAddLLM = () => {
    navigate("/admin/llms/new");
  };

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (llm) => llm.attributes.name },
    { field: "short_description", headerName: "Short Description", renderCell: (llm) => llm.attributes.short_description },
    {
      field: "vendor",
      headerName: "Vendor",
      sortable: true,
      renderCell: (llm) => (
        <Box sx={{ display: "flex", alignItems: "center" }}>
          <img
            src={getVendorLogo(llm.attributes.vendor)}
            alt=""
            style={{
              width: 24,
              height: 24,
              marginRight: 8,
              objectFit: "contain",
            }}
            onError={(e) => {
              e.target.onerror = null;
              e.target.src =
                process.env.PUBLIC_URL +
                "/images/placeholder-logo.png";
            }}
          />
          {getVendorName(llm.attributes.vendor)}
        </Box>
      ),
    },
    {
      field: "privacy_score",
      headerName: "Privacy Level",
      sortable: true,
      renderCell: (llm) => <PrivacyLevelChip score={llm.attributes.privacy_score} />,
    },
    {
      field: "active",
      headerName: "Active",
      sortable: true,
      renderCell: (llm) => (
        // The dot alone read as live even when the provider pointed at an
        // empty secret, so a fresh instance looked fully configured and the
        // first call failed.
        <CredentialStatusDot
          active={llm.attributes.active}
          status={llm.attributes.credential_status}
          reference={llm.attributes.credential_ref}
        />
      ),
    },
  ], []);

  // Edit/delete need llms:write; the active toggle needs llms:publish, which
  // never implies write. Either permission earns the actions column.
  const canWrite = can(P.LLMS_WRITE);
  const canPublish = can(P.LLMS_PUBLISH);
  const canManage = canWrite || canPublish;

  const rowActions = useMemo(() => [
    { key: "edit", label: "Edit LLM provider", onClick: (llm) => navigate(`/admin/llms/edit/${llm.id}`), hidden: () => !canWrite },
    { key: "delete", label: "Delete LLM provider", onClick: (llm) => setDeleteTarget(llm), hidden: () => !canWrite },
    {
      key: "toggle",
      label: (llm) => `${llm?.attributes?.active ? "Deactivate" : "Activate"} LLM provider`,
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

  if (error && llms.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">LLM providers</Typography>
          <Can permission={P.LLMS_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddLLM}
            >
              Add LLM provider
            </PrimaryButton>
          </Can>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">LLM providers power AI chats and can be made available to developers in the portal and gateway when set to Active. To control access, each LLM provider must be part of a catalog to be used by specific teams.</Typography>
        </Box>
        <Box sx={{ p: 3 }}>
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <DataTable
            {...tableProps}
            ariaLabel="LLM providers"
            searchPlaceholder="Search LLM providers by name..."
            columns={columns}
            data={llms}
            loading={loading}
            onRowClick={handleLLMClick}
            actions={canManage ? rowActions : undefined}
            {...(canManage ? bulk.selectionProps : {})}
            bulkActions={canManage ? bulkActions : undefined}
            emptyState={
              !searchTerm ? (
                <EmptyStateWidget
                  title="Want to start working with your favourite LLM?"
                  description="Click the button below to add a new LLM configuration to use in your chat room."
                  buttonText="Add LLM provider"
                  buttonIcon={<AddIcon />}
                  onButtonClick={canWrite ? handleAddLLM : undefined}
                />
              ) : undefined
            }
          />
        </Box>
      </>

      <DeleteConfirmationDialog
        open={Boolean(deleteTarget)}
        resourcePath="llms"
        objectLabel="LLM provider"
        item={deleteTarget ? { id: deleteTarget.id, name: deleteTarget.attributes?.name } : null}
        consequence="Deleting it removes it from all of them; apps that only have this LLM provider will stop working."
        onConfirm={() => {
          const id = deleteTarget?.id;
          setDeleteTarget(null);
          handleDelete(id);
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath="llms"
        objectLabel="LLM provider"
        objectLabelPlural="LLM providers"
        items={bulk.deleteDialogItems}
        consequence="Deleting them removes them from all of those; apps that only have these LLM providers will stop working."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </Box>
  );
};

export default LLMList;
