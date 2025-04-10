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
  Chip,
  Box,
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
import PlayArrowIcon from "@mui/icons-material/PlayArrow";
import StopIcon from "@mui/icons-material/Stop";
import RefreshIcon from "@mui/icons-material/Refresh";
import mcpService from "../../admin/services/mcpService";

const MCPServerListView = () => {
  const [servers, setServers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [serverToDelete, setServerToDelete] = useState(null);
  const [actionInProgress, setActionInProgress] = useState({});
  const navigate = useNavigate();

  useEffect(() => {
    fetchServers();
  }, []);

  const fetchServers = async () => {
    try {
      const data = await mcpService.getServers();
      setServers(data.data || []);
      setLoading(false);
    } catch (err) {
      console.error("Error fetching MCP servers:", err);
      setError("Failed to fetch MCP servers. Please try again later.");
      setLoading(false);
    }
  };

  const handleRowClick = (serverId) => {
    navigate(`/portal/mcp-servers/${serverId}`);
  };

  const handleDeleteClick = (e, server) => {
    e.stopPropagation();
    setServerToDelete(server);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    try {
      await mcpService.deleteServer(serverToDelete.id);
      setDeleteDialogOpen(false);
      setServerToDelete(null);
      fetchServers(); // Refresh the server list
    } catch (err) {
      console.error("Error deleting MCP server:", err);
      setError("Failed to delete MCP server. Please try again later.");
    }
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setServerToDelete(null);
  };

  const handleCreateServer = () => {
    navigate("/portal/mcp-servers/new");
  };

  const handleServerAction = async (e, serverId, action) => {
    e.stopPropagation();
    setActionInProgress({ ...actionInProgress, [serverId]: action });
    
    try {
      if (action === "start") {
        await mcpService.startServer(serverId);
      } else if (action === "stop") {
        await mcpService.stopServer(serverId);
      } else if (action === "restart") {
        await mcpService.restartServer(serverId);
      }
      fetchServers(); // Refresh to get updated status
    } catch (err) {
      console.error(`Error ${action} MCP server:`, err);
      setError(`Failed to ${action} MCP server. Please try again later.`);
    } finally {
      setActionInProgress({ ...actionInProgress, [serverId]: null });
    }
  };

  const getStatusChip = (status) => {
    let color = "default";
    
    switch (status) {
      case "running":
        color = "success";
        break;
      case "stopped":
        color = "error";
        break;
      case "starting":
      case "stopping":
      case "restarting":
        color = "warning";
        break;
      default:
        color = "default";
    }
    
    return (
      <Chip 
        label={status} 
        color={color} 
        size="small" 
        variant="outlined"
      />
    );
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
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 4 }}>
        <Typography variant="h4" component="h1">
          MCP Servers
        </Typography>
        <PrimaryButton
          startIcon={<AddIcon />}
          onClick={handleCreateServer}
        >
          Create MCP Server
        </PrimaryButton>
      </Box>
      
      {servers.length === 0 ? (
        <StyledPaper sx={{ p: 4, textAlign: "center" }}>
          <Typography variant="h6" gutterBottom>
            MCP Servers enable AI agents to interact with your tools
          </Typography>
          <Typography variant="body1" paragraph>
            Create your first MCP server to get started.
          </Typography>
          <PrimaryButton
            startIcon={<AddIcon />}
            onClick={handleCreateServer}
          >
            Create MCP Server
          </PrimaryButton>
        </StyledPaper>
      ) : (
        <TableContainer component={StyledPaper}>
          <Table sx={{ minWidth: 650 }} aria-label="MCP servers table">
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                <StyledTableHeaderCell>Endpoint</StyledTableHeaderCell>
                <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                <StyledTableHeaderCell>Tools</StyledTableHeaderCell>
                <StyledTableHeaderCell>Actions</StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {servers.map((server) => (
                <StyledTableRow
                  key={server.id}
                  onClick={() => handleRowClick(server.id)}
                  sx={{ cursor: "pointer" }}
                >
                  <StyledTableCell>
                    {server.attributes.name}
                  </StyledTableCell>
                  <StyledTableCell>{server.attributes.description}</StyledTableCell>
                  <StyledTableCell>{server.attributes.endpoint}</StyledTableCell>
                  <StyledTableCell>
                    {getStatusChip(server.attributes.status)}
                  </StyledTableCell>
                  <StyledTableCell>{server.attributes.tools ? server.attributes.tools.length : 0}</StyledTableCell>
                  <StyledTableCell>
                    <Box sx={{ display: 'flex' }}>
                      {server.attributes.status === "running" ? (
                        <IconButton
                          aria-label="stop"
                          onClick={(e) => handleServerAction(e, server.id, "stop")}
                          disabled={actionInProgress[server.id]}
                        >
                          {actionInProgress[server.id] === "stop" ? (
                            <CircularProgress size={24} />
                          ) : (
                            <StopIcon />
                          )}
                        </IconButton>
                      ) : (
                        <IconButton
                          aria-label="start"
                          onClick={(e) => handleServerAction(e, server.id, "start")}
                          disabled={actionInProgress[server.id]}
                        >
                          {actionInProgress[server.id] === "start" ? (
                            <CircularProgress size={24} />
                          ) : (
                            <PlayArrowIcon />
                          )}
                        </IconButton>
                      )}
                      <IconButton
                        aria-label="restart"
                        onClick={(e) => handleServerAction(e, server.id, "restart")}
                        disabled={actionInProgress[server.id]}
                      >
                        {actionInProgress[server.id] === "restart" ? (
                          <CircularProgress size={24} />
                        ) : (
                          <RefreshIcon />
                        )}
                      </IconButton>
                      <IconButton
                        aria-label="delete"
                        onClick={(e) => handleDeleteClick(e, server)}
                        disabled={actionInProgress[server.id]}
                      >
                        <DeleteIcon />
                      </IconButton>
                    </Box>
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
            Are you sure you want to delete the MCP server "
            {serverToDelete?.attributes.name}"? This action cannot be undone.
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

export default MCPServerListView;
