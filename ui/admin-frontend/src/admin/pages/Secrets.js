import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import AddIcon from "@mui/icons-material/Add";
import WarningIcon from "@mui/icons-material/Warning";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
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
  Box,
  Snackbar,
  Paper,
} from "@mui/material";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
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
      if (error.response?.status === 503) {
        setError(error.response.data.errors[0].detail);
      } else {
        setError("Failed to load secrets");
      }
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
    return (
      <Box sx={{ p: 3 }}>
        <Paper
          elevation={0}
          sx={{
            p: 3,
            backgroundColor: '',
            color: 'warning.dark',
            display: 'flex',
            alignItems: 'flex-start',
            gap: 2,
            mb: 3
          }}
        >
          <WarningIcon color="error" sx={{ mt: 0.5 }} />
          <Box>
            <Typography variant="h6" color="warning.  " gutterBottom>
              Secrets Management Unavailable
            </Typography>
            <Typography variant="body1" color="warning.dark">
              {error}
            </Typography>
          </Box>
        </Paper>
        <Paper elevation={0} sx={{ p: 3 }}>
          <Typography variant="h6" gutterBottom>
            How to Fix This
          </Typography>
          <Typography variant="body1" color="text.secondary" paragraph>
            To enable secrets management, you need to:
          </Typography>
          <Box component="ol" sx={{ color: 'text.secondary', pl: 2 }}>
            <li>
              <Typography variant="body1">
                Set the TYK_AI_SECRET_KEY environment variable with any string value
              </Typography>
            </li>
            <li>
              <Typography variant="body1">
                Restart the server to apply the changes
              </Typography>
            </li>
          </Box>
          <Typography variant="body1" color="text.secondary" sx={{ mt: 2 }}>
            For more information, please refer to the documentation.
          </Typography>
        </Paper>
      </Box>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Secrets</Typography>
        <PrimaryButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleAddSecret}
        >
          Add secret
        </PrimaryButton>
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
          Edit secret
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedSecret?.id)}>
          Delete secret
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
