import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Button,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
  Chip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  FormControlLabel,
  Switch,
  Alert,
  CircularProgress,
  Tooltip,
} from '@mui/material';
import {
  Add as AddIcon,
  Delete as DeleteIcon,
  Sync as SyncIcon,
  Star as StarIcon,
  StarBorder as StarBorderIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
  Warning as WarningIcon,
} from '@mui/icons-material';
import marketplaceManagementService from '../services/marketplaceManagementService';
import { useSnackbar } from 'notistack';

const MarketplaceSettings = () => {
  const [marketplaces, setMarketplaces] = useState([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newMarketplaceURL, setNewMarketplaceURL] = useState('');
  const [setAsDefault, setSetAsDefault] = useState(false);
  const [validating, setValidating] = useState(false);
  const [validationResult, setValidationResult] = useState(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState(null);
  const { enqueueSnackbar } = useSnackbar();

  useEffect(() => {
    loadMarketplaces();
  }, []);

  const loadMarketplaces = async () => {
    try {
      setLoading(true);
      const data = await marketplaceManagementService.listMarketplaces();
      setMarketplaces(data || []);
    } catch (error) {
      console.error('Failed to load marketplaces:', error);

      // Check if this is a 403 (Community Edition)
      if (error.response?.status === 403) {
        enqueueSnackbar('Multiple marketplace management requires Enterprise Edition', {
          variant: 'warning',
          autoHideDuration: 6000,
        });
      } else {
        enqueueSnackbar('Failed to load marketplaces', { variant: 'error' });
      }
    } finally {
      setLoading(false);
    }
  };

  const handleAddMarketplace = async () => {
    try {
      await marketplaceManagementService.addMarketplace(newMarketplaceURL, setAsDefault);
      enqueueSnackbar('Marketplace added successfully', { variant: 'success' });
      setDialogOpen(false);
      setNewMarketplaceURL('');
      setSetAsDefault(false);
      setValidationResult(null);
      loadMarketplaces();
    } catch (error) {
      console.error('Failed to add marketplace:', error);
      const errorMessage = error.response?.data?.errors?.[0]?.detail || 'Failed to add marketplace';
      enqueueSnackbar(errorMessage, { variant: 'error' });
    }
  };

  const handleValidateURL = async () => {
    if (!newMarketplaceURL.trim()) {
      enqueueSnackbar('Please enter a marketplace URL', { variant: 'warning' });
      return;
    }

    try {
      setValidating(true);
      const result = await marketplaceManagementService.validateMarketplaceURL(newMarketplaceURL);
      setValidationResult(result);

      if (result.valid) {
        enqueueSnackbar(`Valid marketplace with ${result.plugin_count} plugins`, { variant: 'success' });
      } else {
        enqueueSnackbar(result.error_message || 'Invalid marketplace URL', { variant: 'error' });
      }
    } catch (error) {
      console.error('Failed to validate URL:', error);
      setValidationResult({ valid: false, error_message: 'Validation failed' });
      enqueueSnackbar('Failed to validate marketplace URL', { variant: 'error' });
    } finally {
      setValidating(false);
    }
  };

  const handleRemoveMarketplace = async (id) => {
    try {
      await marketplaceManagementService.removeMarketplace(id);
      enqueueSnackbar('Marketplace removed successfully', { variant: 'success' });
      setConfirmDeleteId(null);
      loadMarketplaces();
    } catch (error) {
      console.error('Failed to remove marketplace:', error);
      const errorMessage = error.response?.data?.errors?.[0]?.detail || 'Failed to remove marketplace';
      enqueueSnackbar(errorMessage, { variant: 'error' });
    }
  };

  const handleSetDefault = async (id) => {
    try {
      await marketplaceManagementService.setDefaultMarketplace(id);
      enqueueSnackbar('Default marketplace updated', { variant: 'success' });
      loadMarketplaces();
    } catch (error) {
      console.error('Failed to set default marketplace:', error);
      enqueueSnackbar('Failed to set default marketplace', { variant: 'error' });
    }
  };

  const handleToggleActive = async (id, currentActive) => {
    try {
      await marketplaceManagementService.toggleMarketplace(id, !currentActive);
      enqueueSnackbar(`Marketplace ${!currentActive ? 'activated' : 'deactivated'}`, { variant: 'success' });
      loadMarketplaces();
    } catch (error) {
      console.error('Failed to toggle marketplace:', error);
      const errorMessage = error.response?.data?.errors?.[0]?.detail || 'Failed to update marketplace';
      enqueueSnackbar(errorMessage, { variant: 'error' });
    }
  };

  const handleSyncMarketplace = async (id) => {
    try {
      await marketplaceManagementService.syncMarketplace(id);
      enqueueSnackbar('Marketplace sync requested', { variant: 'info' });
    } catch (error) {
      console.error('Failed to sync marketplace:', error);
      enqueueSnackbar('Failed to trigger sync', { variant: 'error' });
    }
  };

  const getSyncStatusIcon = (status) => {
    switch (status) {
      case 'success':
        return <CheckCircleIcon color="success" />;
      case 'error':
        return <ErrorIcon color="error" />;
      case 'in_progress':
        return <CircularProgress size={20} />;
      default:
        return <WarningIcon color="warning" />;
    }
  };

  const formatDate = (dateString) => {
    if (!dateString || dateString === '0001-01-01T00:00:00Z') return 'Never';
    return new Date(dateString).toLocaleString();
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '400px' }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4">Marketplace Sources</Typography>
        <Button
          variant="contained"
          color="primary"
          startIcon={<AddIcon />}
          onClick={() => setDialogOpen(true)}
        >
          Add Marketplace
        </Button>
      </Box>

      <Alert severity="info" sx={{ mb: 3 }}>
        <Typography variant="body2">
          <strong>Enterprise Feature:</strong> Manage multiple plugin marketplace sources. The default marketplace is the official Tyk AI Studio marketplace.
        </Typography>
      </Alert>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Source URL</TableCell>
              <TableCell align="center">Status</TableCell>
              <TableCell align="center">Default</TableCell>
              <TableCell align="center">Active</TableCell>
              <TableCell align="right">Plugins</TableCell>
              <TableCell align="right">Last Synced</TableCell>
              <TableCell align="center">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {marketplaces.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} align="center">
                  <Typography variant="body2" color="textSecondary">
                    No marketplaces configured
                  </Typography>
                </TableCell>
              </TableRow>
            ) : (
              marketplaces.map((marketplace) => (
                <TableRow key={marketplace.ID}>
                  <TableCell>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>
                      {marketplace.source_url}
                    </Typography>
                    {marketplace.sync_error && (
                      <Typography variant="caption" color="error">
                        Error: {marketplace.sync_error}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell align="center">
                    <Tooltip title={marketplace.sync_status || 'Unknown'}>
                      {getSyncStatusIcon(marketplace.sync_status)}
                    </Tooltip>
                  </TableCell>
                  <TableCell align="center">
                    <IconButton
                      size="small"
                      onClick={() => !marketplace.is_default && handleSetDefault(marketplace.ID)}
                      disabled={marketplace.is_default}
                    >
                      {marketplace.is_default ? (
                        <StarIcon color="primary" />
                      ) : (
                        <StarBorderIcon />
                      )}
                    </IconButton>
                  </TableCell>
                  <TableCell align="center">
                    <Switch
                      checked={marketplace.is_active}
                      onChange={() => handleToggleActive(marketplace.ID, marketplace.is_active)}
                      disabled={marketplace.is_default} // Cannot deactivate default
                      size="small"
                    />
                  </TableCell>
                  <TableCell align="right">
                    {marketplace.plugin_count || 0}
                  </TableCell>
                  <TableCell align="right">
                    <Typography variant="caption">
                      {formatDate(marketplace.last_synced)}
                    </Typography>
                  </TableCell>
                  <TableCell align="center">
                    <Tooltip title="Sync Now">
                      <IconButton
                        size="small"
                        onClick={() => handleSyncMarketplace(marketplace.ID)}
                        disabled={!marketplace.is_active}
                      >
                        <SyncIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Remove">
                      <span>
                        <IconButton
                          size="small"
                          onClick={() => setConfirmDeleteId(marketplace.ID)}
                          disabled={marketplace.is_default} // Cannot remove default
                          color="error"
                        >
                          <DeleteIcon />
                        </IconButton>
                      </span>
                    </Tooltip>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Add Marketplace Dialog */}
      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>Add Marketplace Source</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="Marketplace Index URL"
            type="url"
            fullWidth
            variant="outlined"
            value={newMarketplaceURL}
            onChange={(e) => {
              setNewMarketplaceURL(e.target.value);
              setValidationResult(null); // Clear validation when URL changes
            }}
            placeholder="https://example.com/index.yaml"
            helperText="URL to the marketplace index.yaml file"
            sx={{ mt: 2 }}
          />

          <Box sx={{ mt: 2, display: 'flex', gap: 2, alignItems: 'center' }}>
            <Button
              variant="outlined"
              onClick={handleValidateURL}
              disabled={validating || !newMarketplaceURL.trim()}
            >
              {validating ? <CircularProgress size={20} /> : 'Validate URL'}
            </Button>

            {validationResult && (
              <Chip
                label={validationResult.valid ?
                  `Valid (${validationResult.plugin_count} plugins)` :
                  'Invalid URL'}
                color={validationResult.valid ? 'success' : 'error'}
                size="small"
              />
            )}
          </Box>

          {validationResult && !validationResult.valid && (
            <Alert severity="error" sx={{ mt: 2 }}>
              {validationResult.error_message}
            </Alert>
          )}

          <FormControlLabel
            control={
              <Switch
                checked={setAsDefault}
                onChange={(e) => setSetAsDefault(e.target.checked)}
              />
            }
            label="Set as default marketplace"
            sx={{ mt: 2 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setDialogOpen(false);
            setNewMarketplaceURL('');
            setSetAsDefault(false);
            setValidationResult(null);
          }}>
            Cancel
          </Button>
          <Button
            onClick={handleAddMarketplace}
            variant="contained"
            color="primary"
            disabled={!newMarketplaceURL.trim() || (validationResult && !validationResult.valid)}
          >
            Add Marketplace
          </Button>
        </DialogActions>
      </Dialog>

      {/* Confirm Delete Dialog */}
      <Dialog open={confirmDeleteId !== null} onClose={() => setConfirmDeleteId(null)}>
        <DialogTitle>Confirm Remove</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to remove this marketplace? Plugins from this source will remain installed but updates will no longer be available.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDeleteId(null)}>Cancel</Button>
          <Button
            onClick={() => handleRemoveMarketplace(confirmDeleteId)}
            color="error"
            variant="contained"
          >
            Remove
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default MarketplaceSettings;
