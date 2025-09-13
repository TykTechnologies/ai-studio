import React, { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Paper,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Chip,
  IconButton,
  Tooltip,
  Button,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Alert,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
} from '@mui/material';
import {
  Refresh as RefreshIcon,
  Visibility as ViewIcon,
  CloudSync as PushIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import edgeGatewayService from '../../services/edgeGatewayService';
import useNamespaces from '../../hooks/useNamespaces';
import PushConfigurationModal from './PushConfigurationModal';
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
} from '../../styles/sharedStyles';

const EdgeGatewayList = () => {
  const navigate = useNavigate();
  const { getAvailableNamespaces } = useNamespaces();
  
  const [edgeGateways, setEdgeGateways] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [selectedNamespace, setSelectedNamespace] = useState('');
  const [pushModalOpen, setPushModalOpen] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(new Date());

  const fetchEdgeGateways = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    try {
      const result = selectedNamespace && selectedNamespace !== 'all'
        ? await edgeGatewayService.getEdgesInNamespace(selectedNamespace === 'global' ? '' : selectedNamespace)
        : await edgeGatewayService.listEdgeGateways();
      
      setEdgeGateways(result.data || []);
      setLastRefresh(new Date());
    } catch (err) {
      console.error('Error fetching edge gateways:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [selectedNamespace]);

  useEffect(() => {
    fetchEdgeGateways();
  }, [fetchEdgeGateways]);

  // Auto-refresh every 30 seconds
  useEffect(() => {
    const interval = setInterval(fetchEdgeGateways, 30000);
    return () => clearInterval(interval);
  }, [fetchEdgeGateways]);

  const handleRefresh = () => {
    fetchEdgeGateways();
  };

  const handleViewDetail = (edgeId) => {
    navigate(`/admin/edge-gateways/${edgeId}`);
  };

  const handleNamespaceChange = (event) => {
    setSelectedNamespace(event.target.value);
  };

  const getStatusChip = (edge) => {
    const connectionStatus = edgeGatewayService.getConnectionStatus(edge.lastHeartbeat);
    
    return (
      <Chip
        label={connectionStatus.label}
        color={connectionStatus.color}
        size="small"
        variant="outlined"
      />
    );
  };

  const formatHeartbeat = (lastHeartbeat) => {
    return edgeGatewayService.formatLastHeartbeat(lastHeartbeat);
  };

  const availableNamespaces = getAvailableNamespaces();

  return (
    <Box>
      <TitleBox>
        <Box display="flex" justifyContent="space-between" alignItems="center">
          <Typography variant="h4" component="h1">
            Edge Gateways
          </Typography>
          <Box display="flex" gap={2} alignItems="center">
            <Typography variant="caption" color="textSecondary">
              Last updated: {lastRefresh.toLocaleTimeString()}
            </Typography>
            <PrimaryButton
              startIcon={<PushIcon />}
              onClick={() => setPushModalOpen(true)}
              disabled={loading}
            >
              Push Configuration
            </PrimaryButton>
            <Tooltip title="Refresh">
              <IconButton onClick={handleRefresh} disabled={loading}>
                <RefreshIcon />
              </IconButton>
            </Tooltip>
          </Box>
        </Box>
      </TitleBox>

      <ContentBox>
        {/* Namespace Filter */}
        <Box mb={3}>
          <FormControl size="small" style={{ minWidth: 200 }}>
            <InputLabel>Filter by Namespace</InputLabel>
            <Select
              value={selectedNamespace}
              label="Filter by Namespace"
              onChange={handleNamespaceChange}
            >
              <MenuItem value="">All Namespaces</MenuItem>
              <MenuItem value="global">Global</MenuItem>
              {availableNamespaces.map((namespace) => (
                <MenuItem key={namespace.name} value={namespace.name}>
                  {namespace.name} ({namespace.edgeCount} edges)
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Box>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        {loading && !edgeGateways.length ? (
          <Box display="flex" justifyContent="center" p={4}>
            <CircularProgress />
          </Box>
        ) : (
          <TableContainer component={Paper} variant="outlined">
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>Edge ID</TableCell>
                  <TableCell>Namespace</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Version</TableCell>
                  <TableCell>Last Heartbeat</TableCell>
                  <TableCell>Session ID</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {edgeGateways.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} align="center">
                      <Typography variant="body2" color="textSecondary" py={4}>
                        {selectedNamespace 
                          ? `No edge gateways found in ${selectedNamespace} namespace`
                          : 'No edge gateways found'
                        }
                      </Typography>
                    </TableCell>
                  </TableRow>
                ) : (
                  edgeGateways.map((edge) => (
                    <TableRow key={edge.id} hover>
                      <TableCell>
                        <Typography variant="body2" fontWeight="medium">
                          {edge.edgeId}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Chip
                          label={edge.namespace}
                          size="small"
                          variant="outlined"
                          color={edge.namespace === 'global' ? 'default' : 'primary'}
                        />
                      </TableCell>
                      <TableCell>
                        {getStatusChip(edge)}
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2">
                          {edge.version || 'Unknown'}
                        </Typography>
                        {edge.buildHash && (
                          <Typography variant="caption" color="textSecondary" display="block">
                            {edge.buildHash.substring(0, 8)}
                          </Typography>
                        )}
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2">
                          {formatHeartbeat(edge.lastHeartbeat)}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" sx={{ maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                          {edge.sessionId || 'N/A'}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Tooltip title="View Details">
                          <IconButton
                            size="small"
                            onClick={() => handleViewDetail(edge.edgeId)}
                          >
                            <ViewIcon />
                          </IconButton>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </ContentBox>

      <PushConfigurationModal
        open={pushModalOpen}
        onClose={() => setPushModalOpen(false)}
        onSuccess={handleRefresh}
      />
    </Box>
  );
};

export default EdgeGatewayList;