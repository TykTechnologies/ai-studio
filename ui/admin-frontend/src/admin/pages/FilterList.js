import React, { useState, useEffect, useCallback, memo } from "react";
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
import EnterpriseFeatureBadge from "../components/common/EnterpriseFeatureBadge";
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
import useAdminData from "../hooks/useAdminData";

const FilterList = memo(() => {
  const navigate = useNavigate();
  const { config, loading: configLoading } = useAdminData();
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

  const handleMenuOpen = useCallback((event, filter) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedFilter(filter);
  }, []);

  const handleMenuClose = useCallback(() => {
    setAnchorEl(null);
  }, []);

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

  const handleFilterClick = useCallback((filter) => {
    navigate(`/admin/filters/${filter.id}`);
  }, [navigate]);

  const handleCloseSnackbar = useCallback((event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  }, [snackbar]);

  const handleSort = useCallback((key) => {
    let direction = "asc";
    if (sortConfig.key === key && sortConfig.direction === "asc") {
      direction = "desc";
    }
    setSortConfig({ key, direction });
  }, [sortConfig]);

  const handleAddFilter = useCallback(() => {
    navigate("/admin/filters/new");
  }, [navigate]);

  // Wait for config to load before checking enterprise status
  if (configLoading) {
    return <CircularProgress />;
  }

  if (loading && filters.length === 0) {
    return <CircularProgress />;
  }

  if (error && filters.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  // Show enterprise badge if not enterprise edition
  if (config && !config.is_enterprise) {
    return (
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Filters & Middleware</Typography>
        </TitleBox>
        <ContentBox>
          <EnterpriseFeatureBadge
            feature="Advanced Request Filtering & Scripting"
            description="Create custom filters and middleware using Tengo scripting to process and modify data before it reaches the LLM or after tool execution. Remove PII, enforce policies, and transform data with powerful scripting capabilities."
          />
        </ContentBox>
      </>
    );
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Filters</Typography>
          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddFilter}
          >
            Add filter
          </PrimaryButton>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Filters are used as a security layer to process and modify data before it is passed to the LLM. For example, filters can remove personally identifiable information to ensure privacy.</Typography>
        </Box>
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
          Edit filter
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedFilter?.id)}>
          Delete filter
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
});

FilterList.displayName = 'FilterList';

export default FilterList;
