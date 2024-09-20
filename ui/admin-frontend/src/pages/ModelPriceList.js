import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableCell,
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
  Dialog,
  DialogTitle,
  DialogContent,
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
  StyledTableRow,
  StyledButton,
  StyledDialog,
  StyledDialogTitle,
  StyledDialogContent,
} from "../styles/sharedStyles";
import InfoTooltip from "../components/common/InfoTooltip";
import { getVendorName, getVendorLogo } from "../utils/vendorLogos";

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

  useEffect(() => {
    fetchModelPrices();
  }, []);

  const fetchModelPrices = async () => {
    try {
      const response = await apiClient.get("/model-prices");
      setModelPrices(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching Model Prices", error);
      setError("Failed to load Model Prices");
      setLoading(false);
    }
  };

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
      setModelPrices(modelPrices.filter((price) => price.id !== id));
      setSnackbar({
        open: true,
        message: "Model Price deleted successfully",
        severity: "success",
      });
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
    navigate(`/model-prices/${price.id}`);
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

  const sortedPrices = [...modelPrices].sort((a, b) => {
    if (sortConfig.key === null) return 0;
    const aValue = a.attributes[sortConfig.key];
    const bValue = b.attributes[sortConfig.key];
    if (aValue < bValue) return sortConfig.direction === "asc" ? -1 : 1;
    if (aValue > bValue) return sortConfig.direction === "asc" ? 1 : -1;
    return 0;
  });

  const handleAddPrice = () => {
    navigate("/model-prices/new");
  };

  const handleOpenUpdatePriceModal = () => {
    setUpdatedPrice(selectedPrice.attributes.cpt);
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
            cpt: parseFloat(updatedPrice),
          },
        },
      });

      setModelPrices(
        modelPrices.map((price) =>
          price.id === selectedPrice.id
            ? {
                ...price,
                attributes: {
                  ...price.attributes,
                  cpt: parseFloat(updatedPrice),
                },
              }
            : price,
        ),
      );

      setSnackbar({
        open: true,
        message: "Model Price updated successfully",
        severity: "success",
      });

      handleCloseUpdatePriceModal();
    } catch (error) {
      console.error("Error updating Model Price", error);
      setSnackbar({
        open: true,
        message: "Failed to update Model Price",
        severity: "error",
      });
    }
  };

  if (loading) {
    return <CircularProgress />;
  }

  if (error) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <StyledPaper>
        <TitleBox>
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Model Prices define the cost per token for different language models." />
            <Typography variant="h5">Model Prices</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddPrice}
          >
            Add Model Price
          </StyledButton>
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
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableCell onClick={() => handleSort("model_name")}>
                    Model Name
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("vendor")}>
                    Vendor
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("cpt")}>
                    Cost per Token
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("currency")}>
                    Currency
                  </StyledTableCell>
                  <StyledTableCell align="right">Actions</StyledTableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {sortedPrices.map((price) => (
                  <StyledTableRow
                    key={price.id}
                    onClick={() => handlePriceClick(price)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{price.attributes.model_name}</TableCell>
                    <TableCell>
                      <Box display="flex" alignItems="center">
                        <img
                          src={getVendorLogo(price.attributes.vendor)}
                          alt={price.attributes.vendor}
                          style={{ width: 24, height: 24, marginRight: 8 }}
                        />
                        {getVendorName(price.attributes.vendor)}
                      </Box>
                    </TableCell>
                    <TableCell>{price.attributes.cpt}</TableCell>
                    <TableCell>{price.attributes.currency}</TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, price)}
                      >
                        <MoreVertIcon />
                      </IconButton>
                    </TableCell>
                  </StyledTableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </ContentBox>
      </StyledPaper>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleOpenUpdatePriceModal}>Update Price</MenuItem>
        <MenuItem
          onClick={() => navigate(`/model-prices/edit/${selectedPrice?.id}`)}
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
            label="Cost per Token"
            type="number"
            inputProps={{ step: 0.000001, min: 0 }}
            value={updatedPrice}
            onChange={(e) => setUpdatedPrice(e.target.value)}
            margin="normal"
          />
        </StyledDialogContent>
        <DialogActions>
          <StyledButton onClick={handleCloseUpdatePriceModal}>
            Cancel
          </StyledButton>
          <StyledButton onClick={handleUpdatePrice} color="primary">
            Update
          </StyledButton>
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
    </Box>
  );
};

export default ModelPriceList;
