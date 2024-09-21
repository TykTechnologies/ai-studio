import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Button,
  Table,
  TableBody,
  TableCell,
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
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableRow,
  StyledButton,
} from "../styles/sharedStyles";
import EmptyStateWidget from "../components/common/EmptyStateWidget";

const ToolCatalogueList = () => {
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

  useEffect(() => {
    fetchToolCatalogues();
  }, []);

  const fetchToolCatalogues = async () => {
    try {
      const response = await apiClient.get("/tool-catalogues");
      setToolCatalogues(response.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching tool catalogues", error);
      setError("Failed to load tool catalogues");
      setLoading(false);
    }
  };

  const handleMenuOpen = (event, catalogue) => {
    setAnchorEl(event.currentTarget);
    setSelectedCatalogue(catalogue);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/tool-catalogues/${id}`);
      setToolCatalogues(toolCatalogues.filter((cat) => cat.id !== id));
      setSnackbar({
        open: true,
        message: "Tool catalogue deleted successfully",
        severity: "success",
      });
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

  const handleEdit = (id) => {
    navigate(`/catalogs/tools/edit/${id}`);
  };

  const handleAddToolCatalogue = () => {
    navigate("/catalogs/tools/new");
  };

  const handleCatalogueClick = (id) => {
    navigate(`/catalogs/tools/${id}`);
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">Tool Catalogs</Typography>
        <StyledButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleAddToolCatalogue}
        >
          Add Tool Catalog
        </StyledButton>
      </TitleBox>
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
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableCell>Name</StyledTableCell>
                <StyledTableCell>Description</StyledTableCell>
                <StyledTableCell>Tools</StyledTableCell>
                <StyledTableCell>Tags</StyledTableCell>
                <StyledTableCell align="right">Actions</StyledTableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {toolCatalogues.map((catalogue) => (
                <StyledTableRow
                  key={catalogue.id}
                  onClick={() => handleCatalogueClick(catalogue.id)}
                >
                  <TableCell>{catalogue.attributes.name}</TableCell>
                  <TableCell>
                    {catalogue.attributes.short_description}
                  </TableCell>
                  <TableCell>
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
                  </TableCell>
                  <TableCell>
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
                  </TableCell>
                  <TableCell align="right">
                    <IconButton
                      onClick={(event) => {
                        event.stopPropagation();
                        handleMenuOpen(event, catalogue);
                      }}
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
    </StyledPaper>
  );
};

export default ToolCatalogueList;
