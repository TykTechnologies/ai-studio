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

const LLMSettingsList = () => {
  const navigate = useNavigate();
  const [llmSettings, setLLMSettings] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedSetting, setSelectedSetting] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [sortConfig, setSortConfig] = useState({ key: null, direction: "asc" });

  useEffect(() => {
    fetchLLMSettings();
  }, []);

  const fetchLLMSettings = async () => {
    try {
      const response = await apiClient.get("/llm-settings");
      setLLMSettings(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching LLM Settings", error);
      setError("Failed to load LLM Settings");
      setLoading(false);
    }
  };

  const handleMenuOpen = (event, setting) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedSetting(setting);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/llm-settings/${id}`);
      setLLMSettings(llmSettings.filter((setting) => setting.id !== id));
      setSnackbar({
        open: true,
        message: "LLM Setting deleted successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error deleting LLM Setting", error);
      setSnackbar({
        open: true,
        message: "Failed to delete LLM Setting",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleSettingClick = (setting) => {
    navigate(`/llm-settings/${setting.id}`);
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

  const sortedSettings = [...llmSettings].sort((a, b) => {
    if (sortConfig.key === null) return 0;
    const aValue = a.attributes[sortConfig.key];
    const bValue = b.attributes[sortConfig.key];
    if (aValue < bValue) return sortConfig.direction === "asc" ? -1 : 1;
    if (aValue > bValue) return sortConfig.direction === "asc" ? 1 : -1;
    return 0;
  });

  const handleAddSetting = () => {
    navigate("/llm-settings/new");
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
            <InfoTooltip title="LLM Call Settings define the configuration parameters for Large Language Models when a prompt is sent to the LLM." />
            <Typography variant="h5">LLM Call Settings</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddSetting}
          >
            Add LLM Call Setting
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {llmSettings.length === 0 ? (
            <EmptyStateWidget
              title="No LLM Settings yet"
              description="LLM Call Settings define the parameters that are sent on each prompt to a model. These settings are required in order to set up a chat room. Click the button below to add a new LLM Call Setting."
              buttonText="Add LLM Call Setting"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddSetting}
            />
          ) : (
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableCell onClick={() => handleSort("model_name")}>
                    Model Name
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("temperature")}>
                    Temperature
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("max_tokens")}>
                    Max Tokens
                  </StyledTableCell>
                  <StyledTableCell align="right">Actions</StyledTableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {sortedSettings.map((setting) => (
                  <StyledTableRow
                    key={setting.id}
                    onClick={() => handleSettingClick(setting)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{setting.attributes.model_name}</TableCell>
                    <TableCell>{setting.attributes.temperature}</TableCell>
                    <TableCell>{setting.attributes.max_tokens}</TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, setting)}
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
          onClick={() => navigate(`/llm-settings/edit/${selectedSetting?.id}`)}
        >
          Edit Setting
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedSetting?.id)}>
          Delete Setting
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

export default LLMSettingsList;
