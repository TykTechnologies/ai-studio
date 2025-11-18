import React, { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Typography,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Button,
  Alert,
  CircularProgress,
  Box,
  RadioGroup,
  FormControlLabel,
  Radio,
  FormLabel,
} from '@mui/material';
import { CloudSync as PushIcon } from '@mui/icons-material';
import edgeGatewayService from '../../services/edgeGatewayService';
import useNamespaces from '../../hooks/useNamespaces';
import useSystemFeatures from '../../hooks/useSystemFeatures';

const PushConfigurationModal = ({ open, onClose, onSuccess }) => {
  const { getAvailableNamespaces } = useNamespaces();
  const { features } = useSystemFeatures();

  // CE: Default to 'all' since namespace selection is enterprise-only
  const [targetType, setTargetType] = useState(features.hub_spoke_multi_tenant ? 'namespace' : 'all');
  const [selectedNamespace, setSelectedNamespace] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);

  const availableNamespaces = getAvailableNamespaces();

  const handleClose = () => {
    if (!loading) {
      setTargetType(features.hub_spoke_multi_tenant ? 'namespace' : 'all');
      setSelectedNamespace('');
      setError(null);
      setSuccess(null);
      onClose();
    }
  };

  const handleSubmit = async () => {
    setLoading(true);
    setError(null);
    setSuccess(null);

    try {
      let result;

      if (targetType === 'all') {
        // CE/ENT: Use global reload-all endpoint
        result = await edgeGatewayService.reloadAllEdges();

        const message = features.hub_spoke_multi_tenant
          ? `Configuration successfully pushed to all namespaces. Operation ID: ${result.operationId}`
          : `Configuration successfully pushed to all edge gateways. Operation ID: ${result.operationId}`;

        setSuccess(message);
      } else {
        // ENT only: Push to specific namespace
        if (!selectedNamespace) {
          setError('Please select a namespace');
          return;
        }

        result = await edgeGatewayService.triggerConfigurationReload(
          selectedNamespace === 'global' ? 'global' : selectedNamespace,
          'namespace'
        );

        setSuccess(`Configuration push initiated for ${selectedNamespace} namespace. Operation ID: ${result.operationId}`);
      }

      if (onSuccess) {
        onSuccess();
      }
    } catch (err) {
      console.error('Error pushing configuration:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const isValid = targetType === 'all' || selectedNamespace;

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        <Box display="flex" alignItems="center" gap={1}>
          <PushIcon />
          Push Configuration
        </Box>
      </DialogTitle>
      
      <DialogContent>
        <Typography variant="body2" color="textSecondary" paragraph>
          Push the latest configuration to edge gateways. This will reload all affected edge instances
          with the current configuration from the control server.
        </Typography>

        {/* CE: Hide namespace selection, ENT: Show radio buttons */}
        {features.hub_spoke_multi_tenant ? (
          <>
            <FormControl component="fieldset" fullWidth sx={{ mb: 3 }}>
              <FormLabel component="legend">Target</FormLabel>
              <RadioGroup
                value={targetType}
                onChange={(e) => setTargetType(e.target.value)}
              >
                <FormControlLabel
                  value="namespace"
                  control={<Radio />}
                  label="Specific Namespace"
                />
                <FormControlLabel
                  value="all"
                  control={<Radio />}
                  label="All Namespaces"
                />
              </RadioGroup>
            </FormControl>

            {targetType === 'namespace' && (
              <FormControl fullWidth required sx={{ mb: 3 }}>
                <InputLabel>Select Namespace</InputLabel>
                <Select
                  value={selectedNamespace}
                  label="Select Namespace"
                  onChange={(e) => setSelectedNamespace(e.target.value)}
                >
                  <MenuItem value="global">Global ({availableNamespaces.find(ns => ns.name === 'global')?.edgeCount || 0} edges)</MenuItem>
                  {availableNamespaces
                    .filter(ns => ns.name !== 'global')
                    .map((namespace) => (
                    <MenuItem key={namespace.name} value={namespace.name}>
                      {namespace.name} ({namespace.edgeCount} edges)
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}

            {targetType === 'all' && (
              <Alert severity="info" sx={{ mb: 2 }}>
                This will push configuration to all {availableNamespaces.length} namespaces with active edges.
              </Alert>
            )}
          </>
        ) : (
          <Alert severity="info" sx={{ mb: 2 }}>
            This will push configuration to all edge gateways.
          </Alert>
        )}

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        {success && (
          <Alert severity="success" sx={{ mb: 2 }}>
            {success}
          </Alert>
        )}

        {loading && (
          <Box display="flex" alignItems="center" gap={2} sx={{ mb: 2 }}>
            <CircularProgress size={20} />
            <Typography variant="body2">
              Initiating configuration push...
            </Typography>
          </Box>
        )}
      </DialogContent>

      <DialogActions>
        <Button onClick={handleClose} disabled={loading}>
          {success ? 'Close' : 'Cancel'}
        </Button>
        {!success && (
          <Button
            onClick={handleSubmit}
            variant="contained"
            disabled={loading || !isValid}
            startIcon={loading ? <CircularProgress size={16} /> : <PushIcon />}
          >
            {loading ? 'Pushing...' : 'Push Configuration'}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default PushConfigurationModal;