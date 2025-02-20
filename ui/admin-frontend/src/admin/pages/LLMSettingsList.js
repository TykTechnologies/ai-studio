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
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
  PrimaryButton,
} from "../styles/sharedStyles";
import InfoTooltip from "../components/common/InfoTooltip";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

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

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchLLMSettings = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/llm-settings", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
      setLLMSettings(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching LLM Settings", error);
      setError("Failed to load LLM Settings");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchLLMSettings();
  }, [fetchLLMSettings]);

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
      setSnackbar({
        open: true,
        message: "LLM Setting deleted successfully",
        severity: "success",
      });
      fetchLLMSettings();
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
    navigate(`/admin/llm-settings/${setting.id}`);
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

  const handleAddSetting = () => {
    navigate("/admin/llm-settings/new");
  };

  if (loading && llmSettings.length === 0) {
    return <CircularProgress />;
  }

  if (error && llmSettings.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <>
        <TitleBox top="64px">
          <Box display="flex" alignItems="center">
            <InfoTooltip title="LLM Call Settings define the configuration parameters for Large Language Models when a prompt is sent to the LLM." />
            <Typography variant="h5">LLM Call Settings</Typography>
          </Box>

          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddSetting}
          >
            Add LLM Call Setting
          </PrimaryButton>
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
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell onClick={() => handleSort("model_name")}>
                      Model Name
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("temperature")}>
                      Temperature
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("max_tokens")}>
                      Max Tokens
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {llmSettings.map((setting) => (
                    <StyledTableRow
                      key={setting.id}
                      onClick={() => handleSettingClick(setting)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>{setting.attributes.model_name}</StyledTableCell>
                      <StyledTableCell>{setting.attributes.temperature}</StyledTableCell>
                      <StyledTableCell>{setting.attributes.max_tokens}</StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, setting)}
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
          onClick={() =>
            navigate(`/admin/llm-settings/edit/${selectedSetting?.id}`)
          }
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
