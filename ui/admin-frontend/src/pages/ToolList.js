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
  Tooltip,
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

  useEffect(() => {
    fetchTools();
  }, []);

  const fetchTools = async () => {
    try {
      const response = await apiClient.get("/tools");
      setTools(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching tools", error);
      setError("Failed to load tools");
      setLoading(false);
    }
  };

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
      setTools(tools.filter((tool) => tool.id !== id));
      setSnackbar({
        open: true,
        message: "Tool deleted successfully",
        severity: "success",
      });
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
    navigate(`/tools/${tool.id}`);
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

  const sortedTools = [...tools].sort((a, b) => {
    if (sortConfig.key === null) return 0;
    const aValue = a.attributes[sortConfig.key];
    const bValue = b.attributes[sortConfig.key];
    if (aValue < bValue) return sortConfig.direction === "asc" ? -1 : 1;
    if (aValue > bValue) return sortConfig.direction === "asc" ? 1 : -1;
    return 0;
  });

  const handleAddTool = () => {
    navigate("/tools/new");
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
            <InfoTooltip title="Tools are external services that can be used in chat rooms to enhance or provide additional data access and capabilities to the AI thas the user is interacting with. Tools are defined by an OpenAPI specification, and you can define which operations are available to the LLM to use from the spec as functions it can call to fulfil the user request." />
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
              description="Tools are external services that can be used in chat rooms to enhance or provide additional data access and capabilities to the AI thas the user is interacting with. Tools are defined by an OpenAPI specification, and you can define which operations are available to the LLM to use from the spec as functions it can call to fulfil the user request. Click the button below to add a new tool configuration."
              buttonText="Add Tool"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddTool}
            />
          ) : (
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableCell onClick={() => handleSort("name")}>
                    Name
                  </StyledTableCell>
                  <StyledTableCell>Description</StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("privacy_score")}>
                    Privacy Score
                  </StyledTableCell>
                  <StyledTableCell align="right">Actions</StyledTableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {sortedTools.map((tool) => (
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
          )}
        </ContentBox>
      </StyledPaper>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={() => navigate(`/tools/edit/${selectedTool?.id}`)}>
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
