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

  useEffect(() => {
    fetchFilters();
  }, []);

  const fetchFilters = async () => {
    try {
      const response = await apiClient.get("/filters");
      setFilters(response.data || []); // Remove the .data here
      setLoading(false);
    } catch (error) {
      console.error("Error fetching filters", error);
      setError("Failed to load filters");
      setLoading(false);
    }
  };

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
      setFilters(filters.filter((filter) => filter.id !== id));
      setSnackbar({
        open: true,
        message: "Filter deleted successfully",
        severity: "success",
      });
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
    navigate(`/filters/${filter.id}`);
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

  const sortedFilters = [...filters].sort((a, b) => {
    if (sortConfig.key === null) return 0;
    const aValue = a.attributes[sortConfig.key];
    const bValue = b.attributes[sortConfig.key];
    if (aValue < bValue) return sortConfig.direction === "asc" ? -1 : 1;
    if (aValue > bValue) return sortConfig.direction === "asc" ? 1 : -1;
    return 0;
  });

  const handleAddFilter = () => {
    navigate("/filters/new");
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
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableCell onClick={() => handleSort("name")}>
                    Name
                  </StyledTableCell>
                  <StyledTableCell>Description</StyledTableCell>
                  <StyledTableCell align="right">Actions</StyledTableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {sortedFilters.map((filter) => (
                  <StyledTableRow
                    key={filter.id}
                    onClick={() => handleFilterClick(filter)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{filter.attributes.name}</TableCell>
                    <TableCell>{filter.attributes.description}</TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, filter)}
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
        <MenuItem
          onClick={() => navigate(`/filters/edit/${selectedFilter?.id}`)}
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
    </Box>
  );
};

export default FilterList;
