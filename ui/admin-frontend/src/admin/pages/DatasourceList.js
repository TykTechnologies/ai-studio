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

const DatasourceList = () => {
  const navigate = useNavigate();
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
      const response = await apiClient.get("/datasources", { params: queryParams });
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
  }, [queryParams, updatePaginationData]);

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

  const handleCloneDatasource = useCallback(async (datasource) => {
    try {
      // Use server-side clone endpoint that preserves API keys
      const response = await apiClient.post(`/datasources/${datasource.id}/clone`);
      const newDatasourceId = response.data.data.id;

      notify("Data source cloned successfully");

      navigate(`/admin/datasources/edit/${newDatasourceId}`);
    } catch (error) {
      console.error("Error cloning datasource:", error);
      notify("Failed to clone data source", "error");
    }
  }, [navigate, notify]);

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
      field: "embed_vendor",
      headerName: "Embed Vendor",
      renderCell: (datasource) => (
        <Box sx={{ display: "flex", alignItems: "center" }}>
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
          {getEmbedderName(datasource.attributes.embed_vendor)}
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
  ], []);

  const rowActions = useMemo(() => [
    { key: "edit", label: "Edit data source", onClick: (d) => navigate(`/admin/datasources/edit/${d.id}`) },
    { key: "clone", label: "Clone data source", onClick: handleCloneDatasource },
    { key: "delete", label: "Delete data source", onClick: (d) => setDeleteTarget(d) },
    {
      key: "toggle",
      label: (d) => `${d?.attributes?.active ? "Deactivate" : "Activate"} data source`,
      onClick: handleToggleActive,
    },
  ], [navigate, handleCloneDatasource, handleToggleActive]);

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete, canToggle: true }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && datasources.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Data sources</Typography>
          <Can permission={P.DATASOURCES_WRITE}>
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
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <Can permission={P.DATASOURCES_WRITE}>
            {(canWrite) => (
              <DataTable
                {...tableProps}
                ariaLabel="Data sources"
                searchPlaceholder="Search data sources by name..."
                columns={columns}
                data={datasources}
                loading={loading}
                onRowClick={handleDatasourceClick}
                actions={canWrite ? rowActions : undefined}
                {...(canWrite ? bulk.selectionProps : {})}
                bulkActions={canWrite ? bulkActions : undefined}
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
            )}
          </Can>
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

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default DatasourceList;
