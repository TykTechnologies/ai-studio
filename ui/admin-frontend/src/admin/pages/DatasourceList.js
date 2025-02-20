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
  Chip,
} from "@mui/material";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import AddIcon from "@mui/icons-material/Add";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import {
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
  StyledPaper,
} from "../styles/sharedStyles";
import {
  getVectorStoreName,
  getVectorStoreLogo,
  getEmbedderName,
  getEmbedderLogo,
  fetchVendors,
} from "../utils/vendorUtils";
import InfoTooltip from "../components/common/InfoTooltip";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

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

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchDatasources = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/datasources", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
      setDatasources(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching datasources", error);
      setError("Failed to load datasources");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    const initializePage = async () => {
      const fetchedVendors = await fetchVendors();
      setVendors(fetchedVendors);
      fetchDatasources();
    };
    initializePage();
  }, [fetchDatasources]);

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
      setSnackbar({
        open: true,
        message: "Datasource deleted successfully",
        severity: "success",
      });
      fetchDatasources();
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
            tags: datasource.attributes.tags.map((tag) => tag.attributes.name),
          },
        },
      };
      await apiClient.patch(`/datasources/${datasource.id}`, updatedDatasource);
      setSnackbar({
        open: true,
        message: `Datasource ${
          updatedDatasource.data.attributes.active ? "activated" : "deactivated"
        } successfully`,
        severity: "success",
      });
      fetchDatasources();
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
    navigate(`/admin/datasources/${datasource.id}`);
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

  const handleAddDatasource = () => {
    navigate("/admin/datasources/new");
  };

  if (loading && datasources.length === 0) {
    return <CircularProgress />;
  }

  if (error && datasources.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Vector data sources are used to store and retrieve data to enhance conversations with your models. These can be created using embedding providers that vectorise the content you wish to search, and make for an excellent way to enhance your chat room effectiveness for your users, or to better inform responses in your AI Applications" />
            <Typography variant="h5">Vector Data Sources</Typography>
          </Box>

          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddDatasource}
          >
            Add Datasource
          </PrimaryButton>
        </TitleBox>
        <ContentBox>
          {datasources.length === 0 ? (
            <EmptyStateWidget
              title="No vector DBs yet"
              description="Vector data sources are used to store and retrieve data to enhance LLM response effectiveness. These can be created using embedding providers that vectorize the content you wish to search, and make for an excellent way to enhance your chat room value for your users, or to better inform responses in your AI Applications."
              buttonText="Add Datasource"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddDatasource}
            />
          ) : (
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell onClick={() => handleSort("name")}>
                      Name
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell>Short Description</StyledTableHeaderCell>
                    <StyledTableHeaderCell
                      onClick={() => handleSort("db_source_type")}
                    >
                      DB Source Type
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("embed_vendor")}>
                      Embed Vendor
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell
                      onClick={() => handleSort("privacy_score")}
                    >
                      Privacy Score
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell>Tags</StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("active")}>
                      Active
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {datasources.map((datasource) => (
                    <StyledTableRow
                      key={datasource.id}
                      onClick={() => handleDatasourceClick(datasource)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>{datasource.attributes.name}</StyledTableCell>
                      <StyledTableCell>
                        {datasource.attributes.short_description}
                      </StyledTableCell>
                      <StyledTableCell>
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
                      </StyledTableCell>
                      <StyledTableCell>
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
                      </StyledTableCell>
                      <StyledTableCell>
                        {datasource.attributes.privacy_score}
                      </StyledTableCell>
                      <StyledTableCell>
                        {datasource.attributes.tags.map((tag) => (
                          <Chip
                            key={tag.id}
                            label={tag.attributes.name}
                            size="small"
                            sx={{ mr: 0.5, mb: 0.5 }}
                          />
                        ))}
                      </StyledTableCell>
                      <StyledTableCell>
                        <FiberManualRecordIcon
                          sx={{
                            color: datasource.attributes.active
                              ? "green"
                              : "red",
                          }}
                        />
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, datasource)}
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
            navigate(`/admin/datasources/edit/${selectedDatasource?.id}`)
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
    </>
  );
};

export default DatasourceList;
