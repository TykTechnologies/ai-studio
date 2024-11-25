// src/admin/pages/Secrets.js
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
  Box,
  Snackbar,
} from "@mui/material";
import { Link } from "react-router-dom";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableRow,
  StyledButton,
} from "../styles/sharedStyles";
import AddIcon from "@mui/icons-material/Add";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

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

  if (loading && secrets.length === 0) {
    return <CircularProgress />;
  }

  if (error && secrets.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <StyledPaper>
        <TitleBox>
          <Typography variant="h5">Secrets</Typography>
          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            component={Link}
            to="/admin/secrets/new"
          >
            Add Secret
          </StyledButton>
        </TitleBox>
        <ContentBox>
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableCell>ID</StyledTableCell>
                <StyledTableCell>Variable Name</StyledTableCell>
                <StyledTableCell align="right">Actions</StyledTableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {secrets.length > 0 ? (
                secrets.map((secret) => (
                  <StyledTableRow
                    key={secret.id}
                    onClick={() => handleSecretClick(secret)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{secret.id}</TableCell>
                    <TableCell>{secret.attributes.var_name}</TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, secret)}
                      >
                        <MoreVertIcon />
                      </IconButton>
                    </TableCell>
                  </StyledTableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={3}>No secrets found</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <PaginationControls
            page={page}
            pageSize={pageSize}
            totalPages={totalPages}
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
          />
        </ContentBox>
      </StyledPaper>

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
        onClose={() => setSnackbar({ ...snackbar, open: false })}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default Secrets;
