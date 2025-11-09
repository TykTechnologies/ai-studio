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
  TableRow,
} from '@mui/material';
import {
  ArrowBack as BackIcon,
  Refresh as RefreshIcon,
  CloudSync as PushIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';
import { useParams, useNavigate, Link } from 'react-router-dom';
import edgeGatewayService from '../../services/edgeGatewayService';
import PushConfigurationModal from './PushConfigurationModal';
import RemoveEdgeModal from './RemoveEdgeModal';
import {
  TitleBox,
  ContentBox,
  SecondaryLinkButton,
  PrimaryButton,
  SecondaryOutlineButton,
} from '../../styles/sharedStyles';

const EdgeGatewayDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  
  const [edgeGateway, setEdgeGateway] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [pushModalOpen, setPushModalOpen] = useState(false);
  const [removeModalOpen, setRemoveModalOpen] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(new Date());

  const fetchEdgeGateway = async () => {
    setLoading(true);
    setError(null);
    
    try {
      const result = await edgeGatewayService.getEdgeGateway(id);
      setEdgeGateway(result);
      setLastRefresh(new Date());
    } catch (err) {
      console.error('Error fetching edge gateway:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchEdgeGateway();
  }, [id]);

  // Auto-refresh every 30 seconds
  useEffect(() => {
    const interval = setInterval(fetchEdgeGateway, 30000);
    return () => clearInterval(interval);
  }, []);

  const handleRefresh = () => {
    fetchEdgeGateway();
  };

  const handleRemoveSuccess = () => {
    // Navigate back to the list after successful removal
    navigate('/admin/edge-gateways');
  };

  const getStatusInfo = (edge) => {
    if (!edge) return null;
    return edgeGatewayService.getConnectionStatus(edge.lastHeartbeat);
  };

  const formatTimestamp = (timestamp) => {
    if (!timestamp) return 'Never';
    return new Date(timestamp).toLocaleString();
  };

  const renderMetadata = (metadata) => {
    if (!metadata || Object.keys(metadata).length === 0) {
      return (
        <Typography variant="body2" color="textSecondary">
          No metadata available
        </Typography>
      );
    }

    return (
      <TableContainer>
        <Table size="small">
          <TableBody>
            {Object.entries(metadata).map(([key, value]) => (
              <TableRow key={key}>
                <TableCell component="th" scope="row" sx={{ fontWeight: 'medium', width: '30%' }}>
                  {key}
                </TableCell>
                <TableCell>
                  {typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    );
  };

  if (loading && !edgeGateway) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  if (error && !edgeGateway) {
    return (
      <Box>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Edge Gateway Details</Typography>
          <SecondaryLinkButton
            component={Link}
            to="/admin/edge-gateways"
            startIcon={<BackIcon />}
          >
            Back to Edge Gateways
          </SecondaryLinkButton>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Alert severity="error">
            {error}
          </Alert>
        </Box>
      </Box>
    );
  }

  if (!edgeGateway) {
    return (
      <Box>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Edge Gateway Not Found</Typography>
          <SecondaryLinkButton
            component={Link}
            to="/admin/edge-gateways"
            startIcon={<BackIcon />}
          >
            Back to Edge Gateways
          </SecondaryLinkButton>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Alert severity="warning">
            Edge gateway not found or you don't have permission to view it.
          </Alert>
        </Box>
      </Box>
    );
  }

  const statusInfo = getStatusInfo(edgeGateway);

  return (
    <Box>
      <TitleBox top="64px">
        <Box display="flex" alignItems="center" gap={2}>
          <Typography variant="headingXLarge">
            {edgeGateway.edgeId}
          </Typography>
          <Chip
            label={statusInfo.label}
            color={statusInfo.color}
            size="small"
          />
        </Box>
        <SecondaryLinkButton
          component={Link}
          to="/admin/edge-gateways"
          startIcon={<BackIcon />}
        >
          Back to Edge Gateways
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
                    Edge ID
                  </Typography>
                  <Typography variant="body1" fontWeight="medium">
                    {edgeGateway.edgeId}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Namespace
                  </Typography>
                  <Chip
                    label={edgeGateway.namespace}
                    size="small"
                    variant="outlined"
                    color={edgeGateway.namespace === 'global' ? 'default' : 'primary'}
                  />
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Status
                  </Typography>
                  <Typography variant="body1">
                    {edgeGateway.status}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Session ID
                  </Typography>
                  <Typography variant="body1" sx={{ wordBreak: 'break-all' }}>
                    {edgeGateway.sessionId || 'N/A'}
                  </Typography>
                </Box>
              </CardContent>
            </Card>
          </Grid>

          {/* Version & Build Information */}
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  Version Information
                </Typography>
                <Divider sx={{ mb: 2 }} />
                
                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Version
                  </Typography>
                  <Typography variant="body1" fontWeight="medium">
                    {edgeGateway.version || 'Unknown'}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Build Hash
                  </Typography>
                  <Typography variant="body1" sx={{ fontFamily: 'monospace', fontSize: '0.875rem' }}>
                    {edgeGateway.buildHash || 'N/A'}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Last Heartbeat
                  </Typography>
                  <Typography variant="body1">
                    {edgeGatewayService.formatLastHeartbeat(edgeGateway.lastHeartbeat)}
                  </Typography>
                  {edgeGateway.lastHeartbeat && (
                    <Typography variant="caption" color="textSecondary" display="block">
                      {formatTimestamp(edgeGateway.lastHeartbeat)}
                    </Typography>
                  )}
                </Box>
              </CardContent>
            </Card>
          </Grid>

          {/* Timestamps */}
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  Timestamps
                </Typography>
                <Divider sx={{ mb: 2 }} />
                
                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Created At
                  </Typography>
                  <Typography variant="body1">
                    {formatTimestamp(edgeGateway.createdAt)}
                  </Typography>
                </Box>

                <Box mb={2}>
                  <Typography variant="body2" color="textSecondary">
                    Updated At
                  </Typography>
                  <Typography variant="body1">
                    {formatTimestamp(edgeGateway.updatedAt)}
                  </Typography>
                </Box>
              </CardContent>
            </Card>
          </Grid>

          {/* Metadata */}
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  Metadata
                </Typography>
                <Divider sx={{ mb: 2 }} />
                {renderMetadata(edgeGateway.metadata)}
              </CardContent>
            </Card>
          </Grid>
        </Grid>

        <Box
          mt={4}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <Box>
            <SecondaryOutlineButton
              startIcon={<DeleteIcon />}
              onClick={() => setRemoveModalOpen(true)}
              color="error"
            >
              Remove Entry
            </SecondaryOutlineButton>
          </Box>
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
            >
              Push Config
            </PrimaryButton>
          </Box>
        </Box>
      </Box>

      <PushConfigurationModal
        open={pushModalOpen}
        onClose={() => setPushModalOpen(false)}
        onSuccess={handleRefresh}
      />

      <RemoveEdgeModal
        open={removeModalOpen}
        onClose={() => setRemoveModalOpen(false)}
        edge={edgeGateway}
        onSuccess={handleRemoveSuccess}
      />
    </Box>
  );
};

export default EdgeGatewayDetail;