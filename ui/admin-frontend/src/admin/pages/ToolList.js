import React, { useState, useEffect, useCallback } from "react";
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
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const ToolList = () => {
  const navigate = useNavigate();
  const [tools, setTools] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedTool, setSelectedTool] = useState(null);
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

  const fetchTools = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/tools", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
      setTools(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching tools", error);
      setError("Failed to load tools");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchTools();
  }, [fetchTools]);

  const handleMenuOpen = (event, tool) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedTool(tool);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/tools/${id}`);
      setSnackbar({
        open: true,
        message: "Tool deleted successfully",
        severity: "success",
      });
      fetchTools();
    } catch (error) {
      console.error("Error deleting tool", error);
      setSnackbar({
        open: true,
        message: "Failed to delete tool",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleToolClick = (tool) => {
    navigate(`/admin/tools/${tool.id}`);
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

  const handleAddTool = () => {
    navigate("/admin/tools/new");
  };

  if (loading && tools.length === 0) {
    return <CircularProgress />;
  }

  if (error && tools.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <StyledPaper>
        <TitleBox>
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Tools are external services that can be used in chat rooms to enhance or provide additional data access and capabilities to the AI that the user is interacting with. Tools are defined by an OpenAPI specification, and you can define which operations are available to the LLM to use from the spec as functions it can call to fulfil the user request." />
            <Typography variant="h5">Tools</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddTool}
          >
            Add Tool
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {tools.length === 0 ? (
            <EmptyStateWidget
              title="No tools added yet"
              description="Tools are external services that can be used in chat rooms to enhance or provide additional data access and capabilities to the AI that the user is interacting with. Tools are defined by an OpenAPI specification, and you can define which operations are available to the LLM to use from the spec as functions it can call to fulfil the user request. Click the button below to add a new tool configuration."
              buttonText="Add Tool"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddTool}
            />
          ) : (
            <>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableCell onClick={() => handleSort("name")}>
                      Name
                    </StyledTableCell>
                    <StyledTableCell>Description</StyledTableCell>
                    <StyledTableCell
                      onClick={() => handleSort("privacy_score")}
                    >
                      Privacy Score
                    </StyledTableCell>
                    <StyledTableCell align="right">Actions</StyledTableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {tools.map((tool) => (
                    <StyledTableRow
                      key={tool.id}
                      onClick={() => handleToolClick(tool)}
                      sx={{ cursor: "pointer" }}
                    >
                      <TableCell>{tool.attributes.name}</TableCell>
                      <TableCell>{tool.attributes.description}</TableCell>
                      <TableCell>{tool.attributes.privacy_score}</TableCell>
                      <TableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, tool)}
                        >
                          <MoreVertIcon />
                        </IconButton>
                      </TableCell>
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
            </>
          )}
        </ContentBox>
      </StyledPaper>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem
          onClick={() => navigate(`/admin/tools/edit/${selectedTool?.id}`)}
        >
          Edit Tool
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedTool?.id)}>
          Delete Tool
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

export default ToolList;
