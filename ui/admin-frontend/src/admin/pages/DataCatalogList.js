import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { Typography, Alert, Box, Chip } from "@mui/material";
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

const DataCatalogList = () => {
  const navigate = useNavigate();
  const [dataCatalogs, setDataCatalogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchDataCatalogs = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/data-catalogues", { params: queryParams });
      setDataCatalogs(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching data catalogs", error);
      setError("Failed to load data catalogs");
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchDataCatalogs();
  }, [fetchDataCatalogs]);

  // No /bulk endpoint for data catalogues: deletes go one request per item.
  const bulk = useBulkActions({
    items: dataCatalogs,
    resource: "data-catalogues",
    singular: "data catalog",
    plural: "data catalogs",
    notify,
    refresh: fetchDataCatalogs,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/data-catalogues/${id}`);
      notify("Data catalog deleted successfully");
      fetchDataCatalogs();
    } catch (error) {
      console.error("Error deleting data catalog", error);
      notify("Failed to delete data catalog", "error");
    }
  };

  const handleEdit = (id) => {
    navigate(`/admin/catalogs/data/edit/${id}`);
  };

  const handleAddDataCatalog = () => {
    navigate("/admin/catalogs/data/new");
  };

  const handleCatalogClick = (catalog) => {
    navigate(`/admin/catalogs/data/${catalog.id}`);
  };

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (catalog) => catalog.attributes.name },
    { field: "short_description", headerName: "Description", renderCell: (catalog) => catalog.attributes.short_description },
    {
      field: "datasources",
      headerName: "Data sources",
      renderCell: (catalog) => chips(catalog.attributes.datasources, (datasource) => datasource.attributes.name),
    },
    {
      field: "tags",
      headerName: "Tags",
      renderCell: (catalog) => chips(catalog.attributes.tags, (tag) => tag.attributes.name),
    },
  ], []);

  const rowActions = [
    { key: "edit", label: "Edit data catalog", onClick: (catalog) => handleEdit(catalog.id) },
    { key: "delete", label: "Delete data catalog", onClick: (catalog) => handleDelete(catalog.id) },
  ];

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && dataCatalogs.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Data catalogs</Typography>
          <Can permission={P.DATA_CATALOGUES_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddDataCatalog}
            >
              Add catalog
            </PrimaryButton>
          </Can>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of data sources that you can assign to specific teams to manage access easily.</Typography>
        </Box>
        <ContentBox>
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <Can permission={P.DATA_CATALOGUES_WRITE}>
            {(canWrite) => (
              <DataTable
                {...tableProps}
                ariaLabel="Data catalogs"
                searchPlaceholder="Search data catalogs by name..."
                columns={columns}
                data={dataCatalogs}
                loading={loading}
                onRowClick={handleCatalogClick}
                actions={canWrite ? rowActions : undefined}
                {...(canWrite ? bulk.selectionProps : {})}
                bulkActions={canWrite ? bulkActions : undefined}
                emptyState={
                  !searchTerm ? (
                    <EmptyStateWidget
                      title="No data catalogs found"
                      description="Click the button below to add a new data catalog."
                      buttonText="Add Data Catalog"
                      buttonIcon={<AddIcon />}
                      onButtonClick={canWrite ? handleAddDataCatalog : undefined}
                    />
                  ) : undefined
                }
              />
            )}
          </Can>
        </ContentBox>
      </>

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath={null}
        objectLabel="data catalog"
        objectLabelPlural="data catalogs"
        items={bulk.deleteDialogItems}
        consequence="Teams assigned these catalogs lose access to the data sources in them."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default DataCatalogList;
