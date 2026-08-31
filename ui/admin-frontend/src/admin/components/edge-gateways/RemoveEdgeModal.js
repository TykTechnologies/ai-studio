import React, { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Typography,
  Button,
  Alert,
  CircularProgress,
  Box,
} from '@mui/material';
import { Warning as WarningIcon } from '@mui/icons-material';
import edgeGatewayService from '../../services/edgeGatewayService';

const RemoveEdgeModal = ({ open, onClose, edge, onSuccess }) => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const handleClose = () => {
    if (!loading) {
      setError(null);
      onClose();
    }
  };

  const handleRemove = async () => {
    if (!edge) return;

    setLoading(true);
    setError(null);

    try {
      await edgeGatewayService.deleteEdgeGateway(edge.edgeId);

      if (onSuccess) {
        onSuccess();
      }

      handleClose();
    } catch (err) {
      console.error('Error removing edge gateway:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  if (!edge) return null;

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        <Box display="flex" alignItems="center" gap={1}>
          <WarningIcon color="warning" />
          Remove Edge Gateway Entry
        </Box>
      </DialogTitle>

      <DialogContent>
        <Typography variant="body1" paragraph>
          Are you sure you want to remove this edge gateway entry?
        </Typography>

        <Box sx={{ bgcolor: 'grey.100', p: 2, borderRadius: 1, mb: 2 }}>
          <Typography variant="body2" color="textSecondary">
            <strong>Edge ID:</strong> {edge.edgeId}
          </Typography>
          <Typography variant="body2" color="textSecondary">
            <strong>Namespace:</strong> {edge.namespace}
          </Typography>
        </Box>

        <Alert severity="info" sx={{ mb: 2 }}>
          This removes the entry from the database. It does <strong>not</strong>{' '}
          deregister or revoke the gateway: if that gateway is still running it
          will re-register itself on its next heartbeat and reappear here.
          Use this to tidy up entries for gateways that are genuinely gone.
        </Alert>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        {loading && (
          <Box display="flex" alignItems="center" gap={2} sx={{ mb: 2 }}>
            <CircularProgress size={20} />
            <Typography variant="body2">
              Removing edge gateway entry...
            </Typography>
          </Box>
        )}
      </DialogContent>

      <DialogActions>
        <Button onClick={handleClose} disabled={loading}>
          Cancel
        </Button>
        <Button
          onClick={handleRemove}
          variant="contained"
          color="error"
          disabled={loading}
          startIcon={loading ? <CircularProgress size={16} /> : null}
        >
          {loading ? 'Removing...' : 'Remove Entry'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default RemoveEdgeModal;
