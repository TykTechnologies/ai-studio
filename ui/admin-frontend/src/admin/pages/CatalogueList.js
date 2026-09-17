import React, { useState, useEffect, useCallback, memo, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Typography,
  Alert,
  Box,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Select,
  FormControl,
  InputLabel,
  MenuItem,
  Chip,
} from "@mui/material";
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
import { listAll } from "../utils/listAll";

const CatalogueList = memo(() => {
  const navigate = useNavigate();
  const [catalogues, setCatalogues] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selectedCatalogue, setSelectedCatalogue] = useState(null);
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const [modalOpen, setModalOpen] = useState(false);
  const [modalType, setModalType] = useState("");
  const [availableLLMs, setAvailableLLMs] = useState([]);
  const [catalogueLLMs, setCatalogueLLMs] = useState([]);
  const [selectedLLM, setSelectedLLM] = useState("");

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchCatalogues = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/catalogues", { params: queryParams });
      setCatalogues(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching catalogues", error);
      setError("Failed to load catalogs");
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchCatalogues();
  }, [fetchCatalogues]);

  // No /bulk endpoint for catalogues: deletes go one request per item.
  const bulk = useBulkActions({
    items: catalogues,
    resource: "catalogues",
    singular: "catalog",
    plural: "catalogs",
    notify,
    refresh: fetchCatalogues,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/catalogues/${id}`);
      setCatalogues((current) => current.filter((catalogue) => catalogue.id !== id));
      notify("Catalog deleted successfully");
    } catch (error) {
      console.error("Error deleting catalog", error);
      notify("Failed to delete catalog", "error");
    }
  };

  const handleEdit = (id) => {
    navigate(`/admin/catalogs/llms/edit/${id}`);
  };

  const handleAddLLM = async () => {
    try {
      await apiClient.post(`/catalogues/${selectedCatalogue.id}/llms`, {
        data: { id: selectedLLM, type: "LLM" },
      });
      notify("LLM added to catalog successfully");
      setModalOpen(false);
      fetchCatalogues();
    } catch (error) {
      console.error("Error adding LLM to catalog", error);
      notify("Failed to add LLM to catalog", "error");
    }
  };

  const handleRemoveLLM = async () => {
    try {
      await apiClient.delete(
        `/catalogues/${selectedCatalogue.id}/llms/${selectedLLM}`,
      );
      notify("LLM removed from catalog successfully");
      setModalOpen(false);
      fetchCatalogues();
    } catch (error) {
      console.error("Error removing LLM from catalog", error);
      notify("Failed to remove LLM from catalog", "error");
    }
  };

  const handleOpenModal = async (type, catalogue) => {
    setSelectedCatalogue(catalogue);
    setModalType(type);
    if (type === "add") {
      try {
        const response = await listAll(apiClient, "/llms");
        setAvailableLLMs(
          response.data.data.filter((llm) => llm.attributes.active),
        );
      } catch (error) {
        console.error("Error fetching LLMs", error);
      }
    } else if (type === "remove") {
      try {
        const response = await apiClient.get(
          `/catalogues/${catalogue.id}/llms`,
        );
        setCatalogueLLMs(response.data.data);
      } catch (error) {
        console.error("Error fetching catalog LLMs", error);
      }
    }
    setModalOpen(true);
  };

  const handleCloseModal = () => {
    setModalOpen(false);
    setSelectedLLM("");
  };

  const handleAddCatalogue = () => {
    navigate("/admin/catalogs/llms/new");
  };

  const handleCatalogueClick = (catalogue) => {
    navigate(`/admin/catalogs/llms/${catalogue.id}`);
  };

  const getLLMNames = useCallback((catalogue) => {
    if (catalogue.attributes.llm_names) {
      return catalogue.attributes.llm_names;
    } else if (
      catalogue.attributes.llms &&
      Array.isArray(catalogue.attributes.llms)
    ) {
      return catalogue.attributes.llms.map((llm) => llm.attributes.name);
    }
    return [];
  }, []);

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (catalogue) => catalogue.attributes.name },
    {
      field: "llms",
      headerName: "LLM providers",
      renderCell: (catalogue) => (
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
          {getLLMNames(catalogue).map((llmName, index) => (
            <Chip
              key={index}
              label={llmName}
              size="small"
              sx={{ marginRight: 0.5, marginBottom: 0.5 }}
            />
          ))}
        </Box>
      ),
    },
  ], [getLLMNames]);

  const rowActions = [
    { key: "edit", label: "Edit catalog", onClick: (catalogue) => handleEdit(catalogue.id) },
    { key: "delete", label: "Delete catalog", onClick: (catalogue) => handleDelete(catalogue.id) },
    { key: "add-llm", label: "Add LLM to catalog", onClick: (catalogue) => handleOpenModal("add", catalogue) },
    { key: "remove-llm", label: "Remove LLM from catalog", onClick: (catalogue) => handleOpenModal("remove", catalogue) },
  ];

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && catalogues.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">LLM catalogs</Typography>
          <Can permission={P.CATALOGUES_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddCatalogue}
            >
              Add catalog
            </PrimaryButton>
          </Can>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of LLM providers that you can assign to specific teams to manage access easily.</Typography>
        </Box>
        <ContentBox>
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <Can permission={P.CATALOGUES_WRITE}>
            {(canWrite) => (
              <DataTable
                {...tableProps}
                ariaLabel="LLM catalogs"
                searchPlaceholder="Search catalogs by name..."
                columns={columns}
                data={catalogues}
                loading={loading}
                onRowClick={handleCatalogueClick}
                actions={canWrite ? rowActions : undefined}
                {...(canWrite ? bulk.selectionProps : {})}
                bulkActions={canWrite ? bulkActions : undefined}
                emptyState={
                  !searchTerm ? (
                    <EmptyStateWidget
                      title="No catalogs found"
                      description="Click the button below to add a new catalog."
                      buttonText="Add Catalog"
                      buttonIcon={<AddIcon />}
                      onButtonClick={canWrite ? handleAddCatalogue : undefined}
                    />
                  ) : undefined
                }
              />
            )}
          </Can>
        </ContentBox>
      </>

      <Dialog open={modalOpen} onClose={handleCloseModal}>
        <DialogTitle>
          {modalType === "add"
            ? "Add LLM provider to catalog"
            : "Remove LLM provider from catalog"}
        </DialogTitle>
        <DialogContent>
          <FormControl fullWidth sx={{ mt: 2 }}>
            <InputLabel id="llm-select-label">Select LLM</InputLabel>
            <Select
              labelId="llm-select-label"
              value={selectedLLM}
              onChange={(e) => setSelectedLLM(e.target.value)}
              label="Select LLM"
            >
              {modalType === "add"
                ? availableLLMs.map((llm) => (
                    <MenuItem key={llm.id} value={llm.id}>
                      {llm.attributes.name}
                    </MenuItem>
                  ))
                : catalogueLLMs.map((llm) => (
                    <MenuItem key={llm.id} value={llm.id}>
                      {llm.attributes.name}
                    </MenuItem>
                  ))}
            </Select>
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseModal}>Cancel</Button>
          <PrimaryButton
            onClick={modalType === "add" ? handleAddLLM : handleRemoveLLM}
            variant="contained"
            color="primary"
          >
            {modalType === "add" ? "Add" : "Remove"}
          </PrimaryButton>
        </DialogActions>
      </Dialog>

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath={null}
        objectLabel="catalog"
        objectLabelPlural="catalogs"
        items={bulk.deleteDialogItems}
        consequence="Teams assigned these catalogs lose access to the LLM providers in them."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
});

CatalogueList.displayName = 'CatalogueList';

export default CatalogueList;
