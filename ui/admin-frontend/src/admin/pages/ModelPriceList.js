import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  Button,
  TableHead,
  TableRow,
  Typography,
  IconButton,
  CircularProgress,
  Alert,
  Menu,
  MenuItem,
  Snackbar,
  Box,
  DialogActions,
  TextField,
} from "@mui/material";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import AddIcon from "@mui/icons-material/Add";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
  StyledDialog,
  StyledDialogTitle,
  StyledDialogContent,
} from "../styles/sharedStyles";
import InfoTooltip from "../components/common/InfoTooltip";
import { getVendorName, getVendorLogo } from "../utils/vendorLogos";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const ModelPriceList = () => {
  const navigate = useNavigate();
  const [modelPrices, setModelPrices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedPrice, setSelectedPrice] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [sortConfig, setSortConfig] = useState({ key: null, direction: "asc" });
  const [openUpdatePriceModal, setOpenUpdatePriceModal] = useState(false);
  const [updatedPrice, setUpdatedPrice] = useState(0);
  const [updatedOutputPrice, setUpdatedOutputPrice] = useState(0);
  const [updatedInputPrice, setUpdatedInputPrice] = useState(0);

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchModelPrices = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/model-prices", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
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
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchModelPrices();
  }, [fetchModelPrices]);

  const handleMenuOpen = (event, price) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedPrice(price);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/model-prices/${id}`);
      setSnackbar({
        open: true,
        message: "Model Price deleted successfully",
        severity: "success",
      });
      fetchModelPrices();
    } catch (error) {
      console.error("Error deleting Model Price", error);
      setSnackbar({
        open: true,
        message: "Failed to delete Model Price",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handlePriceClick = (price) => {
    navigate(`/admin/model-prices/${price.id}`);
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleSort = (key) => {
    let direction = "asc";
    if (sortConfig.key === key && sortConfig.direction === "asc") {
      direction = "desc";
    }
    setSortConfig({ key, direction });
  };

  const handleAddPrice = () => {
    navigate("/admin/model-prices/new");
  };

  const handleOpenUpdatePriceModal = () => {
    setUpdatedOutputPrice(selectedPrice.attributes.cpt * 1000000);
    setUpdatedInputPrice(selectedPrice.attributes.cpit * 1000000);
    setOpenUpdatePriceModal(true);
    handleMenuClose();
  };

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
          },
        },
      });

      setSnackbar({
        open: true,
        message: "Model Price updated successfully",
        severity: "success",
      });

      handleCloseUpdatePriceModal();
      fetchModelPrices();
    } catch (error) {
      console.error("Error updating Model Price", error);
      setSnackbar({
        open: true,
        message: "Failed to update Model Price",
        severity: "error",
      });
    }
  };

  if (loading && modelPrices.length === 0) {
    return <CircularProgress />;
  }

  if (error && modelPrices.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Model Prices define the cost per token for different language models." />
            <Typography variant="h5">Model Prices</Typography>
          </Box>

          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddPrice}
          >
            Add Model Price
          </PrimaryButton>
        </TitleBox>
        <ContentBox>
          {modelPrices.length === 0 ? (
            <EmptyStateWidget
              title="No Model Prices yet"
              description="Model Prices define the cost per token for different language models. This is reflected in analytics recorded in the AI Gateway and from conversations in Chatrooms. Click the button below to add a new Model Price."
              buttonText="Add Model Price"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddPrice}
            />
          ) : (
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell
                      onClick={() => handleSort("model_name")}
                    >
                      Model Name
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("vendor")}>
                      Vendor
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("cpit")}>
                      Cost per Million Input Tokens
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("cpt")}>
                      Cost per Million Output Tokens
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell
                      onClick={() => handleSort("currency")}
                    >
                      Currency
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">
                      Actions
                    </StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {modelPrices.map((price) => (
                    <StyledTableRow
                      key={price.id}
                      onClick={() => handlePriceClick(price)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>
                        {price.attributes.model_name}
                      </StyledTableCell>
                      <StyledTableCell>
                        <Box display="flex" alignItems="center">
                          <img
                            src={getVendorLogo(price.attributes.vendor)}
                            alt={price.attributes.vendor}
                            style={{ width: 24, height: 24, marginRight: 8 }}
                          />
                          {getVendorName(price.attributes.vendor)}
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell>
                        {`${(price.attributes.cpit * 1000000).toFixed(2)} ${price.attributes.currency}`}
                      </StyledTableCell>
                      <StyledTableCell>
                        {`${(price.attributes.cpt * 1000000).toFixed(2)} ${price.attributes.currency}`}
                      </StyledTableCell>
                      <StyledTableCell>
                        {price.attributes.currency}
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, price)}
                        >
                          <MoreVertIcon />
                        </IconButton>
                      </StyledTableCell>
                    </StyledTableRow>
                  ))}
                </TableBody>
              </Table>
              <PaginationControls
                page={page}
                pageSize={pageSize}
                totalPages={totalPages}
                onPageChange={handlePageChange}
                onPageSizeChange={handlePageSizeChange}
              />
            </StyledPaper>
          )}
        </ContentBox>
      </>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleOpenUpdatePriceModal}>Update Price</MenuItem>
        <MenuItem
          onClick={() =>
            navigate(`/admin/model-prices/edit/${selectedPrice?.id}`)
          }
        >
          Edit Price
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedPrice?.id)}>
          Delete Price
        </MenuItem>
      </Menu>

      <StyledDialog
        open={openUpdatePriceModal}
        onClose={handleCloseUpdatePriceModal}
      >
        <StyledDialogTitle>Update Model Price</StyledDialogTitle>
        <StyledDialogContent>
          <TextField
            fullWidth
            label="Cost per Million Input Tokens"
            type="number"
            inputProps={{ step: 0.01, min: 0 }}
            value={updatedInputPrice}
            onChange={(e) => setUpdatedInputPrice(e.target.value)}
            margin="normal"
          />
          <TextField
            fullWidth
            label="Cost per Million Output Tokens"
            type="number"
            inputProps={{ step: 0.01, min: 0 }}
            value={updatedOutputPrice}
            onChange={(e) => setUpdatedOutputPrice(e.target.value)}
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

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={handleCloseSnackbar}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default ModelPriceList;
