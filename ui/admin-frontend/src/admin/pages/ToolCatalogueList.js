import React, { useState, useEffect, useCallback, memo, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { Typography, Box, Chip } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../styles/sharedStyles";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";

const chips = (items, labelOf) => (
  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
    {(items || []).map((item) => (
      <Chip
        key={item.id}
        label={labelOf(item)}
        size="small"
        sx={{ marginRight: 0.5, marginBottom: 0.5 }}
      />
    ))}
  </Box>
);

const ToolCatalogueList = memo(() => {
  const [toolCatalogues, setToolCatalogues] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const navigate = useNavigate();

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchToolCatalogues = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/tool-catalogues", { params: queryParams });
      setToolCatalogues(response.data?.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching tool catalogues", error);
      setError("Failed to load tool catalogs");
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchToolCatalogues();
  }, [fetchToolCatalogues]);

  // No /bulk endpoint for tool catalogues: deletes go one request per item.
  const bulk = useBulkActions({
    items: toolCatalogues,
    resource: "tool-catalogues",
    singular: "tool catalog",
    plural: "tool catalogs",
    notify,
    refresh: fetchToolCatalogues,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/tool-catalogues/${id}`);
      notify("Tool catalog deleted successfully");
      fetchToolCatalogues();
    } catch (error) {
      console.error("Error deleting tool catalogue", error);
      notify("Failed to delete tool catalog", "error");
    }
  };

  const handleEdit = useCallback((id) => {
    navigate(`/admin/catalogs/tools/edit/${id}`);
  }, [navigate]);

  const handleAddToolCatalogue = useCallback(() => {
    navigate("/admin/catalogs/tools/new");
  }, [navigate]);

  const handleCatalogueClick = useCallback((catalogue) => {
    navigate(`/admin/catalogs/tools/${catalogue.id}`);
  }, [navigate]);

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (catalogue) => catalogue.attributes.name },
    { field: "short_description", headerName: "Description", renderCell: (catalogue) => catalogue.attributes.short_description },
    {
      field: "tools",
      headerName: "Tools",
      renderCell: (catalogue) => chips(catalogue.attributes.tools, (tool) => tool.attributes.name),
    },
    {
      field: "tags",
      headerName: "Tags",
      renderCell: (catalogue) => chips(catalogue.attributes.tags, (tag) => tag.attributes.name),
    },
  ], []);

  const rowActions = [
    { key: "edit", label: "Edit", onClick: (catalogue) => handleEdit(catalogue.id) },
    { key: "delete", label: "Delete", onClick: (catalogue) => handleDelete(catalogue.id) },
  ];

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && toolCatalogues.length === 0)
    return <Typography color="error">{error}</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Tool catalogs</Typography>
        <Can permission={P.TOOL_CATALOGUES_WRITE}>
          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddToolCatalogue}
          >
            Add catalog
          </PrimaryButton>
        </Can>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of tools that you can assign to specific teams to manage access easily.</Typography>
      </Box>
      <ContentBox>
        <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
        <Can permission={P.TOOL_CATALOGUES_WRITE}>
          {(canWrite) => (
            <DataTable
              {...tableProps}
              ariaLabel="Tool catalogs"
              searchPlaceholder="Search tool catalogs by name..."
              columns={columns}
              data={toolCatalogues}
              loading={loading}
              onRowClick={handleCatalogueClick}
              actions={canWrite ? rowActions : undefined}
              {...(canWrite ? bulk.selectionProps : {})}
              bulkActions={canWrite ? bulkActions : undefined}
              emptyState={
                !searchTerm ? (
                  <EmptyStateWidget
                    title="No tool catalogs found"
                    description="Click the button below to add a new tool catalog."
                    buttonText="Add Tool Catalog"
                    buttonIcon={<AddIcon />}
                    onButtonClick={canWrite ? handleAddToolCatalogue : undefined}
                  />
                ) : undefined
              }
            />
          )}
        </Can>
      </ContentBox>

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath={null}
        objectLabel="tool catalog"
        objectLabelPlural="tool catalogs"
        items={bulk.deleteDialogItems}
        consequence="Teams assigned these catalogs lose access to the tools in them."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
});

ToolCatalogueList.displayName = 'ToolCatalogueList';

export default ToolCatalogueList;
