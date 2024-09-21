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

const DataCatalogList = () => {
  const navigate = useNavigate();
  const [dataCatalogs, setDataCatalogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedCatalog, setSelectedCatalog] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  useEffect(() => {
    fetchDataCatalogs();
  }, []);

  const fetchDataCatalogs = async () => {
    try {
      const response = await apiClient.get("/data-catalogues");
      setDataCatalogs(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching data catalogs", error);
      setError("Failed to load data catalogs");
      setLoading(false);
    }
  };

  const handleMenuOpen = (event, catalog) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedCatalog(catalog);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/data-catalogues/${id}`);
      setDataCatalogs(dataCatalogs.filter((catalog) => catalog.id !== id));
      setSnackbar({
        open: true,
        message: "Data catalog deleted successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error deleting data catalog", error);
      setSnackbar({
        open: true,
        message: "Failed to delete data catalog",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleEdit = (id) => {
    navigate(`/catalogs/data/edit/${id}`);
  };

  const handleAddDataCatalog = () => {
    navigate("/catalogs/data/new");
  };

  const handleCatalogClick = (id) => {
    navigate(`/catalogs/data/${id}`);
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
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
            <InfoTooltip title="Data Catalogs are collections of data sources that can be assigned to groups." />
            <Typography variant="h5">Data Catalogs</Typography>
          </Box>
          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddDataCatalog}
          >
            Add Data Catalog
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {dataCatalogs.length === 0 ? (
            <EmptyStateWidget
              title="No data catalogs found"
              description="Click the button below to add a new data catalog."
              buttonText="Add Data Catalog"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddDataCatalog}
            />
          ) : (
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableCell>Name</StyledTableCell>
                  <StyledTableCell>Description</StyledTableCell>
                  <StyledTableCell>Data Sources</StyledTableCell>
                  <StyledTableCell>Tags</StyledTableCell>
                  <StyledTableCell align="right">Actions</StyledTableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {dataCatalogs.map((catalog) => (
                  <StyledTableRow
                    key={catalog.id}
                    onClick={() => handleCatalogClick(catalog.id)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{catalog.attributes.name}</TableCell>
                    <TableCell>
                      {catalog.attributes.short_description}
                    </TableCell>
                    <TableCell>
                      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                        {catalog.attributes.datasources.map((datasource) => (
                          <Chip
                            key={datasource.id}
                            label={datasource.attributes.name}
                            size="small"
                            sx={{ marginRight: 0.5, marginBottom: 0.5 }}
                          />
                        ))}
                      </Box>
                    </TableCell>
                    <TableCell>
                      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                        {catalog.attributes.tags.map((tag) => (
                          <Chip
                            key={tag.id}
                            label={tag.attributes.name}
                            size="small"
                            sx={{ marginRight: 0.5, marginBottom: 0.5 }}
                          />
                        ))}
                      </Box>
                    </TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, catalog)}
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
        <MenuItem onClick={() => handleEdit(selectedCatalog?.id)}>
          Edit Data Catalog
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedCatalog?.id)}>
          Delete Data Catalog
        </MenuItem>
      </Menu>

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

export default DataCatalogList;
