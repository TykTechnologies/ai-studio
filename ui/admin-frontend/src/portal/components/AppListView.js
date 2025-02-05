import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import {
  Container,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  IconButton,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Button,
  Box,
} from "@mui/material";
import DeleteIcon from "@mui/icons-material/Delete";
import AddIcon from "@mui/icons-material/Add";
import pubClient from "../../admin/utils/pubClient";

const AppListView = () => {
  const [apps, setApps] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [appToDelete, setAppToDelete] = useState(null);
  const navigate = useNavigate();

  useEffect(() => {
    fetchApps();
  }, []);

  const fetchApps = async () => {
    try {
      const response = await pubClient.get("/common/apps");
      setApps(response.data.data);
      setLoading(false);
    } catch (err) {
      console.error("Error fetching apps:", err);
      setError("Failed to fetch apps. Please try again later.");
      setLoading(false);
    }
  };

  const handleRowClick = (appId) => {
    navigate(`/portal/apps/${appId}`);
  };

  const handleDeleteClick = (e, app) => {
    e.stopPropagation();
    setAppToDelete(app);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    try {
      await pubClient.delete(`/common/apps/${appToDelete.id}`);
      setDeleteDialogOpen(false);
      setAppToDelete(null);
      fetchApps(); // Refresh the app list
    } catch (err) {
      console.error("Error deleting app:", err);
      setError("Failed to delete app. Please try again later.");
    }
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setAppToDelete(null);
  };

  const handleCreateApp = () => {
    navigate("/portal/app/new");
  };

  if (loading) {
    return (
      <Container sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Container>
    );
  }

  if (error) {
    return (
      <Container>
        <Typography color="error" sx={{ textAlign: "center", mt: 4 }}>
          {error}
        </Typography>
      </Container>
    );
  }

  return (
    <Container
      maxWidth={false}
      sx={{
        px: 3,
        py: 3,
        boxSizing: "border-box",
        width: "100%",
      }}
    >
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 4 }}>
        My Apps
      </Typography>
      {apps.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: "center" }}>
          <Typography variant="h6" gutterBottom>
            Apps provide access to LLMs and Data sources via the AI Gateway
          </Typography>
          <Typography variant="body1" paragraph>
            Create your first app to get started.
          </Typography>
          <Button
            variant="contained"
            color="primary"
            startIcon={<AddIcon />}
            onClick={handleCreateApp}
          >
            Create App
          </Button>
        </Paper>
      ) : (
        <TableContainer component={Paper}>
          <Table sx={{ minWidth: 650 }} aria-label="apps table">
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Description</TableCell>
                <TableCell>Data Sources</TableCell>
                <TableCell>LLMs</TableCell>
                <TableCell>Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {apps.map((app) => (
                <TableRow
                  key={app.id}
                  sx={{
                    "&:last-child td, &:last-child th": { border: 0 },
                    "&:hover": {
                      backgroundColor: "rgba(0, 0, 0, 0.04)",
                      cursor: "pointer",
                    },
                  }}
                  onClick={() => handleRowClick(app.id)}
                >
                  <TableCell component="th" scope="row">
                    {app.attributes.name}
                  </TableCell>
                  <TableCell>{app.attributes.description}</TableCell>
                  <TableCell>{app.attributes.datasource_ids.length}</TableCell>
                  <TableCell>{app.attributes.llm_ids.length}</TableCell>
                  <TableCell>
                    <IconButton
                      aria-label="delete"
                      onClick={(e) => handleDeleteClick(e, app)}
                    >
                      <DeleteIcon />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Dialog
        open={deleteDialogOpen}
        onClose={handleDeleteCancel}
        aria-labelledby="alert-dialog-title"
        aria-describedby="alert-dialog-description"
      >
        <DialogTitle id="alert-dialog-title">{"Confirm Deletion"}</DialogTitle>
        <DialogContent>
          <DialogContentText id="alert-dialog-description">
            Are you sure you want to delete the app "
            {appToDelete?.attributes.name}"? This action cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDeleteCancel}>Cancel</Button>
          <Button onClick={handleDeleteConfirm} autoFocus>
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default AppListView;
