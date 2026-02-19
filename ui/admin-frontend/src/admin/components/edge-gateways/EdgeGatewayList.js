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
  Menu,
  Snackbar,
  Link,
} from '@mui/material';
import {
  Refresh as RefreshIcon,
  Visibility as ViewIcon,
  CloudSync as PushIcon,
  MoreVert as MoreVertIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import edgeGatewayService from '../../services/edgeGatewayService';
import useNamespaces from '../../hooks/useNamespaces';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import { useSyncStatus } from '../../context/SyncStatusContext';
import PushConfigurationModal from './PushConfigurationModal';
import RemoveEdgeModal from './RemoveEdgeModal';
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryOutlineButton,
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
} from '../../styles/sharedStyles';

const EdgeGatewayList = () => {
  const navigate = useNavigate();
  const { getAvailableNamespaces } = useNamespaces();
  const { features } = useSystemFeatures();
  const { syncStatus: globalSyncStatus } = useSyncStatus();

  const [edgeGateways, setEdgeGateways] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [selectedNamespace, setSelectedNamespace] = useState('');
  const [pushModalOpen, setPushModalOpen] = useState(false);
  const [removeModalOpen, setRemoveModalOpen] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(new Date());
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });

  // Menu state for actions
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedEdge, setSelectedEdge] = useState(null);

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

  const handleMenuOpen = (event, edge) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedEdge(edge);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleViewDetail = (edgeId) => {
    navigate(`/admin/edge-gateways/${edgeId}`);
  };

  const handleRemoveClick = () => {
    handleMenuClose();
    setRemoveModalOpen(true);
  };

  const handleRemoveSuccess = () => {
    setSnackbar({
      open: true,
      message: `Edge gateway "${selectedEdge?.edgeId}" removed successfully`,
      severity: 'success',
    });
    fetchEdgeGateways();
  };

  const handleSnackbarClose = () => {
    setSnackbar({ ...snackbar, open: false });
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

  // Get expected checksum for a namespace from the global sync status
  const getExpectedChecksum = (namespace) => {
    const nsStatus = globalSyncStatus?.data?.find(ns => ns.namespace === (namespace || 'default'));
    return nsStatus?.expected_checksum || null;
  };

  // Get sync status chip with checksum info tooltip
  const getSyncStatusChip = (edge) => {
    const syncStatusDisplay = edgeGatewayService.getSyncStatusDisplay(edge.syncStatus);
    const expectedChecksum = getExpectedChecksum(edge.namespace);
    const loadedChecksum = edge.loadedChecksum;

    // Build tooltip content
    let tooltipContent = '';
    if (loadedChecksum) {
      tooltipContent = `Loaded: ${loadedChecksum.substring(0, 12)}...`;
      if (edge.syncStatus !== 'in_sync' && expectedChecksum) {
        tooltipContent += `\nExpected: ${expectedChecksum.substring(0, 12)}...`;
      }
    } else {
      tooltipContent = 'No config checksum reported';
    }

    return (
      <Tooltip title={<span style={{ whiteSpace: 'pre-line' }}>{tooltipContent}</span>} arrow>
        <Chip
          label={syncStatusDisplay.label}
          color={syncStatusDisplay.color}
          size="small"
          variant="outlined"
        />
      </Tooltip>
    );
  };

  const availableNamespaces = getAvailableNamespaces();

  return (
    <Box sx={{ p: 0 }}>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Edge Gateways</Typography>
        <Box display="flex" gap={2} alignItems="center">
          <Typography variant="caption" color="textSecondary">
            Last updated: {lastRefresh.toLocaleTimeString()}
          </Typography>
          <SecondaryOutlineButton
            startIcon={<RefreshIcon />}
            onClick={handleRefresh}
            disabled={loading}
          >
            Refresh
          </SecondaryOutlineButton>
          <PrimaryButton
            variant="contained"
            startIcon={<PushIcon />}
            onClick={() => setPushModalOpen(true)}
            disabled={loading}
          >
            Push Configuration
          </PrimaryButton>
        </Box>
      </TitleBox>

      <Box sx={{ p: 3 }}>
        {/* Namespace Filter - Enterprise Edition only */}
        {features.hub_spoke_multi_tenant && (
          <Box mb={3}>
            <FormControl size="small" style={{ minWidth: 200 }}>
              <InputLabel>Filter by Namespace</InputLabel>
              <Select
                value={selectedNamespace}
                label="Filter by Namespace"
                onChange={handleNamespaceChange}
              >
                <MenuItem value="">All Namespaces</MenuItem>
                {availableNamespaces.map((namespace) => (
                  <MenuItem key={namespace.name} value={namespace.name}>
                    {namespace.name === 'global' ? 'Global' : namespace.name} ({namespace.edgeCount} edges)
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Box>
        )}

        {/* Community Edition - Show info banner */}
        {!features.hub_spoke_multi_tenant && (
          <Alert severity="info" sx={{ mb: 2 }}>
            Multi-tenant namespace support is available in Enterprise Edition.{' '}
            <Link href="https://tyk.io/enterprise" target="_blank" rel="noopener">
              Learn more
            </Link>
          </Alert>
        )}

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
          <StyledPaper>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Edge ID</StyledTableHeaderCell>
                  {features.hub_spoke_multi_tenant && (
                    <StyledTableHeaderCell>Namespace</StyledTableHeaderCell>
                  )}
                  <StyledTableHeaderCell>Connection</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Config Sync</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Version</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Last Heartbeat</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {edgeGateways.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={features.hub_spoke_multi_tenant ? 7 : 6} align="center">
                      <Typography variant="body2" color="textSecondary" py={4}>
                        {selectedNamespace
                          ? `No edge gateways found in ${selectedNamespace} namespace`
                          : 'No edge gateways found'
                        }
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  edgeGateways.map((edge) => (
                    <StyledTableRow
                      key={edge.id}
                      onClick={() => handleViewDetail(edge.edgeId)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>
                        <Typography variant="body2" fontWeight="medium">
                          {edge.edgeId}
                        </Typography>
                      </StyledTableCell>
                      {features.hub_spoke_multi_tenant && (
                        <StyledTableCell>
                          <Chip
                            label={edge.namespace}
                            size="small"
                            variant="outlined"
                            color={edge.namespace === 'global' ? 'default' : 'primary'}
                          />
                        </StyledTableCell>
                      )}
                      <StyledTableCell>
                        {getStatusChip(edge)}
                      </StyledTableCell>
                      <StyledTableCell>
                        {getSyncStatusChip(edge)}
                      </StyledTableCell>
                      <StyledTableCell>
                        <Typography variant="body2">
                          {edge.version || 'Unknown'}
                        </Typography>
                        {edge.buildHash && (
                          <Typography variant="caption" color="textSecondary" display="block">
                            {edge.buildHash.substring(0, 8)}
                          </Typography>
                        )}
                      </StyledTableCell>
                      <StyledTableCell>
                        <Typography variant="body2">
                          {formatHeartbeat(edge.lastHeartbeat)}
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, edge)}
                        >
                          <MoreVertIcon />
                        </IconButton>
                      </StyledTableCell>
                    </StyledTableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </StyledPaper>
        )}
        
        <Menu
          anchorEl={anchorEl}
          open={Boolean(anchorEl)}
          onClose={handleMenuClose}
        >
          <MenuItem
            onClick={() => {
              handleViewDetail(selectedEdge?.edgeId);
              handleMenuClose();
            }}
          >
            <ViewIcon fontSize="small" sx={{ mr: 1 }} />
            View Details
          </MenuItem>
          <MenuItem
            onClick={handleRemoveClick}
            sx={{ color: 'error.main' }}
          >
            <DeleteIcon fontSize="small" sx={{ mr: 1 }} />
            Remove Entry
          </MenuItem>
        </Menu>
      </Box>

      <PushConfigurationModal
        open={pushModalOpen}
        onClose={() => setPushModalOpen(false)}
        onSuccess={handleRefresh}
      />

      <RemoveEdgeModal
        open={removeModalOpen}
        onClose={() => setRemoveModalOpen(false)}
        edge={selectedEdge}
        onSuccess={handleRemoveSuccess}
      />

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleSnackbarClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      >
        <Alert onClose={handleSnackbarClose} severity={snackbar.severity} sx={{ width: '100%' }}>
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default EdgeGatewayList;