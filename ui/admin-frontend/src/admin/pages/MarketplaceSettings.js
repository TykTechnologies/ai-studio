import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Button,
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
  Snackbar,
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
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  DangerButton,
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  StyledDialogTitle,
  StyledDialogContent,
} from '../styles/sharedStyles';

const MarketplaceSettings = () => {
  const [marketplaces, setMarketplaces] = useState([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newMarketplaceURL, setNewMarketplaceURL] = useState('');
  const [setAsDefault, setSetAsDefault] = useState(false);
  const [validating, setValidating] = useState(false);
  const [validationResult, setValidationResult] = useState(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success',
  });

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
        setSnackbar({
          open: true,
          message: 'Multiple marketplace management requires Enterprise Edition',
          severity: 'warning',
        });
      } else {
        setSnackbar({
          open: true,
          message: 'Failed to load marketplaces',
          severity: 'error',
        });
      }
    } finally {
      setLoading(false);
    }
  };

  const handleAddMarketplace = async () => {
    try {
      await marketplaceManagementService.addMarketplace(newMarketplaceURL, setAsDefault);
      setSnackbar({
        open: true,
        message: 'Marketplace added successfully',
        severity: 'success',
      });
      setDialogOpen(false);
      setNewMarketplaceURL('');
      setSetAsDefault(false);
      setValidationResult(null);
      loadMarketplaces();
    } catch (error) {
      console.error('Failed to add marketplace:', error);
      const errorMessage = error.response?.data?.errors?.[0]?.detail || 'Failed to add marketplace';
      setSnackbar({
        open: true,
        message: errorMessage,
        severity: 'error',
      });
    }
  };

  const handleValidateURL = async () => {
    if (!newMarketplaceURL.trim()) {
      setSnackbar({
        open: true,
        message: 'Please enter a marketplace URL',
        severity: 'warning',
      });
      return;
    }

    try {
      setValidating(true);
      const result = await marketplaceManagementService.validateMarketplaceURL(newMarketplaceURL);
      setValidationResult(result);

      if (result.valid) {
        setSnackbar({
          open: true,
          message: `Valid marketplace with ${result.plugin_count} plugins`,
          severity: 'success',
        });
      } else {
        setSnackbar({
          open: true,
          message: result.error_message || 'Invalid marketplace URL',
          severity: 'error',
        });
      }
    } catch (error) {
      console.error('Failed to validate URL:', error);
      setValidationResult({ valid: false, error_message: 'Validation failed' });
      setSnackbar({
        open: true,
        message: 'Failed to validate marketplace URL',
        severity: 'error',
      });
    } finally {
      setValidating(false);
    }
  };

  const handleRemoveMarketplace = async (id) => {
    try {
      await marketplaceManagementService.removeMarketplace(id);
      setSnackbar({
        open: true,
        message: 'Marketplace removed successfully',
        severity: 'success',
      });
      setConfirmDeleteId(null);
      loadMarketplaces();
    } catch (error) {
      console.error('Failed to remove marketplace:', error);
      const errorMessage = error.response?.data?.errors?.[0]?.detail || 'Failed to remove marketplace';
      setSnackbar({
        open: true,
        message: errorMessage,
        severity: 'error',
      });
    }
  };

  const handleSetDefault = async (id) => {
    try {
      await marketplaceManagementService.setDefaultMarketplace(id);
      setSnackbar({
        open: true,
        message: 'Default marketplace updated',
        severity: 'success',
      });
      loadMarketplaces();
    } catch (error) {
      console.error('Failed to set default marketplace:', error);
      setSnackbar({
        open: true,
        message: 'Failed to set default marketplace',
        severity: 'error',
      });
    }
  };

  const handleToggleActive = async (id, currentActive) => {
    try {
      await marketplaceManagementService.toggleMarketplace(id, !currentActive);
      setSnackbar({
        open: true,
        message: `Marketplace ${!currentActive ? 'activated' : 'deactivated'}`,
        severity: 'success',
      });
      loadMarketplaces();
    } catch (error) {
      console.error('Failed to toggle marketplace:', error);
      const errorMessage = error.response?.data?.errors?.[0]?.detail || 'Failed to update marketplace';
      setSnackbar({
        open: true,
        message: errorMessage,
        severity: 'error',
      });
    }
  };

  const handleSyncMarketplace = async (id) => {
    try {
      await marketplaceManagementService.syncMarketplace(id);
      setSnackbar({
        open: true,
        message: 'Marketplace sync requested',
        severity: 'info',
      });
    } catch (error) {
      console.error('Failed to sync marketplace:', error);
      setSnackbar({
        open: true,
        message: 'Failed to trigger sync',
        severity: 'error',
      });
    }
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === 'clickaway') {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
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
    <Box sx={{ p: 0 }}>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Marketplace Sources</Typography>
        <PrimaryButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => setDialogOpen(true)}
        >
          Add Marketplace
        </PrimaryButton>
      </TitleBox>

      <Box sx={{ p: 3 }}>
        <ContentBox>
          <TableContainer component={StyledPaper}>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Source URL</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="center">Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="center">Default</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="center">Active</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Plugins</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Last Synced</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="center">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {marketplaces.length === 0 ? (
                  <StyledTableRow>
                    <StyledTableCell colSpan={7} align="center">
                      <Typography variant="body2" color="textSecondary">
                        No marketplaces configured
                      </Typography>
                    </StyledTableCell>
                  </StyledTableRow>
                ) : (
                  marketplaces.map((marketplace) => (
                    <StyledTableRow key={marketplace.id}>
                      <StyledTableCell>
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>
                          {marketplace.source_url}
                        </Typography>
                        {marketplace.sync_error && (
                          <Typography variant="caption" color="error" sx={{ display: 'block', mt: 0.5 }}>
                            Error: {marketplace.sync_error}
                          </Typography>
                        )}
                      </StyledTableCell>
                      <StyledTableCell align="center">
                        <Tooltip title={marketplace.sync_status || 'Unknown'}>
                          <Box>{getSyncStatusIcon(marketplace.sync_status)}</Box>
                        </Tooltip>
                      </StyledTableCell>
                      <StyledTableCell align="center">
                        <IconButton
                          size="small"
                          onClick={() => !marketplace.is_default && handleSetDefault(marketplace.id)}
                          disabled={marketplace.is_default}
                        >
                          {marketplace.is_default ? (
                            <StarIcon color="primary" />
                          ) : (
                            <StarBorderIcon />
                          )}
                        </IconButton>
                      </StyledTableCell>
                      <StyledTableCell align="center">
                        <Switch
                          checked={marketplace.is_active}
                          onChange={() => handleToggleActive(marketplace.id, marketplace.is_active)}
                          disabled={marketplace.is_default}
                          size="small"
                        />
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        {marketplace.plugin_count || 0}
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <Typography variant="caption">
                          {formatDate(marketplace.last_synced)}
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell align="center">
                        <Tooltip title="Sync Now">
                          <span>
                            <IconButton
                              size="small"
                              onClick={() => handleSyncMarketplace(marketplace.id)}
                              disabled={!marketplace.is_active}
                            >
                              <SyncIcon />
                            </IconButton>
                          </span>
                        </Tooltip>
                        <Tooltip title="Remove">
                          <span>
                            <IconButton
                              size="small"
                              onClick={() => setConfirmDeleteId(marketplace.id)}
                              disabled={marketplace.is_default}
                              color="error"
                            >
                              <DeleteIcon />
                            </IconButton>
                          </span>
                        </Tooltip>
                      </StyledTableCell>
                    </StyledTableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </ContentBox>
      </Box>

      {/* Add Marketplace Dialog */}
      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="md" fullWidth>
        <StyledDialogTitle>Add Marketplace Source</StyledDialogTitle>
        <StyledDialogContent>
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
              setValidationResult(null);
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
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => {
            setDialogOpen(false);
            setNewMarketplaceURL('');
            setSetAsDefault(false);
            setValidationResult(null);
          }}>
            Cancel
          </Button>
          <PrimaryButton
            onClick={handleAddMarketplace}
            disabled={!newMarketplaceURL.trim() || (validationResult && !validationResult.valid)}
          >
            Add Marketplace
          </PrimaryButton>
        </DialogActions>
      </Dialog>

      {/* Confirm Delete Dialog */}
      <Dialog open={confirmDeleteId !== null} onClose={() => setConfirmDeleteId(null)}>
        <StyledDialogTitle>Confirm Remove</StyledDialogTitle>
        <StyledDialogContent>
          <Typography>
            Are you sure you want to remove this marketplace? Plugins from this source will remain installed but updates will no longer be available.
          </Typography>
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDeleteId(null)}>Cancel</Button>
          <DangerButton
            onClick={() => handleRemoveMarketplace(confirmDeleteId)}
            variant="contained"
          >
            Remove
          </DangerButton>
        </DialogActions>
      </Dialog>

      {/* Snackbar for notifications */}
      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert onClose={handleCloseSnackbar} severity={snackbar.severity} sx={{ width: '100%' }}>
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default MarketplaceSettings;
