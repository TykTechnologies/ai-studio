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
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableRow,
  StyledButton,
} from "../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../utils/vendorLogos";
import InfoTooltip from "../components/common/InfoTooltip";

const LLMList = () => {
  const navigate = useNavigate();
  const [llms, setLLMs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedLLM, setSelectedLLM] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [sortConfig, setSortConfig] = useState({ key: null, direction: "asc" });

  useEffect(() => {
    fetchLLMs();
  }, []);

  const fetchLLMs = async () => {
    try {
      const response = await apiClient.get("/llms");
      setLLMs(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching LLMs", error);
      setError("Failed to load LLMs");
      setLoading(false);
    }
  };

  const handleMenuOpen = (event, llm) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedLLM(llm);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/llms/${id}`);
      setLLMs(llms.filter((llm) => llm.id !== id));
      setSnackbar({
        open: true,
        message: "LLM deleted successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error deleting LLM", error);
      setSnackbar({
        open: true,
        message: "Failed to delete LLM",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleToggleActive = async (llm) => {
    try {
      const updatedLLM = {
        ...llm,
        attributes: { ...llm.attributes, active: !llm.attributes.active },
      };
      await apiClient.patch(`/llms/${llm.id}`, { data: updatedLLM });
      setLLMs(llms.map((l) => (l.id === llm.id ? updatedLLM : l)));
      setSnackbar({
        open: true,
        message: `LLM ${updatedLLM.attributes.active ? "activated" : "deactivated"} successfully`,
        severity: "success",
      });
    } catch (error) {
      console.error("Error toggling LLM active state", error);
      setSnackbar({
        open: true,
        message: "Failed to update LLM active state",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleLLMClick = (llm) => {
    navigate(`/llms/${llm.id}`);
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

  const sortedLLMs = [...llms].sort((a, b) => {
    if (sortConfig.key === null) return 0;
    const aValue = a.attributes[sortConfig.key];
    const bValue = b.attributes[sortConfig.key];
    if (aValue < bValue) return sortConfig.direction === "asc" ? -1 : 1;
    if (aValue > bValue) return sortConfig.direction === "asc" ? 1 : -1;
    return 0;
  });

  const handleAddLLM = () => {
    navigate("/llms/new");
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
            <InfoTooltip title="Large Language Models (LLMs) registered here can be used in chat rooms, and are available to developers in the Portal if set to Active. They must be part of a catalog in order to be usable by a group." />
            <Typography variant="h5">LLMs</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddLLM}
          >
            Add LLM
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {llms.length === 0 ? (
            <EmptyStateWidget
              title="Want to start working with your favourite LLM?"
              description="Click the button below to add a new LLM configuration to use in your chat room."
              buttonText="Add LLM"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddLLM}
            />
          ) : (
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableCell onClick={() => handleSort("name")}>
                    Name
                  </StyledTableCell>
                  <StyledTableCell>Short Description</StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("vendor")}>
                    Vendor
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("privacy_score")}>
                    Privacy Score
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("active")}>
                    Active
                  </StyledTableCell>
                  <StyledTableCell align="right">Actions</StyledTableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {sortedLLMs.map((llm) => (
                  <StyledTableRow
                    key={llm.id}
                    onClick={() => handleLLMClick(llm)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{llm.attributes.name}</TableCell>
                    <TableCell>{llm.attributes.short_description}</TableCell>
                    <TableCell>
                      <Box sx={{ display: "flex", alignItems: "center" }}>
                        <img
                          src={getVendorLogo(llm.attributes.vendor)}
                          alt={getVendorName(llm.attributes.vendor)}
                          style={{
                            width: 24,
                            height: 24,
                            marginRight: 8,
                            objectFit: "contain",
                          }}
                          onError={(e) => {
                            e.target.onerror = null;
                            e.target.src =
                              process.env.PUBLIC_URL +
                              "/images/placeholder-logo.png";
                          }}
                        />
                        {getVendorName(llm.attributes.vendor)}
                      </Box>
                    </TableCell>
                    <TableCell>{llm.attributes.privacy_score}</TableCell>
                    <TableCell>
                      <FiberManualRecordIcon
                        sx={{
                          color: llm.attributes.active ? "green" : "red",
                        }}
                      />
                    </TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, llm)}
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
        <MenuItem onClick={() => navigate(`/llms/edit/${selectedLLM?.id}`)}>
          Edit LLM
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedLLM?.id)}>
          Delete LLM
        </MenuItem>
        <MenuItem onClick={() => handleToggleActive(selectedLLM)}>
          {selectedLLM?.attributes.active ? "Deactivate" : "Activate"} LLM
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

export default LLMList;
