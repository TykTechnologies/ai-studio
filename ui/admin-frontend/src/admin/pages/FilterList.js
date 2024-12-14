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
  StyledButton,
} from "../styles/sharedStyles";
import InfoTooltip from "../components/common/InfoTooltip";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const FilterList = () => {
  const navigate = useNavigate();
  const [filters, setFilters] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedFilter, setSelectedFilter] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [sortConfig, setSortConfig] = useState({ key: null, direction: "asc" });

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchFilters = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/filters", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
      setFilters(response.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching filters", error);
      setError("Failed to load filters");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchFilters();
  }, [fetchFilters]);

  const handleMenuOpen = (event, filter) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedFilter(filter);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/filters/${id}`);
      setSnackbar({
        open: true,
        message: "Filter deleted successfully",
        severity: "success",
      });
      fetchFilters();
    } catch (error) {
      console.error("Error deleting filter", error);
      setSnackbar({
        open: true,
        message: "Failed to delete filter",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleFilterClick = (filter) => {
    navigate(`/admin/filters/${filter.id}`);
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

  const handleAddFilter = () => {
    navigate("/admin/filters/new");
  };

  if (loading && filters.length === 0) {
    return <CircularProgress />;
  }

  if (error && filters.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Filters are used to process and modify data before it is passed to the LLM." />
            <Typography variant="h5">Filters</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddFilter}
          >
            Add Filter
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {filters.length === 0 ? (
            <EmptyStateWidget
              title="No filters created yet"
              description="Click the button below to add a new filter."
              buttonText="Add Filter"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddFilter}
            />
          ) : (
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell onClick={() => handleSort("name")}>
                      Name
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {filters.map((filter) => (
                    <StyledTableRow
                      key={filter.id}
                      onClick={() => handleFilterClick(filter)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>{filter.attributes.name}</StyledTableCell>
                      <StyledTableCell>{filter.attributes.description}</StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, filter)}
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
        <MenuItem
          onClick={() => navigate(`/admin/filters/edit/${selectedFilter?.id}`)}
        >
          Edit Filter
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedFilter?.id)}>
          Delete Filter
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

export default FilterList;
