import React, { useState, useEffect } from 'react';
import {
  Box,
  Paper,
  Typography,
  Grid,
  Chip,
  Card,
  CardContent,
  IconButton,
  Tooltip,
  Divider,
  Alert,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Button,
} from '@mui/material';
import {
  ArrowBack as BackIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';
import { useParams, useNavigate, Link } from 'react-router-dom';
import pluginService from '../../services/pluginService';
import agentService from '../../services/agentService';
import { isAgentPlugin } from '../../constants/agentTypes';
import {
  TitleBox,
  ContentBox,
  SecondaryLinkButton,
  PrimaryButton,
  DangerButton,
} from '../../styles/sharedStyles';

const PluginDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();

  const [plugin, setPlugin] = useState(null);
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [agentsLoading, setAgentsLoading] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchPlugin();
  }, [id]);

  const fetchPlugin = async () => {
    setLoading(true);
    setError(null);

    try {
      const result = await pluginService.getPlugin(id);
      setPlugin(result);

      // If this is an agent plugin, fetch associated agents
      if (isAgentPlugin(result)) {
        fetchAgents();
      }
    } catch (err) {
      console.error('Error fetching plugin:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchAgents = async () => {
    setAgentsLoading(true);
    try {
      // Fetch agents for this plugin
      const result = await agentService.listAgents(1, 100);
      const pluginAgents = result.data.filter(agent => agent.pluginId === parseInt(id));
      setAgents(pluginAgents);
    } catch (err) {
      console.error('Error fetching agents:', err);
    } finally {
      setAgentsLoading(false);
    }
  };

  const handleEdit = () => {
    navigate(`/admin/plugins/${id}/edit`);
  };

  const handleDelete = async () => {
    if (window.confirm(`Are you sure you want to delete the plugin "${plugin.name}"?`)) {
      try {
        await pluginService.deletePlugin(id);
        navigate('/admin/plugins');
      } catch (err) {
        setError(err.message);
      }
    }
  };

  const formatTimestamp = (timestamp) => {
    if (!timestamp) return 'Unknown';
    return new Date(timestamp).toLocaleString();
  };

  const renderConfiguration = (config) => {
    if (!config || Object.keys(config).length === 0) {
      return (
        <Typography variant="body2" color="textSecondary">
          No configuration specified
        </Typography>
      );
    }

    return (
      <Box sx={{ 
        backgroundColor: 'grey.50', 
        p: 2, 
        borderRadius: 1,
        fontFamily: 'monospace',
        fontSize: '0.875rem',
        overflow: 'auto'
      }}>
        <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>
          {JSON.stringify(config, null, 2)}
        </pre>
      </Box>
    );
  };

  const renderLLMAssociations = (llms) => {
    if (!llms || llms.length === 0) {
      return (
        <Typography variant="body2" color="textSecondary">
          This plugin is not associated with any LLMs
        </Typography>
      );
    }

    return (
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>LLM Name</TableCell>
              <TableCell>Vendor</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {llms.map((llm) => (
              <TableRow key={llm.id}>
                <TableCell>
                  <Typography variant="body2" fontWeight="medium">
                    {llm.name}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="body2" color="textSecondary">
                    {llm.vendor}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Chip
                    label={llm.isActive ? 'Active' : 'Inactive'}
                    size="small"
                    color={llm.isActive ? 'success' : 'default'}
                    variant={llm.isActive ? 'filled' : 'outlined'}
                  />
                </TableCell>
                <TableCell>
                  <Button
                    component={Link}
                    to={`/admin/llms/${llm.id}`}
                    size="small"
                    variant="outlined"
                  >
                    View LLM
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    );
  };

  const renderAgentAssociations = () => {
    if (agentsLoading) {
      return (
        <Box display="flex" justifyContent="center" p={2}>
          <CircularProgress size={24} />
        </Box>
      );
    }

    if (!agents || agents.length === 0) {
      return (
        <Box>
          <Typography variant="body2" color="textSecondary" mb={2}>
            No agent configurations using this plugin
          </Typography>
          <Button
            variant="contained"
            color="primary"
            size="small"
            onClick={() => navigate('/admin/agents/new')}
          >
            Create Agent
          </Button>
        </Box>
      );
    }

    return (
      <Box>
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Agent Name</TableCell>
                <TableCell>App</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {agents.map((agent) => (
                <TableRow key={agent.id}>
                  <TableCell>
                    <Typography variant="body2" fontWeight="medium">
                      {agent.name}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" color="textSecondary">
                      {agent.app?.name || 'Unknown'}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={agent.isActive ? 'Active' : 'Inactive'}
                      size="small"
                      color={agent.isActive ? 'success' : 'default'}
                      variant={agent.isActive ? 'filled' : 'outlined'}
                    />
                  </TableCell>
                  <TableCell>
                    <Button
                      component={Link}
                      to={`/admin/agents/${agent.id}`}
                      size="small"
                      variant="outlined"
                    >
                      View Agent
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        <Box mt={2}>
          <Button
            variant="outlined"
            size="small"
            onClick={() => navigate('/admin/agents/new')}
          >
            Create Another Agent
          </Button>
        </Box>
      </Box>
    );
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  if (error && !plugin) {
    return (
      <Box>
        <TitleBox>
          <Box display="flex" alignItems="center" gap={2}>
            <SecondaryLinkButton
              component={Link}
              to="/admin/plugins"
              startIcon={<BackIcon />}
            >
              Back to Plugins
            </SecondaryLinkButton>
            <Typography variant="h4">Plugin Details</Typography>
          </Box>
        </TitleBox>
        <ContentBox>
          <Alert severity="error">
            {error}
          </Alert>
        </ContentBox>
      </Box>
    );
  }

  if (!plugin) {
    return (
      <Box>
        <TitleBox>
          <Box display="flex" alignItems="center" gap={2}>
            <SecondaryLinkButton
              component={Link}
              to="/admin/plugins"
              startIcon={<BackIcon />}
            >
              Back to Plugins
            </SecondaryLinkButton>
            <Typography variant="h4">Plugin Not Found</Typography>
          </Box>
        </TitleBox>
        <ContentBox>
          <Alert severity="warning">
            Plugin not found or you don't have permission to view it.
          </Alert>
        </ContentBox>
      </Box>
    );
  }

  return (
    <Box>
      <TitleBox top="64px">
        <Box display="flex" alignItems="center" gap={2}>
          <Typography variant="headingXLarge">
            {plugin.name}
          </Typography>
          <Chip
            label={plugin.isActive ? 'Active' : 'Inactive'}
            color={plugin.isActive ? 'success' : 'default'}
            variant={plugin.isActive ? 'filled' : 'outlined'}
          />
        </Box>
        <SecondaryLinkButton
          component={Link}
          to="/admin/plugins"
          startIcon={<BackIcon />}
        >
          Back to Plugins
        </SecondaryLinkButton>
      </TitleBox>

      <Box sx={{ p: 3 }}>
        {error && (
          <Alert severity="error" sx={{ mb: 3 }}>
            {error}
          </Alert>
        )}

        <Grid container spacing={3}>
          {/* Basic Information */}
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  Basic Information
                </Typography>
                <Divider sx={{ mb: 2 }} />
                
                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Plugin Name
                  </Typography>
                  <Typography variant="body1" fontWeight="medium">
                    {plugin.name}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Description
                  </Typography>
                  <Typography variant="body1">
                    {plugin.description || 'No description provided'}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Hook Type
                  </Typography>
                  <Chip
                    label={pluginService.getHookTypeLabel(plugin.hookType)}
                    size="small"
                    variant="outlined"
                    color="primary"
                  />
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Namespace
                  </Typography>
                  <Chip
                    label={plugin.namespace}
                    size="small"
                    variant="outlined"
                    color={plugin.namespace === 'global' ? 'default' : 'secondary'}
                  />
                </Box>
              </CardContent>
            </Card>
          </Grid>

          {/* Technical Details */}
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  Technical Details
                </Typography>
                <Divider sx={{ mb: 2 }} />
                
                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Command
                  </Typography>
                  <Typography variant="body1" sx={{ 
                    fontFamily: 'monospace', 
                    fontSize: '0.875rem',
                    wordBreak: 'break-all',
                    backgroundColor: 'grey.50',
                    p: 1,
                    borderRadius: 1
                  }}>
                    {plugin.command}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Checksum
                  </Typography>
                  <Typography variant="body1" sx={{ fontFamily: 'monospace', fontSize: '0.875rem' }}>
                    {plugin.checksum || 'Not specified'}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Created At
                  </Typography>
                  <Typography variant="body1">
                    {formatTimestamp(plugin.createdAt)}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Updated At
                  </Typography>
                  <Typography variant="body1">
                    {formatTimestamp(plugin.updatedAt)}
                  </Typography>
                </Box>
              </CardContent>
            </Card>
          </Grid>

          {/* Configuration */}
          <Grid item xs={12}>
            <Card>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  Configuration
                </Typography>
                <Divider sx={{ mb: 2 }} />
                {renderConfiguration(plugin.config)}
              </CardContent>
            </Card>
          </Grid>

          {/* LLM Associations */}
          <Grid item xs={12}>
            <Card>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  Associated LLMs
                </Typography>
                <Divider sx={{ mb: 2 }} />
                {renderLLMAssociations(plugin.llms)}
              </CardContent>
            </Card>
          </Grid>

          {/* Agent Associations (only for agent plugins) */}
          {isAgentPlugin(plugin) && (
            <Grid item xs={12}>
              <Card>
                <CardContent>
                  <Typography variant="h6" gutterBottom>
                    Agent Configurations
                  </Typography>
                  <Divider sx={{ mb: 2 }} />
                  {renderAgentAssociations()}
                </CardContent>
              </Card>
            </Grid>
          )}
        </Grid>

        <Box
          mt={4}
          display="flex"
          justifyContent="flex-end"
          alignItems="center"
          gap={2}
        >
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={handleEdit}
          >
            Edit
          </PrimaryButton>
          <DangerButton
            startIcon={<DeleteIcon />}
            onClick={handleDelete}
          >
            Delete
          </DangerButton>
        </Box>
      </Box>
    </Box>
  );
};

export default PluginDetail;