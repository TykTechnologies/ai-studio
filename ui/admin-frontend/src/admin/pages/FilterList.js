import React, { useState, useEffect, useCallback, useMemo, memo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { Typography, CircularProgress, Alert, Box } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import DeleteConfirmationDialog from "../components/common/DeleteConfirmationDialog";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import EnterpriseFeatureBadge from "../components/common/EnterpriseFeatureBadge";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../styles/sharedStyles";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import useAdminData from "../hooks/useAdminData";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";

const FilterList = memo(() => {
  const navigate = useNavigate();
  const { config, loading: configLoading } = useAdminData();
  const [filters, setFilters] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState(null);
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchFilters = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/filters", { params: queryParams });
      // The filters endpoint returns a bare array, not a { data } envelope.
      setFilters(Array.isArray(response.data) ? response.data : response.data?.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching filters", error);
      setError("Failed to load filters");
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchFilters();
  }, [fetchFilters]);

  const bulk = useBulkActions({
    items: filters,
    resource: "filters",
    singular: "filter",
    plural: "filters",
    notify,
    refresh: fetchFilters,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/filters/${id}`);
      notify("Filter deleted successfully");
      fetchFilters();
    } catch (error) {
      console.error("Error deleting filter", error);
      notify("Failed to delete filter", "error");
    }
  };

  const handleFilterClick = useCallback((filter) => {
    navigate(`/admin/filters/${filter.id}`);
  }, [navigate]);

  const handleAddFilter = useCallback(() => {
    navigate("/admin/filters/new");
  }, [navigate]);

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (filter) => filter.attributes.name },
    { field: "description", headerName: "Description", renderCell: (filter) => filter.attributes.description },
    {
      field: "response_filter",
      headerName: "Type",
      renderCell: (filter) => (filter.attributes.response_filter ? "Response" : "Request"),
    },
  ], []);

  const rowActions = useMemo(() => [
    { key: "edit", label: "Edit filter", onClick: (filter) => navigate(`/admin/filters/edit/${filter.id}`) },
    { key: "delete", label: "Delete filter", onClick: (filter) => setDeleteTarget(filter) },
  ], [navigate]);

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete }),
    [bulk.run, bulk.requestDelete],
  );

  // Wait for config to load before checking enterprise status
  if (configLoading) {
    return <CircularProgress />;
  }

  if (error && filters.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  // Show enterprise badge if not enterprise edition
  if (config && !config.is_enterprise) {
    return (
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Filters</Typography>
        </TitleBox>
        <ContentBox>
          <EnterpriseFeatureBadge
            feature="Advanced Request Filtering & Scripting"
            description="Create custom filters using Tengo scripting to process and modify data before it reaches the LLM or after tool execution. Remove PII, enforce policies, and transform data with powerful scripting capabilities."
          />
        </ContentBox>
      </>
    );
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Filters</Typography>
          <Can permission={P.FILTERS_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddFilter}
            >
              Add filter
            </PrimaryButton>
          </Can>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Filters are used as a security layer to process and modify data before it is passed to the LLM. For example, filters can remove personally identifiable information to ensure privacy.</Typography>
        </Box>
        <ContentBox>
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <Can permission={P.FILTERS_WRITE}>
            {(canWrite) => (
              <DataTable
                {...tableProps}
                ariaLabel="Filters"
                searchPlaceholder="Search filters by name..."
                columns={columns}
                data={filters}
                loading={loading}
                onRowClick={handleFilterClick}
                actions={canWrite ? rowActions : undefined}
                {...(canWrite ? bulk.selectionProps : {})}
                bulkActions={canWrite ? bulkActions : undefined}
                emptyState={
                  !searchTerm ? (
                    <EmptyStateWidget
                      title="No filters created yet"
                      description="Click the button below to add a new filter."
                      buttonText="Add Filter"
                      buttonIcon={<AddIcon />}
                      onButtonClick={canWrite ? handleAddFilter : undefined}
                    />
                  ) : undefined
                }
              />
            )}
          </Can>
        </ContentBox>
      </>

      <DeleteConfirmationDialog
        open={Boolean(deleteTarget)}
        resourcePath="filters"
        objectLabel="filter"
        item={deleteTarget ? { id: deleteTarget.id, name: deleteTarget.attributes?.name } : null}
        consequence="Deleting it removes it from all of them; the LLMs and apps that use it will run without this filter."
        onConfirm={() => {
          const id = deleteTarget?.id;
          setDeleteTarget(null);
          handleDelete(id);
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath="filters"
        objectLabel="filter"
        objectLabelPlural="filters"
        items={bulk.deleteDialogItems}
        consequence="Deleting them removes them from all of those; the LLMs and apps that use them will run without these filters."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
});

FilterList.displayName = 'FilterList';

export default FilterList;
