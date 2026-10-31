import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Button,
  Typography,
  Alert,
  Box,
  DialogActions,
  TextField,
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
  StyledDialog,
  StyledDialogTitle,
  StyledDialogContent,
} from "../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../utils/vendorLogos";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";

const perMillion = (price, field) =>
  `${((price.attributes[field] || 0) * 1000000).toFixed(2)} ${price.attributes.currency}`;

const priceName = (price) => price?.attributes?.model_name || String(price?.id ?? "");

const ModelPriceList = () => {
  const navigate = useNavigate();
  const [modelPrices, setModelPrices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selectedPrice, setSelectedPrice] = useState(null);
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const [openUpdatePriceModal, setOpenUpdatePriceModal] = useState(false);
  const [updatedOutputPrice, setUpdatedOutputPrice] = useState(0);
  const [updatedInputPrice, setUpdatedInputPrice] = useState(0);
  const [updatedCacheWritePrice, setUpdatedCacheWritePrice] = useState(0);
  const [updatedCacheReadPrice, setUpdatedCacheReadPrice] = useState(0);
  const [backfillPrice, setBackfillPrice] = useState(null);
  const [backfillFrom, setBackfillFrom] = useState("");
  const [backfillTo, setBackfillTo] = useState("");
  const [backfilling, setBackfilling] = useState(false);

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchModelPrices = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/model-prices", { params: queryParams });
      setModelPrices(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching Model Prices", error);
      setError("Failed to load Model Prices");
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchModelPrices();
  }, [fetchModelPrices]);

  // No /bulk endpoint for model prices: deletes go one request per item.
  const bulk = useBulkActions({
    items: modelPrices,
    resource: "model-prices",
    singular: "model price",
    plural: "model prices",
    nameOf: priceName,
    notify,
    refresh: fetchModelPrices,
  });

  const handleDelete = useCallback(async (id) => {
    try {
      await apiClient.delete(`/model-prices/${id}`);
      notify("Model Price deleted successfully");
      fetchModelPrices();
    } catch (error) {
      console.error("Error deleting Model Price", error);
      notify("Failed to delete Model Price", "error");
    }
  }, [fetchModelPrices, notify]);

  const handlePriceClick = (price) => {
    navigate(`/admin/model-prices/${price.id}`);
  };

  const handleAddPrice = () => {
    navigate("/admin/model-prices/new");
  };

  const handleOpenUpdatePriceModal = useCallback((price) => {
    setSelectedPrice(price);
    setUpdatedOutputPrice(price.attributes.cpt * 1000000);
    setUpdatedInputPrice(price.attributes.cpit * 1000000);
    setUpdatedCacheWritePrice(price.attributes.cache_write_pt * 1000000);
    setUpdatedCacheReadPrice(price.attributes.cache_read_pt * 1000000);
    setOpenUpdatePriceModal(true);
  }, []);

  const handleCloseUpdatePriceModal = () => {
    setOpenUpdatePriceModal(false);
  };

  const handleUpdatePrice = async () => {
    try {
      await apiClient.patch(`/model-prices/${selectedPrice.id}`, {
        data: {
          type: "ModelPrice",
          attributes: {
            ...selectedPrice.attributes,
            cpt: parseFloat(updatedOutputPrice) / 1000000,
            cpit: parseFloat(updatedInputPrice) / 1000000,
            cache_write_pt: parseFloat(updatedCacheWritePrice) / 1000000,
            cache_read_pt: parseFloat(updatedCacheReadPrice) / 1000000,
          },
        },
      });

      notify("Model Price updated successfully");

      handleCloseUpdatePriceModal();
      fetchModelPrices();
    } catch (error) {
      console.error("Error updating Model Price", error);
      notify("Failed to update Model Price", "error");
    }
  };

  const handleOpenBackfill = useCallback((price) => {
    setBackfillPrice(price);
    setBackfillFrom("");
    setBackfillTo("");
  }, []);

  // Prices the requests recorded at no cost before the model had a price.
  const handleBackfill = async () => {
    setBackfilling(true);
    try {
      const body = {};
      if (backfillFrom) body.from = backfillFrom;
      if (backfillTo) body.to = backfillTo;
      const response = await apiClient.post(`/model-prices/${backfillPrice.id}/backfill`, body);
      const rows = response.data.data.rows_priced;
      notify(`Priced ${rows.toLocaleString()} unpriced ${rows === 1 ? "request" : "requests"}`);
      setBackfillPrice(null);
      fetchModelPrices();
    } catch (error) {
      console.error("Error backfilling unpriced history", error);
      const detail = error.response?.data?.errors?.[0]?.detail;
      notify(detail || "Failed to backfill unpriced history", "error");
    } finally {
      setBackfilling(false);
    }
  };

  const columns = useMemo(() => [
    { field: "name", headerName: "Model Name", sortable: true, renderCell: (price) => price.attributes.model_name },
    {
      field: "vendor",
      headerName: "Vendor",
      sortable: true,
      renderCell: (price) => (
        <Box display="flex" alignItems="center">
          <img
            src={getVendorLogo(price.attributes.vendor)}
            alt={price.attributes.vendor}
            style={{ width: 24, height: 24, marginRight: 8 }}
          />
          {getVendorName(price.attributes.vendor)}
        </Box>
      ),
    },
    { field: "cpit", headerName: "Cost per Million Input Tokens", renderCell: (price) => perMillion(price, "cpit") },
    { field: "cpt", headerName: "Cost per Million Output Tokens", renderCell: (price) => perMillion(price, "cpt") },
    { field: "cache_write_pt", headerName: "Cost per Million Cache Write Tokens", renderCell: (price) => perMillion(price, "cache_write_pt") },
    { field: "cache_read_pt", headerName: "Cost per Million Cache Read Tokens", renderCell: (price) => perMillion(price, "cache_read_pt") },
    { field: "currency", headerName: "Currency", renderCell: (price) => price.attributes.currency },
    {
      field: "unpriced_rows",
      headerName: "Unpriced Requests",
      renderCell: (price) => (price.meta?.unpriced_rows || 0).toLocaleString(),
    },
  ], []);

  const rowActions = useMemo(() => [
    { key: "update", label: "Update model price", onClick: handleOpenUpdatePriceModal },
    { key: "edit", label: "Edit model price", onClick: (price) => navigate(`/admin/model-prices/edit/${price.id}`) },
    { key: "backfill", label: "Backfill unpriced history", onClick: handleOpenBackfill },
    { key: "delete", label: "Delete model price", onClick: (price) => handleDelete(price.id) },
  ], [handleOpenUpdatePriceModal, navigate, handleOpenBackfill, handleDelete]);

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && modelPrices.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="var(--studio-header-height)">
          <Typography variant="headingXLarge">Model prices</Typography>
          <Can permission={P.MODEL_PRICES_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddPrice}
            >
              Add model price
            </PrimaryButton>
          </Can>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Model Prices define the cost per million tokens for using different language models. You can set the cost per million tokens for input and output, the provider, and the currency. This helps track usage costs, allowing you to manage and optimize expenses when interacting with different models.</Typography>
        </Box>
        <ContentBox>
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <Can permission={P.MODEL_PRICES_WRITE}>
            {(canWrite) => (
              <DataTable
                {...tableProps}
                ariaLabel="Model prices"
                searchPlaceholder="Search model prices by model name..."
                columns={columns}
                data={modelPrices}
                loading={loading}
                onRowClick={handlePriceClick}
                getRowLabel={priceName}
                actions={canWrite ? rowActions : undefined}
                {...(canWrite ? bulk.selectionProps : {})}
                bulkActions={canWrite ? bulkActions : undefined}
                emptyState={
                  !searchTerm ? (
                    <EmptyStateWidget
                      title="No Model Prices yet"
                      description="Model Prices define the cost per token for different language models. This is reflected in analytics recorded in the AI Gateway and from conversations in Chatrooms. Click the button below to add a new Model Price."
                      buttonText="Add Model Price"
                      buttonIcon={<AddIcon />}
                      onButtonClick={canWrite ? handleAddPrice : undefined}
                    />
                  ) : undefined
                }
              />
            )}
          </Can>
        </ContentBox>
      </>

      <StyledDialog
        open={openUpdatePriceModal}
        onClose={handleCloseUpdatePriceModal}
      >
        <StyledDialogTitle>Update model price</StyledDialogTitle>
        <StyledDialogContent>
          <TextField
            fullWidth
            label="Cost per Million Input Tokens"
            type="number"
            inputProps={{ step: 0.01, min: 0, inputMode: "decimal" }}
            value={updatedInputPrice}
            onChange={(e) => setUpdatedInputPrice(e.target.value.replace(',', '.'))}
            margin="normal"
          />
          <TextField
            fullWidth
            label="Cost per Million Output Tokens"
            type="number"
            inputProps={{ step: 0.01, min: 0, inputMode: "decimal" }}
            value={updatedOutputPrice}
            onChange={(e) => setUpdatedOutputPrice(e.target.value.replace(',', '.'))}
            margin="normal"
          />
          <TextField
            fullWidth
            label="Cost per Million Cache Write Tokens"
            type="number"
            inputProps={{ step: 0.01, min: 0, inputMode: "decimal" }}
            value={updatedCacheWritePrice}
            onChange={(e) => setUpdatedCacheWritePrice(e.target.value.replace(',', '.'))}
            margin="normal"
          />
          <TextField
            fullWidth
            label="Cost per Million Cache Read Tokens"
            type="number"
            inputProps={{ step: 0.01, min: 0, inputMode: "decimal" }}
            value={updatedCacheReadPrice}
            onChange={(e) => setUpdatedCacheReadPrice(e.target.value.replace(',', '.'))}
            margin="normal"
          />
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={handleCloseUpdatePriceModal}>
            Cancel
          </Button>
          <PrimaryButton onClick={handleUpdatePrice} color="primary">
            Update
          </PrimaryButton>
        </DialogActions>
      </StyledDialog>

      <StyledDialog open={!!backfillPrice} onClose={() => setBackfillPrice(null)}>
        <StyledDialogTitle>Backfill unpriced history</StyledDialogTitle>
        <StyledDialogContent>
          <Typography variant="bodyLargeDefault" component="p" sx={{ mb: 2 }}>
            {`${(backfillPrice?.meta?.unpriced_rows || 0).toLocaleString()} requests for ${priceName(backfillPrice)} were recorded at no cost, before the model had a price. Backfilling prices them at the current price. Requests that already have a cost are not changed; to reprice those, edit the price and recalculate.`}
          </Typography>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued" component="p">
            Leave the dates empty to backfill all of the unpriced history.
          </Typography>
          <TextField
            fullWidth
            label="From"
            type="date"
            InputLabelProps={{ shrink: true }}
            value={backfillFrom}
            onChange={(e) => setBackfillFrom(e.target.value)}
            margin="normal"
          />
          <TextField
            fullWidth
            label="To"
            type="date"
            InputLabelProps={{ shrink: true }}
            value={backfillTo}
            onChange={(e) => setBackfillTo(e.target.value)}
            margin="normal"
          />
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => setBackfillPrice(null)}>
            Cancel
          </Button>
          <PrimaryButton onClick={handleBackfill} color="primary" disabled={backfilling}>
            Backfill
          </PrimaryButton>
        </DialogActions>
      </StyledDialog>

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath={null}
        objectLabel="model price"
        objectLabelPlural="model prices"
        items={bulk.deleteDialogItems}
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default ModelPriceList;
