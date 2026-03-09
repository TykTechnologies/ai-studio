import React, { useState, useEffect, useCallback, memo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Table,
  TableBody,
  TableHead,
  TableRow,
  IconButton,
  Menu,
  MenuItem,
  Snackbar,
  Alert,
  Chip,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
} from "../styles/sharedStyles";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const ToolCatalogueList = memo(() => {
  const [toolCatalogues, setToolCatalogues] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedCatalogue, setSelectedCatalogue] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchToolCatalogues = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/tool-catalogues", {
        params: {
          page,
          page_size: pageSize,
        },
      });
      setToolCatalogues(response.data?.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching tool catalogues", error);
      setError("Failed to load tool catalogues");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, updatePaginationData]);

  useEffect(() => {
    fetchToolCatalogues();
  }, [fetchToolCatalogues]);

  const handleMenuOpen = useCallback((event, catalogue) => {
    setAnchorEl(event.currentTarget);
    setSelectedCatalogue(catalogue);
  }, []);

  const handleMenuClose = useCallback(() => {
    setAnchorEl(null);
  }, []);

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/tool-catalogues/${id}`);
      setSnackbar({
        open: true,
        message: "Tool catalogue deleted successfully",
        severity: "success",
      });
      fetchToolCatalogues();
    } catch (error) {
      console.error("Error deleting tool catalogue", error);
      setSnackbar({
        open: true,
        message: "Failed to delete tool catalogue",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleEdit = useCallback((id) => {
    navigate(`/admin/catalogs/tools/edit/${id}`);
  }, [navigate]);

  const handleAddToolCatalogue = useCallback(() => {
    navigate("/admin/catalogs/tools/new");
  }, [navigate]);

  const handleCatalogueClick = useCallback((id) => {
    navigate(`/admin/catalogs/tools/${id}`);
  }, [navigate]);

  const handleCloseSnackbar = useCallback((event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  }, [snackbar]);

  if (loading && toolCatalogues.length === 0) return <CircularProgress />;
  if (error && toolCatalogues.length === 0)
    return <Typography color="error">{error}</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Tool catalogs</Typography>
        <PrimaryButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleAddToolCatalogue}
        >
          Add catalog
        </PrimaryButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of tools that you can assign to specific teams to manage access easily.</Typography>  
      </Box>
      <ContentBox>
        {toolCatalogues.length === 0 ? (
          <EmptyStateWidget
            title="No tool catalogs found"
            description="Click the button below to add a new tool catalog."
            buttonText="Add Tool Catalog"
            buttonIcon={<AddIcon />}
            onButtonClick={handleAddToolCatalogue}
          />
        ) : (
          <StyledPaper>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Tools</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Tags</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {toolCatalogues.map((catalogue) => (
                  <StyledTableRow
                    key={catalogue.id}
                    onClick={() => handleCatalogueClick(catalogue.id)}
                  >
                    <StyledTableCell>{catalogue.attributes.name}</StyledTableCell>
                    <StyledTableCell>
                      {catalogue.attributes.short_description}
                    </StyledTableCell>
                    <StyledTableCell>
                      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                        {catalogue.attributes.tools.map((tool) => (
                          <Chip
                            key={tool.id}
                            label={tool.attributes.name}
                            size="small"
                            sx={{ marginRight: 0.5, marginBottom: 0.5 }}
                          />
                        ))}
                      </Box>
                    </StyledTableCell>
                    <StyledTableCell>
                      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                        {catalogue.attributes.tags.map((tag) => (
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
                        onClick={(event) => {
                          event.stopPropagation();
                          handleMenuOpen(event, catalogue);
                        }}
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
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={() => handleEdit(selectedCatalogue?.id)}>
          Edit
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedCatalogue?.id)}>
          Delete
        </MenuItem>
      </Menu>
      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
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

ToolCatalogueList.displayName = 'ToolCatalogueList';

export default ToolCatalogueList;
