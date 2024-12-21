import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import AddIcon from "@mui/icons-material/Add";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
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
  Box,
  Snackbar,
} from "@mui/material";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  StyledButton,
} from "../styles/sharedStyles";

const Secrets = () => {
  const navigate = useNavigate();
  const [secrets, setSecrets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedSecret, setSelectedSecret] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchSecrets = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/secrets", {
        params: {
          page,
          page_size: pageSize,
        },
      });
      setSecrets(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching secrets", error);
      setError("Failed to load secrets");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, updatePaginationData]);

  useEffect(() => {
    fetchSecrets();
  }, [fetchSecrets]);

  const handleMenuOpen = (event, secret) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedSecret(secret);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/secrets/${id}`);
      setSnackbar({
        open: true,
        message: "Secret deleted successfully",
        severity: "success",
      });
      fetchSecrets();
    } catch (error) {
      console.error("Error deleting secret", error);
      setSnackbar({
        open: true,
        message: "Failed to delete secret",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleSecretClick = (secret) => {
    navigate(`/admin/secrets/${secret.id}`);
  };

  const handleAddSecret = () => {
    navigate("/admin/secrets/new");
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading && secrets.length === 0) {
    return <CircularProgress />;
  }

  if (error && secrets.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5">Secrets</Typography>
        <StyledButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleAddSecret}
        >
          Add Secret
        </StyledButton>
      </TitleBox>
      <ContentBox>
        {secrets.length === 0 ? (
          <EmptyStateWidget
            title="No secrets found"
            description="Click the button below to add a new secret."
            buttonText="Add Secret"
            buttonIcon={<AddIcon />}
            onButtonClick={handleAddSecret}
          />
        ) : (
          <StyledPaper>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>ID</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Variable Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {secrets.map((secret) => (
                  <StyledTableRow
                    key={secret.id}
                    onClick={() => handleSecretClick(secret)}
                    sx={{ cursor: "pointer" }}
                  >
                    <StyledTableCell>{secret.id}</StyledTableCell>
                    <StyledTableCell>{secret.attributes.var_name}</StyledTableCell>
                    <StyledTableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, secret)}
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

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem
          onClick={() => navigate(`/admin/secrets/edit/${selectedSecret?.id}`)}
        >
          Edit Secret
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedSecret?.id)}>
          Delete Secret
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

export default Secrets;
