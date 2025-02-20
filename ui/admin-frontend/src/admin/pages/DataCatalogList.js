import React, { useState, useEffect, useCallback } from "react";
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
import InfoTooltip from "../components/common/InfoTooltip";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

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

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchDataCatalogs = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/data-catalogues", {
        params: {
          page,
          page_size: pageSize,
        },
      });
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
  }, [page, pageSize, updatePaginationData]);

  useEffect(() => {
    fetchDataCatalogs();
  }, [fetchDataCatalogs]);

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
      setSnackbar({
        open: true,
        message: "Data catalog deleted successfully",
        severity: "success",
      });
      fetchDataCatalogs();
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
    navigate(`/admin/catalogs/data/edit/${id}`);
  };

  const handleAddDataCatalog = () => {
    navigate("/admin/catalogs/data/new");
  };

  const handleCatalogClick = (id) => {
    navigate(`/admin/catalogs/data/${id}`);
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading && dataCatalogs.length === 0) {
    return <CircularProgress />;
  }

  if (error && dataCatalogs.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Data Catalogs are collections of data sources that can be assigned to groups." />
            <Typography variant="h5">Data Catalogs</Typography>
          </Box>
          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddDataCatalog}
          >
            Add Data Catalog
          </PrimaryButton>
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
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Data Sources</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Tags</StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {dataCatalogs.map((catalog) => (
                    <StyledTableRow
                      key={catalog.id}
                      onClick={() => handleCatalogClick(catalog.id)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>{catalog.attributes.name}</StyledTableCell>
                      <StyledTableCell>
                        {catalog.attributes.short_description}
                      </StyledTableCell>
                      <StyledTableCell>
                        <Box
                          sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}
                        >
                          {catalog.attributes.datasources.map((datasource) => (
                            <Chip
                              key={datasource.id}
                              label={datasource.attributes.name}
                              size="small"
                              sx={{ marginRight: 0.5, marginBottom: 0.5 }}
                            />
                          ))}
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Box
                          sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}
                        >
                          {catalog.attributes.tags.map((tag) => (
                            <Chip
                              key={tag.id}
                              label={tag.attributes.name}
                              size="small"
                              sx={{ marginRight: 0.5, marginBottom: 0.5 }}
                            />
                          ))}
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, catalog)}
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
    </>
  );
};

export default DataCatalogList;
