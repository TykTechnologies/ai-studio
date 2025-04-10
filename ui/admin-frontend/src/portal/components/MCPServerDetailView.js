import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  Container,
  Typography,
  Box,
  Paper,
  Grid,
  CircularProgress,
  Chip,
  Button,
  Tabs,
  Tab,
  IconButton,
  Tooltip,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from "@mui/material";
import {
  StyledPaper,
  PrimaryButton,
  DangerButton,
} from "../../admin/styles/sharedStyles";
import PlayArrowIcon from "@mui/icons-material/PlayArrow";
import StopIcon from "@mui/icons-material/Stop";
import RefreshIcon from "@mui/icons-material/Refresh";
import DeleteIcon from "@mui/icons-material/Delete";
import AddIcon from "@mui/icons-material/Add";
import EditIcon from "@mui/icons-material/Edit";
import mcpService from "../../admin/services/mcpService";

// Tab Panel Component
function TabPanel(props) {
  const { children, value, index, ...other } = props;

  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`simple-tabpanel-${index}`}
      aria-labelledby={`simple-tab-${index}`}
      {...other}
    >
      {value === index && <Box sx={{ p: 3 }}>{children}</Box>}
    </div>
  );
}

// Main component
const MCPServerDetailView = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [server, setServer] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState(0);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [formData, setFormData] = useState({ name: "", description: "" });
  const [tools, setTools] = useState([]);
  const [sessions, setSessions] = useState([]);
  const [logs, setLogs] = useState([]);
  const [actionInProgress, setActionInProgress] = useState(null);
  const [deleteToolDialogOpen, setDeleteToolDialogOpen] = useState(false);
  const [toolToRemove, setToolToRemove] = useState(null);
  const [addToolDialogOpen, setAddToolDialogOpen] = useState(false);
  const [availableTools, setAvailableTools] = useState([]);
  const [selectedToolId, setSelectedToolId] = useState("");
  const [sseConnected, setSseConnected] = useState(false);
  const [eventSource, setEventSource] = useState(null);

  // Fetch server details
  useEffect(() => {
    const fetchServerDetails = async () => {
      try {
        const data = await mcpService.getServer(id);
        const serverData = data.data;
        setServer(serverData);
        setFormData({
          name: serverData.attributes.name,
          description: serverData.attributes.description || "",
        });
        setLoading(false);
        
        // Also fetch tools for this server
        fetchServerTools();
        
        // And active sessions
        fetchActiveSessions();
        
        // Setup SSE connection for real-time updates
        setupSSEConnection();
      } catch (err) {
        console.error("Error fetching server details:", err);
        setError("Failed to fetch server details. Please try again later.");
        setLoading(false);
      }
    };

    fetchServerDetails();

    // Cleanup SSE connection on unmount
    return () => {
      if (eventSource) {
        eventSource.close();
      }
    };
  }, [id]);

  // Setup SSE (Server-Sent Events) connection
  const setupSSEConnection = () => {
    const sse = mcpService.createEventSource(id);
    
    sse.addEventListener('open', () => {
      console.log('SSE connection established');
      setSseConnected(true);
    });
    
    sse.addEventListener('status', (event) => {
      const data = JSON.parse(event.data);
      // Update server status
      setServer(prevServer => {
        if (!prevServer) return prevServer;
        
        return {
          ...prevServer,
          attributes: {
            ...prevServer.attributes,
            status: data.status
          }
        };
      });
    });
    
    sse.addEventListener('session', (event) => {
      const data = JSON.parse(event.data);
      // Update sessions list
      fetchActiveSessions();
    });
    
    sse.addEventListener('log', (event) => {
      const data = JSON.parse(event.data);
      // Add new log entry
      setLogs(prevLogs => [data, ...prevLogs.slice(0, 99)]); // Keep last 100 logs
    });
    
    sse.addEventListener('error', (error) => {
      console.error('SSE Error:', error);
      setSseConnected(false);
      sse.close();
    });
    
    setEventSource(sse);
  };

  // Fetch tools associated with this server
  const fetchServerTools = async () => {
    try {
      const data = await mcpService.getServerTools(id);
      setTools(data.data || []);
    } catch (err) {
      console.error("Error fetching server tools:", err);
      // Don't set an error state here, just log it
    }
  };
  
  // Fetch active sessions for this server
  const fetchActiveSessions = async () => {
    try {
      const data = await mcpService.getSessions(id);
      setSessions(data.data || []);
    } catch (err) {
      console.error("Error fetching active sessions:", err);
      // Don't set an error state here, just log it
    }
  };
  
  // Handle tab change
  const handleTabChange = (event, newValue) => {
    setActiveTab(newValue);
    
    // If switching to the Tools tab, fetch available tools
    if (newValue === 1 && availableTools.length === 0) {
      fetchAvailableTools();
    }
  };

  // Fetch all available tools
  const fetchAvailableTools = async () => {
    try {
      const data = await mcpService.getAvailableTools();
      setAvailableTools(data.data || []);
    } catch (err) {
      console.error("Error fetching available tools:", err);
    }
  };

  // Server control actions (start, stop, restart)
  const handleServerAction = async (action) => {
    setActionInProgress(action);
    
    try {
      if (action === "start") {
        await mcpService.startServer(id);
      } else if (action === "stop") {
        await mcpService.stopServer(id);
      } else if (action === "restart") {
        await mcpService.restartServer(id);
      }
      
      // Update the server status (assuming the request is successful)
      const statusMap = {
        start: "starting",
        stop: "stopping",
        restart: "restarting"
      };
      
      setServer(prevServer => ({
        ...prevServer,
        attributes: {
          ...prevServer.attributes,
          status: statusMap[action] || prevServer.attributes.status
        }
      }));
      
    } catch (err) {
      console.error(`Error ${action} server:`, err);
      setError(`Failed to ${action} server. Please try again later.`);
    } finally {
      setActionInProgress(null);
    }
  };

  // Open edit dialog
  const handleEditClick = () => {
    setFormData({
      name: server.attributes.name,
      description: server.attributes.description || "",
    });
    setEditDialogOpen(true);
  };

  // Save server changes
  const handleSaveChanges = async () => {
    try {
      await mcpService.updateServer(id, {
        name: formData.name,
        description: formData.description,
      });
      
      // Update the server data
      setServer({
        ...server,
        attributes: {
          ...server.attributes,
          name: formData.name,
          description: formData.description,
        },
      });
      
      setEditDialogOpen(false);
    } catch (err) {
      console.error("Error updating server:", err);
      setError("Failed to update server. Please try again later.");
    }
  };

  // Handle form field changes
  const handleFormChange = (e) => {
    const { name, value } = e.target;
    setFormData({
      ...formData,
      [name]: value
    });
  };

  // Open remove tool dialog
  const handleRemoveToolClick = (tool) => {
    setToolToRemove(tool);
    setDeleteToolDialogOpen(true);
  };

  // Remove tool from server
  const handleRemoveToolConfirm = async () => {
    try {
      await mcpService.removeToolFromServer(id, toolToRemove.id);
      fetchServerTools(); // Refresh tools
      setDeleteToolDialogOpen(false);
      setToolToRemove(null);
    } catch (err) {
      console.error("Error removing tool:", err);
      setError("Failed to remove tool. Please try again later.");
    }
  };

  // Open add tool dialog
  const handleAddToolClick = () => {
    if (availableTools.length === 0) {
      fetchAvailableTools();
    }
    setSelectedToolId("");
    setAddToolDialogOpen(true);
  };

  // Add tool to server
  const handleAddToolConfirm = async () => {
    if (!selectedToolId) return;
    
    try {
      await mcpService.addToolToServer(id, selectedToolId);
      fetchServerTools(); // Refresh tools
      setAddToolDialogOpen(false);
    } catch (err) {
      console.error("Error adding tool:", err);
      setError("Failed to add tool. Please try again later.");
    }
  };

  // Handle tool selection change
  const handleToolSelectionChange = (e) => {
    setSelectedToolId(e.target.value);
  };

  // End a session
  const handleEndSession = async (sessionId) => {
    try {
      await mcpService.endSession(sessionId);
      fetchActiveSessions(); // Refresh sessions
    } catch (err) {
      console.error("Error ending session:", err);
      setError("Failed to end session. Please try again later.");
    }
  };

  // Get status chip
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

  if (!server) {
    return (
      <Container>
        <Typography sx={{ textAlign: "center", mt: 4 }}>
          Server not found.
        </Typography>
        <Box sx={{ display: "flex", justifyContent: "center", mt: 2 }}>
          <Button 
            variant="contained" 
            onClick={() => navigate("/portal/mcp-servers")}
          >
            Back to Servers List
          </Button>
        </Box>
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
      {/* Header section with server name and actions */}
      <Box sx={{ mb: 3 }}>
        <Grid container spacing={2} alignItems="center">
          <Grid item xs>
            <Typography variant="h4" component="h1" gutterBottom>
              {server.attributes.name}
            </Typography>
            <Typography variant="body1" color="text.secondary">
              {server.attributes.description || "No description"}
            </Typography>
          </Grid>
          <Grid item>
            <Box sx={{ display: "flex", gap: 1 }}>
              {server.attributes.status === "running" ? (
                <Button
                  variant="outlined"
                  color="error"
                  startIcon={<StopIcon />}
                  onClick={() => handleServerAction("stop")}
                  disabled={!!actionInProgress}
                >
                  {actionInProgress === "stop" ? "Stopping..." : "Stop"}
                </Button>
              ) : (
                <Button
                  variant="outlined"
                  color="success"
                  startIcon={<PlayArrowIcon />}
                  onClick={() => handleServerAction("start")}
                  disabled={!!actionInProgress}
                >
                  {actionInProgress === "start" ? "Starting..." : "Start"}
                </Button>
              )}
              <Button
                variant="outlined"
                startIcon={<RefreshIcon />}
                onClick={() => handleServerAction("restart")}
                disabled={!!actionInProgress}
              >
                {actionInProgress === "restart" ? "Restarting..." : "Restart"}
              </Button>
              <Button
                variant="outlined"
                startIcon={<EditIcon />}
                onClick={handleEditClick}
              >
                Edit
              </Button>
            </Box>
          </Grid>
        </Grid>
      </Box>

      {/* Server status and info */}
      <StyledPaper sx={{ p: 3, mb: 3 }}>
        <Grid container spacing={2}>
          <Grid item xs={12} sm={6} md={3}>
            <Typography variant="subtitle2" gutterBottom>
              Status
            </Typography>
            {getStatusChip(server.attributes.status)}
            {sseConnected && (
              <Chip 
                label="Live Updates" 
                color="info" 
                size="small" 
                variant="outlined"
                sx={{ ml: 1 }}
              />
            )}
          </Grid>
          <Grid item xs={12} sm={6} md={3}>
            <Typography variant="subtitle2" gutterBottom>
              Endpoint
            </Typography>
            <Typography variant="body2">
              {server.attributes.endpoint}
            </Typography>
          </Grid>
          <Grid item xs={12} sm={6} md={3}>
            <Typography variant="subtitle2" gutterBottom>
              Created
            </Typography>
            <Typography variant="body2">
              {new Date(server.attributes.created_at).toLocaleString()}
            </Typography>
          </Grid>
          <Grid item xs={12} sm={6} md={3}>
            <Typography variant="subtitle2" gutterBottom>
              Last Updated
            </Typography>
            <Typography variant="body2">
              {new Date(server.attributes.updated_at).toLocaleString()}
            </Typography>
          </Grid>
        </Grid>
      </StyledPaper>

      {/* Tabs for different sections */}
      <Box sx={{ width: '100%' }}>
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs 
            value={activeTab} 
            onChange={handleTabChange} 
            aria-label="server management tabs"
          >
            <Tab label="Details" />
            <Tab label="Tools" />
            <Tab label="Sessions" />
            <Tab label="Logs" />
          </Tabs>
        </Box>
        
        {/* Details Tab */}
        <TabPanel value={activeTab} index={0}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom>
                Server Information
              </Typography>
              <StyledPaper>
                <Grid container spacing={2} sx={{ p: 2 }}>
                  <Grid item xs={12} sm={6}>
                    <Typography variant="subtitle2">Name</Typography>
                    <Typography variant="body1">{server.attributes.name}</Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <Typography variant="subtitle2">Description</Typography>
                    <Typography variant="body1">
                      {server.attributes.description || "No description provided"}
                    </Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <Typography variant="subtitle2">ID</Typography>
                    <Typography variant="body1">{server.id}</Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <Typography variant="subtitle2">Status</Typography>
                    <Typography variant="body1">{server.attributes.status}</Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <Typography variant="subtitle2">Endpoint</Typography>
                    <Typography variant="body1">{server.attributes.endpoint}</Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <Typography variant="subtitle2">User ID</Typography>
                    <Typography variant="body1">{server.attributes.user_id}</Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <Typography variant="subtitle2">Created At</Typography>
                    <Typography variant="body1">
                      {new Date(server.attributes.created_at).toLocaleString()}
                    </Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <Typography variant="subtitle2">Updated At</Typography>
                    <Typography variant="body1">
                      {new Date(server.attributes.updated_at).toLocaleString()}
                    </Typography>
                  </Grid>
                </Grid>
              </StyledPaper>
            </Grid>
            
            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom>
                How to Connect
              </Typography>
              <StyledPaper sx={{ p: 3 }}>
                <Typography variant="body1" paragraph>
                  To connect to this MCP server, use the endpoint URL in your client application:
                </Typography>
                <Box 
                  sx={{ 
                    backgroundColor: 'background.paper', 
                    p: 2, 
                    borderRadius: 1,
                    fontFamily: 'monospace'
                  }}
                >
                  {server.attributes.endpoint}
                </Box>
                <Typography variant="body1" sx={{ mt: 2 }} paragraph>
                  Example session creation:
                </Typography>
                <Box 
                  sx={{ 
                    backgroundColor: 'background.paper', 
                    p: 2, 
                    borderRadius: 1,
                    fontFamily: 'monospace'
                  }}
                >
                  {`POST /mcp/sessions
{
  "mcp_server_id": ${server.id},
  "client_id": "your-client-id"
}`}
                </Box>
              </StyledPaper>
            </Grid>
          </Grid>
        </TabPanel>
        
        {/* Tools Tab */}
        <TabPanel value={activeTab} index={1}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <Typography variant="h6">
              Associated Tools ({tools.length})
            </Typography>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddToolClick}
            >
              Add Tool
            </Button>
          </Box>
          
          {tools.length === 0 ? (
            <StyledPaper sx={{ p: 4, textAlign: 'center' }}>
              <Typography variant="body1">
                No tools associated with this server yet. Add tools to enable functionality.
              </Typography>
            </StyledPaper>
          ) : (
            <StyledPaper>
              <List>
                {tools.map((tool) => (
                  <ListItem key={tool.id}>
                    <ListItemText
                      primary={tool.attributes.name}
                      secondary={tool.attributes.description}
                    />
                    <ListItemSecondaryAction>
                      <IconButton
                        edge="end"
                        aria-label="delete"
                        onClick={() => handleRemoveToolClick(tool)}
                      >
                        <DeleteIcon />
                      </IconButton>
                    </ListItemSecondaryAction>
                  </ListItem>
                ))}
              </List>
            </StyledPaper>
          )}
        </TabPanel>
        
        {/* Sessions Tab */}
        <TabPanel value={activeTab} index={2}>
          <Typography variant="h6" gutterBottom>
            Active Sessions ({sessions.length})
          </Typography>
          
          {sessions.length === 0 ? (
            <StyledPaper sx={{ p: 4, textAlign: 'center' }}>
              <Typography variant="body1">
                No active sessions for this server.
              </Typography>
            </StyledPaper>
          ) : (
            <StyledPaper>
              <List>
                {sessions.map((session) => (
                  <ListItem key={session.id}>
                    <ListItemText
                      primary={`Session ID: ${session.attributes.session_id}`}
                      secondary={`Client: ${session.attributes.client_id} • Last seen: ${new Date(session.attributes.last_seen).toLocaleString()}`}
                    />
                    <ListItemSecondaryAction>
                      <Button
                        variant="outlined"
                        color="error"
                        size="small"
                        onClick={() => handleEndSession(session.attributes.session_id)}
                      >
                        End Session
                      </Button>
                    </ListItemSecondaryAction>
                  </ListItem>
                ))}
              </List>
            </StyledPaper>
          )}
        </TabPanel>
        
        {/* Logs Tab */}
        <TabPanel value={activeTab} index={3}>
          <Typography variant="h6" gutterBottom>
            Server Logs
          </Typography>
          
          {logs.length === 0 ? (
            <StyledPaper sx={{ p: 4, textAlign: 'center' }}>
              <Typography variant="body1">
                No logs available. Logs will appear here when the server is active.
              </Typography>
            </StyledPaper>
          ) : (
            <StyledPaper sx={{ 
              p: 2, 
              maxHeight: '400px', 
              overflow: 'auto',
              fontFamily: 'monospace',
              fontSize: '0.85rem'
            }}>
              {logs.map((log, index) => (
                <Box key={index} sx={{ mb: 1, p: 1, borderBottom: '1px solid rgba(0,0,0,0.1)' }}>
                  <Box component="span" sx={{ color: 'text.secondary', mr: 1 }}>
                    {new Date(log.timestamp).toLocaleString()}
                  </Box>
                  <Box 
                    component="span" 
                    sx={{ 
                      color: log.level === 'error' ? 'error.main' : 
                             log.level === 'warning' ? 'warning.main' : 'text.primary'
                    }}
                  >
                    {log.message}
                  </Box>
                </Box>
              ))}
            </StyledPaper>
          )}
        </TabPanel>
      </Box>

      {/* Edit Server Dialog */}
      <Dialog open={editDialogOpen} onClose={() => setEditDialogOpen(false)}>
        <DialogTitle>Edit MCP Server</DialogTitle>
        <DialogContent>
          <Box sx={{ pt: 1 }}>
            <Typography variant="body2" gutterBottom>
              Update server information:
            </Typography>
            <Grid container spacing={2}>
              <Grid item xs={12}>
                <Typography variant="subtitle2" gutterBottom>
                  Name
                </Typography>
                <Box 
                  component="input"
                  sx={{
                    width: '100%',
                    p: 1.5,
                    borderRadius: 1,
                    border: '1px solid',
                    borderColor: 'divider',
                    mb: 2
                  }}
                  name="name"
                  value={formData.name}
                  onChange={handleFormChange}
                />
              </Grid>
              <Grid item xs={12}>
                <Typography variant="subtitle2" gutterBottom>
                  Description
                </Typography>
                <Box 
                  component="textarea"
                  sx={{
                    width: '100%',
                    p: 1.5,
                    borderRadius: 1,
                    border: '1px solid',
                    borderColor: 'divider',
                    minHeight: '100px',
                    fontFamily: 'inherit',
                    fontSize: 'inherit'
                  }}
                  name="description"
                  value={formData.description}
                  onChange={handleFormChange}
                />
              </Grid>
            </Grid>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditDialogOpen(false)}>Cancel</Button>
          <PrimaryButton onClick={handleSaveChanges}>Save Changes</PrimaryButton>
        </DialogActions>
      </Dialog>

      {/* Remove Tool Dialog */}
      <Dialog
        open={deleteToolDialogOpen}
        onClose={() => setDeleteToolDialogOpen(false)}
      >
        <DialogTitle>Remove Tool</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to remove tool "{toolToRemove?.attributes.name}" from this server?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteToolDialogOpen(false)}>Cancel</Button>
          <DangerButton onClick={handleRemoveToolConfirm}>Remove</DangerButton>
        </DialogActions>
      </Dialog>

      {/* Add Tool Dialog */}
      <Dialog
        open={addToolDialogOpen}
        onClose={() => setAddToolDialogOpen(false)}
      >
        <DialogTitle>Add Tool to Server</DialogTitle>
        <DialogContent>
          <Box sx={{ mt: 2, minWidth: '400px' }}>
            <Typography variant="subtitle2" gutterBottom>
              Select a tool to add:
            </Typography>
            <Box 
              component="select"
              sx={{
                width: '100%',
                p: 1.5,
                borderRadius: 1,
                border: '1px solid',
                borderColor: 'divider',
                mb: 2
              }}
              value={selectedToolId}
              onChange={handleToolSelectionChange}
            >
              <option value="">-- Select a tool --</option>
              {availableTools.map(tool => (
                <option key={tool.id} value={tool.id}>
                  {tool.attributes.name}
                </option>
              ))}
            </Box>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAddToolDialogOpen(false)}>Cancel</Button>
          <PrimaryButton 
            onClick={handleAddToolConfirm}
            disabled={!selectedToolId}
          >
            Add Tool
          </PrimaryButton>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default MCPServerDetailView;
