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
  Chip,
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
import {
  getVectorStoreName,
  getVectorStoreLogo,
  getEmbedderName,
  getEmbedderLogo,
  fetchVendors,
} from "../utils/vendorUtils";
import InfoTooltip from "../components/common/InfoTooltip";

const DatasourceList = () => {
  const navigate = useNavigate();
  const [datasources, setDatasources] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedDatasource, setSelectedDatasource] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [sortConfig, setSortConfig] = useState({ key: null, direction: "asc" });
  const [vendors, setVendors] = useState({ embedders: [], vectorStores: [] });

  useEffect(() => {
    const initializePage = async () => {
      const fetchedVendors = await fetchVendors();
      setVendors(fetchedVendors);
      fetchDatasources();
    };
    initializePage();
  }, []);

  const fetchDatasources = async () => {
    try {
      const response = await apiClient.get("/datasources");
      setDatasources(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching datasources", error);
      setError("Failed to load datasources");
      setLoading(false);
    }
  };

  const handleMenuOpen = (event, datasource) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedDatasource(datasource);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/datasources/${id}`);
      setDatasources(datasources.filter((ds) => ds.id !== id));
      setSnackbar({
        open: true,
        message: "Datasource deleted successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error deleting datasource", error);
      setSnackbar({
        open: true,
        message: "Failed to delete datasource",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleToggleActive = async (datasource) => {
    try {
      const updatedDatasource = {
        data: {
          type: "Datasource",
          id: datasource.id,
          attributes: {
            ...datasource.attributes,
            active: !datasource.attributes.active,
            // Convert tags to an array of strings
            tags: datasource.attributes.tags.map((tag) => tag.attributes.name),
          },
        },
      };
      await apiClient.patch(`/datasources/${datasource.id}`, updatedDatasource);

      // Update local state, keeping the original tag structure
      setDatasources(
        datasources.map((ds) =>
          ds.id === datasource.id
            ? {
                ...ds,
                attributes: {
                  ...ds.attributes,
                  active: !ds.attributes.active,
                },
              }
            : ds,
        ),
      );

      setSnackbar({
        open: true,
        message: `Datasource ${
          updatedDatasource.data.attributes.active ? "activated" : "deactivated"
        } successfully`,
        severity: "success",
      });
    } catch (error) {
      console.error("Error toggling datasource active state", error);
      setSnackbar({
        open: true,
        message: "Failed to update datasource active state",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleDatasourceClick = (datasource) => {
    navigate(`/datasources/${datasource.id}`);
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

  const sortedDatasources = [...datasources].sort((a, b) => {
    if (sortConfig.key === null) return 0;
    const aValue = a.attributes[sortConfig.key];
    const bValue = b.attributes[sortConfig.key];
    if (aValue < bValue) return sortConfig.direction === "asc" ? -1 : 1;
    if (aValue > bValue) return sortConfig.direction === "asc" ? 1 : -1;
    return 0;
  });

  const handleAddDatasource = () => {
    navigate("/datasources/new");
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
            <InfoTooltip title="Vector data sources are used to store and retrieve data to enhance conversations with your models. These can be created using embedding providers that vecotrise the content you wish to search, and make for an excellent way to enhance your chat room effectiveness for your users, or to better inform responses in your AI Applications" />
            <Typography variant="h5">Vector Data Sources</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddDatasource}
          >
            Add Datasource
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {datasources.length === 0 ? (
            <EmptyStateWidget
              title="No vector DBs yet"
              description="Vector data sources are used to store and retrieve data to enhance LLM response effectiveness. These can be created using embedding providers that vecotrize the content you wish to search, and make for an excellent way to enhance your chat room value for your users, or to better inform responses in your AI Applications."
              buttonText="Add Datasource"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddDatasource}
            />
          ) : (
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableCell onClick={() => handleSort("name")}>
                    Name
                  </StyledTableCell>
                  <StyledTableCell>Short Description</StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("db_source_type")}>
                    DB Source Type
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("embed_vendor")}>
                    Embed Vendor
                  </StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("privacy_score")}>
                    Privacy Score
                  </StyledTableCell>
                  <StyledTableCell>Tags</StyledTableCell>
                  <StyledTableCell onClick={() => handleSort("active")}>
                    Active
                  </StyledTableCell>
                  <StyledTableCell align="right">Actions</StyledTableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {sortedDatasources.map((datasource) => (
                  <StyledTableRow
                    key={datasource.id}
                    onClick={() => handleDatasourceClick(datasource)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{datasource.attributes.name}</TableCell>
                    <TableCell>
                      {datasource.attributes.short_description}
                    </TableCell>
                    <TableCell>
                      <Box sx={{ display: "flex", alignItems: "center" }}>
                        <img
                          src={getVectorStoreLogo(
                            datasource.attributes.db_source_type,
                          )}
                          alt={getVectorStoreName(
                            datasource.attributes.db_source_type,
                          )}
                          style={{
                            width: 24,
                            height: 24,
                            marginRight: 8,
                            objectFit: "contain",
                          }}
                        />
                        {getVectorStoreName(
                          datasource.attributes.db_source_type,
                        )}
                      </Box>
                    </TableCell>
                    <TableCell>
                      <Box sx={{ display: "flex", alignItems: "center" }}>
                        <img
                          src={getEmbedderLogo(
                            datasource.attributes.embed_vendor,
                          )}
                          alt={getEmbedderName(
                            datasource.attributes.embed_vendor,
                          )}
                          style={{
                            width: 24,
                            height: 24,
                            marginRight: 8,
                            objectFit: "contain",
                          }}
                        />
                        {getEmbedderName(datasource.attributes.embed_vendor)}
                      </Box>
                    </TableCell>
                    <TableCell>{datasource.attributes.privacy_score}</TableCell>
                    <TableCell>
                      {datasource.attributes.tags.map((tag) => (
                        <Chip
                          key={tag.id}
                          label={tag.attributes.name}
                          size="small"
                          sx={{ mr: 0.5, mb: 0.5 }}
                        />
                      ))}
                    </TableCell>
                    <TableCell>
                      <FiberManualRecordIcon
                        sx={{
                          color: datasource.attributes.active ? "green" : "red",
                        }}
                      />
                    </TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, datasource)}
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
          onClick={() =>
            navigate(`/datasources/edit/${selectedDatasource?.id}`)
          }
        >
          Edit Datasource
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedDatasource?.id)}>
          Delete Datasource
        </MenuItem>
        <MenuItem onClick={() => handleToggleActive(selectedDatasource)}>
          {selectedDatasource?.attributes.active ? "Deactivate" : "Activate"}{" "}
          Datasource
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

export default DatasourceList;
