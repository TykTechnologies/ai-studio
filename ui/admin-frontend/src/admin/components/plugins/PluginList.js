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
  TextField,
  InputAdornment,
  TablePagination,
  Menu,
  Snackbar,
} from '@mui/material';
import {
  Add as AddIcon,
  Edit as EditIcon,
  Visibility as ViewIcon,
  Delete as DeleteIcon,
  Search as SearchIcon,
  MoreVert as MoreVertIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import pluginService from '../../services/pluginService';
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  DangerButton,
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
} from '../../styles/sharedStyles';

const PluginList = () => {
  const navigate = useNavigate();
  
  const [plugins, setPlugins] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [hookTypeFilter, setHookTypeFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('all'); // 'all', true = active, false = inactive
  
  // Pagination
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [totalCount, setTotalCount] = useState(0);
  
  // Menu state for actions
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedPlugin, setSelectedPlugin] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success',
  });

  const fetchPlugins = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    try {
      const result = await pluginService.listPlugins(
        page + 1, // API uses 1-based pagination
        rowsPerPage,
        hookTypeFilter,
        statusFilter === 'all' ? undefined : statusFilter
      );
      
      setPlugins(result.data || []);
      setTotalCount(result.meta?.total_count || 0);
    } catch (err) {
      console.error('Error fetching plugins:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [page, rowsPerPage, hookTypeFilter, statusFilter]);

  useEffect(() => {
    fetchPlugins();
  }, [fetchPlugins]);

  // Reset to first page when filters change
  useEffect(() => {
    setPage(0);
  }, [hookTypeFilter, statusFilter]);

  const handleCreate = () => {
    navigate('/admin/plugins/create');
  };

  const handleEdit = (id) => {
    navigate(`/admin/plugins/${id}/edit`);
  };

  const handleView = (id) => {
    navigate(`/admin/plugins/${id}`);
  };

  const handleMenuOpen = (event, plugin) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedPlugin(plugin);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id, name) => {
    if (window.confirm(`Are you sure you want to delete the plugin "${name}"?`)) {
      try {
        await pluginService.deletePlugin(id);
        setSnackbar({
          open: true,
          message: 'Plugin deleted successfully',
          severity: 'success',
        });
        fetchPlugins(); // Refresh the list
      } catch (err) {
        setError(err.message);
        setSnackbar({
          open: true,
          message: 'Failed to delete plugin',
          severity: 'error',
        });
      }
    }
    handleMenuClose();
  };

  const handleReloadPlugin = async (id, name) => {
    if (window.confirm(`Reload plugin "${name}"? This will reload the plugin binary and refresh its manifest.`)) {
      try {
        setLoading(true);
        await pluginService.reloadPlugin(id);
        setSnackbar({
          open: true,
          message: `Plugin "${name}" reloaded successfully`,
          severity: 'success',
        });
        fetchPlugins(); // Refresh the list to show updated status
      } catch (err) {
        setError(err.message);
        setSnackbar({
          open: true,
          message: `Failed to reload plugin "${name}"`,
          severity: 'error',
        });
      } finally {
        setLoading(false);
      }
    }
    handleMenuClose();
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === 'clickaway') {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleHookTypeFilterChange = (event) => {
    setHookTypeFilter(event.target.value);
  };

  const handleStatusFilterChange = (event) => {
    setStatusFilter(event.target.value);
  };

  const handleChangePage = (event, newPage) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (event) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  // Client-side search filtering
  const filteredPlugins = plugins.filter(plugin =>
    searchTerm === '' || 
    plugin.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    plugin.slug.toLowerCase().includes(searchTerm.toLowerCase()) ||
    plugin.description.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const availableHookTypes = pluginService.getAvailableHookTypes();

  return (
    <Box sx={{ p: 0 }}>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Plugins</Typography>
        <PrimaryButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleCreate}
        >
          Create Plugin
        </PrimaryButton>
      </TitleBox>

      <Box sx={{ p: 3 }}>
        {/* Filters and Search */}
        <Box mb={3} display="flex" gap={2} flexWrap="wrap" alignItems="center">
          <TextField
            size="small"
            placeholder="Search plugins..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon />
                </InputAdornment>
              ),
            }}
            sx={{ minWidth: 250 }}
          />
          
          <FormControl size="small" style={{ minWidth: 150 }}>
            <InputLabel>Hook Type</InputLabel>
            <Select
              value={hookTypeFilter}
              label="Hook Type"
              onChange={handleHookTypeFilterChange}
            >
              <MenuItem value="">All Types</MenuItem>
              {availableHookTypes.map((hookType) => (
                <MenuItem key={hookType.value} value={hookType.value}>
                  {hookType.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl size="small" style={{ minWidth: 120 }}>
            <InputLabel>Status</InputLabel>
            <Select
              value={statusFilter}
              label="Status"
              onChange={handleStatusFilterChange}
            >
              <MenuItem value="all">All</MenuItem>
              <MenuItem value={true}>Active</MenuItem>
              <MenuItem value={false}>Inactive</MenuItem>
            </Select>
          </FormControl>
        </Box>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        {loading ? (
          <Box display="flex" justifyContent="center" p={4}>
            <CircularProgress />
          </Box>
        ) : (
          <>
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Hook Type</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Namespace</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {filteredPlugins.length === 0 ? (
                    <TableRow>
                      <StyledTableCell colSpan={6} align="center">
                        <Typography variant="body2" color="textSecondary" py={4}>
                          {searchTerm ? 'No plugins match your search criteria' : 'No plugins found'}
                        </Typography>
                      </StyledTableCell>
                    </TableRow>
                  ) : (
                    filteredPlugins.map((plugin) => (
                      <StyledTableRow 
                        key={plugin.id} 
                        onClick={() => handleView(plugin.id)}
                        sx={{ cursor: "pointer" }}
                      >
                        <StyledTableCell>
                          <Box>
                            <Typography variant="body2" fontWeight="medium">
                              {plugin.name}
                            </Typography>
                            <Typography variant="caption" color="textSecondary">
                              {plugin.slug}
                            </Typography>
                          </Box>
                        </StyledTableCell>
                        <StyledTableCell>
                          <Chip
                            label={pluginService.getHookTypeLabel(plugin.hookType)}
                            size="small"
                            variant="outlined"
                            color="primary"
                          />
                        </StyledTableCell>
                        <StyledTableCell>
                          <Chip
                            label={plugin.namespace}
                            size="small"
                            variant="outlined"
                            color={plugin.namespace === 'global' ? 'default' : 'secondary'}
                          />
                        </StyledTableCell>
                        <StyledTableCell>
                          <Chip
                            label={plugin.isActive ? 'Active' : 'Inactive'}
                            size="small"
                            color={plugin.isActive ? 'success' : 'default'}
                            variant={plugin.isActive ? 'filled' : 'outlined'}
                          />
                        </StyledTableCell>
                        <StyledTableCell>
                          <Typography 
                            variant="body2" 
                            sx={{ 
                              maxWidth: 300, 
                              overflow: 'hidden', 
                              textOverflow: 'ellipsis',
                              whiteSpace: 'nowrap'
                            }}
                          >
                            {plugin.description || 'No description'}
                          </Typography>
                        </StyledTableCell>
                        <StyledTableCell align="right">
                          <IconButton
                            onClick={(event) => handleMenuOpen(event, plugin)}
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

            <TablePagination
              rowsPerPageOptions={[10, 25, 50, 100]}
              component="div"
              count={totalCount}
              rowsPerPage={rowsPerPage}
              page={page}
              onPageChange={handleChangePage}
              onRowsPerPageChange={handleChangeRowsPerPage}
            />
          </>
        )}
        
        <Menu
          anchorEl={anchorEl}
          open={Boolean(anchorEl)}
          onClose={handleMenuClose}
        >
          <MenuItem
            onClick={() => {
              handleView(selectedPlugin?.id);
              handleMenuClose();
            }}
          >
            View Details
          </MenuItem>
          <MenuItem
            onClick={() => {
              handleEdit(selectedPlugin?.id);
              handleMenuClose();
            }}
          >
            Edit Plugin
          </MenuItem>
          {selectedPlugin?.pluginType === 'ai_studio' && (
            <MenuItem
              onClick={() => {
                handleReloadPlugin(selectedPlugin.id, selectedPlugin.name);
                handleMenuClose();
              }}
            >
              Reload Plugin
            </MenuItem>
          )}
          <MenuItem
            onClick={() => handleDelete(selectedPlugin?.id, selectedPlugin?.name)}
          >
            Delete Plugin
          </MenuItem>
        </Menu>

        <Snackbar
          open={snackbar.open}
          autoHideDuration={6000}
          onClose={handleCloseSnackbar}
          anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
        >
          <Alert
            onClose={handleCloseSnackbar}
            severity={snackbar.severity}
            sx={{ width: "100%" }}
          >
            {snackbar.message}
          </Alert>
        </Snackbar>
      </Box>
    </Box>
  );
};

export default PluginList;