import React, { useState, useEffect, useCallback, memo, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
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
  Button,
  Select,
  FormControl,
  InputLabel,
  Chip,
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
} from "../styles/sharedStyles";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const CatalogueList = memo(() => {
  const navigate = useNavigate();
  const [catalogues, setCatalogues] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedCatalogue, setSelectedCatalogue] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [modalOpen, setModalOpen] = useState(false);
  const [modalType, setModalType] = useState("");
  const [availableLLMs, setAvailableLLMs] = useState([]);
  const [catalogueLLMs, setCatalogueLLMs] = useState([]);
  const [selectedLLM, setSelectedLLM] = useState("");

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchCatalogues = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/catalogues", {
        params: {
          page,
          page_size: pageSize,
        },
      });
      setCatalogues(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching catalogues", error);
      setError("Failed to load catalogues");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, updatePaginationData]);

  useEffect(() => {
    fetchCatalogues();
  }, [fetchCatalogues]);

  const handleMenuOpen = useCallback((event, catalogue) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedCatalogue(catalogue);
  }, []);

  const handleMenuClose = useCallback(() => {
    setAnchorEl(null);
  }, []);

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/catalogues/${id}`);
      setCatalogues(catalogues.filter((catalogue) => catalogue.id !== id));
      setSnackbar({
        open: true,
        message: "Catalog deleted successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error deleting catalog", error);
      setSnackbar({
        open: true,
        message: "Failed to delete catalog",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleEdit = (event, id) => {
    event.stopPropagation();
    navigate(`/admin/catalogs/llms/edit/${id}`);
  };

  const handleAddLLM = async () => {
    try {
      await apiClient.post(`/catalogues/${selectedCatalogue.id}/llms`, {
        data: { id: selectedLLM, type: "LLM" },
      });
      setSnackbar({
        open: true,
        message: "LLM added to catalog successfully",
        severity: "success",
      });
      setModalOpen(false);
      fetchCatalogues();
    } catch (error) {
      console.error("Error adding LLM to catalog", error);
      setSnackbar({
        open: true,
        message: "Failed to add LLM to catalog",
        severity: "error",
      });
    }
  };

  const handleRemoveLLM = async () => {
    try {
      await apiClient.delete(
        `/catalogues/${selectedCatalogue.id}/llms/${selectedLLM}`,
      );
      setSnackbar({
        open: true,
        message: "LLM removed from catalog successfully",
        severity: "success",
      });
      setModalOpen(false);
      fetchCatalogues();
    } catch (error) {
      console.error("Error removing LLM from catalog", error);
      setSnackbar({
        open: true,
        message: "Failed to remove LLM from catalog",
        severity: "error",
      });
    }
  };

  const handleOpenModal = async (type) => {
    setModalType(type);
    if (type === "add") {
      try {
        const response = await apiClient.get("/llms");
        setAvailableLLMs(
          response.data.data.filter((llm) => llm.attributes.active),
        );
      } catch (error) {
        console.error("Error fetching LLMs", error);
      }
    } else if (type === "remove") {
      try {
        const response = await apiClient.get(
          `/catalogues/${selectedCatalogue.id}/llms`,
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

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleAddCatalogue = () => {
    navigate("/admin/catalogs/llms/new");
  };

  const handleCatalogueClick = (id) => {
    navigate(`/admin/catalogs/llms/${id}`);
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

  if (loading && catalogues.length === 0) {
    return <CircularProgress />;
  }

  if (error && catalogues.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">LLM catalogs</Typography>
          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddCatalogue}
          >
            Add catalog
          </PrimaryButton>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of LLM providers that you can assign to specific user groups to manage access easily.</Typography>  
        </Box>
        <ContentBox>
          {catalogues.length === 0 ? (
            <EmptyStateWidget
              title="No catalogs found"
              description="Click the button below to add a new catalog."
              buttonText="Add Catalog"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddCatalogue}
            />
          ) : (
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                    <StyledTableHeaderCell>LLMs</StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {catalogues.map((catalogue) => (
                    <StyledTableRow
                      key={catalogue.id}
                      onClick={() => handleCatalogueClick(catalogue.id)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>{catalogue.attributes.name}</StyledTableCell>
                      <StyledTableCell>
                        <Box
                          sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}
                        >
                          {getLLMNames(catalogue).map((llmName, index) => (
                            <Chip
                              key={index}
                              label={llmName}
                              size="small"
                              sx={{ marginRight: 0.5, marginBottom: 0.5 }}
                            />
                          ))}
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, catalogue)}
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
        <MenuItem onClick={(event) => handleEdit(event, selectedCatalogue?.id)}>
          Edit catalog
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedCatalogue?.id)}>
          Delete catalog
        </MenuItem>
        <MenuItem onClick={() => handleOpenModal("add")}>
          Add LLM to catalog
        </MenuItem>
        <MenuItem onClick={() => handleOpenModal("remove")}>
          Remove LLM from catalog
        </MenuItem>
      </Menu>

      <Dialog open={modalOpen} onClose={handleCloseModal}>
        <DialogTitle>
          {modalType === "add"
            ? "Add LLM to Catalog"
            : "Remove LLM from Catalog"}
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
});

CatalogueList.displayName = 'CatalogueList';

export default CatalogueList;
