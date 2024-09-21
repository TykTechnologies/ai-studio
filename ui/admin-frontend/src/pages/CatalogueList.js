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
  StyledTableRow,
  StyledButton,
} from "../styles/sharedStyles";
import InfoTooltip from "../components/common/InfoTooltip";

const CatalogueList = () => {
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

  useEffect(() => {
    fetchCatalogues();
  }, []);

  const fetchCatalogues = async () => {
    try {
      const response = await apiClient.get("/catalogues");
      setCatalogues(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching catalogues", error);
      setError("Failed to load catalogues");
      setLoading(false);
    }
  };

  const handleMenuOpen = (event, catalogue) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedCatalogue(catalogue);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

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
    navigate(`/catalogs/llms/edit/${id}`);
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
      fetchCatalogues(); // Refresh the list after adding an LLM
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
      fetchCatalogues(); // Refresh the list after removing an LLM
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
    navigate("/catalogs/llms/new");
  };

  const handleCatalogueClick = (id) => {
    navigate(`/catalogs/llms/${id}`);
  };

  if (loading) {
    return <CircularProgress />;
  }

  if (error) {
    return <Alert severity="error">{error}</Alert>;
  }

  const getLLMNames = (catalogue) => {
    if (catalogue.attributes.llm_names) {
      return catalogue.attributes.llm_names;
    } else if (
      catalogue.attributes.llms &&
      Array.isArray(catalogue.attributes.llms)
    ) {
      return catalogue.attributes.llms.map((llm) => llm.attributes.name);
    }
    return [];
  };

  return (
    <Box sx={{ p: 0 }}>
      <StyledPaper>
        <TitleBox>
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Catalogs are collections of LLMs that can be assigned to groups." />
            <Typography variant="h5">Catalogs</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddCatalogue}
          >
            Add Catalog
          </StyledButton>
        </TitleBox>
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
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableCell>Name</StyledTableCell>
                  <StyledTableCell>LLMs</StyledTableCell>
                  <StyledTableCell align="right">Actions</StyledTableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {catalogues.map((catalogue) => (
                  <StyledTableRow
                    key={catalogue.id}
                    onClick={() => handleCatalogueClick(catalogue.id)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{catalogue.attributes.name}</TableCell>
                    <TableCell>
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
                    </TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, catalogue)}
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
        <MenuItem onClick={(event) => handleEdit(event, selectedCatalogue?.id)}>
          Edit Catalog
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedCatalogue?.id)}>
          Delete Catalog
        </MenuItem>
        <MenuItem onClick={() => handleOpenModal("add")}>
          Add LLM to Catalog
        </MenuItem>
        <MenuItem onClick={() => handleOpenModal("remove")}>
          Remove LLM from Catalog
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
          <Button
            onClick={modalType === "add" ? handleAddLLM : handleRemoveLLM}
            variant="contained"
            color="primary"
          >
            {modalType === "add" ? "Add" : "Remove"}
          </Button>
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
    </Box>
  );
};

export default CatalogueList;
