import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import {
  Container,
  Typography,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Button,
} from "@mui/material";
import {
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  StyledPaper,
  PrimaryButton,
  DangerButton
} from "../../admin/styles/sharedStyles";
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
        <StyledPaper sx={{ p: 4, textAlign: "center" }}>
          <Typography variant="h6" gutterBottom>
            Apps provide access to LLMs and Data sources via the AI Gateway
          </Typography>
          <Typography variant="body1" paragraph>
            Create your first app to get started.
          </Typography>
          <PrimaryButton
            startIcon={<AddIcon />}
            onClick={handleCreateApp}
          >
            Create App
          </PrimaryButton>
        </StyledPaper>
      ) : (
        <TableContainer component={StyledPaper}>
          <Table sx={{ minWidth: 650 }} aria-label="apps table">
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                <StyledTableHeaderCell>Data Sources</StyledTableHeaderCell>
                <StyledTableHeaderCell>LLMs</StyledTableHeaderCell>
                <StyledTableHeaderCell>Tools</StyledTableHeaderCell>
                <StyledTableHeaderCell>Actions</StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {apps.map((app) => (
                <StyledTableRow
                  key={app.id}
                  onClick={() => handleRowClick(app.id)}
                  sx={{ cursor: "pointer" }}
                >
                  <StyledTableCell>
                    {app.attributes.name}
                  </StyledTableCell>
                  <StyledTableCell>{app.attributes.description}</StyledTableCell>
                  <StyledTableCell>{app.attributes.datasource_ids ? app.attributes.datasource_ids.length : 0}</StyledTableCell>
                  <StyledTableCell>{app.attributes.llm_ids ? app.attributes.llm_ids.length : 0}</StyledTableCell>
                  <StyledTableCell>{app.attributes.tool_ids ? app.attributes.tool_ids.length : 0}</StyledTableCell>
                  <StyledTableCell>
                    <IconButton
                      aria-label="delete"
                      onClick={(e) => handleDeleteClick(e, app)}
                    >
                      <DeleteIcon />
                    </IconButton>
                  </StyledTableCell>
                </StyledTableRow>
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
          <DangerButton onClick={handleDeleteConfirm} autoFocus>
            Delete
          </DangerButton>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default AppListView;
