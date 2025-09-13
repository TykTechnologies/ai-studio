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
} from '@mui/material';
import {
  Add as AddIcon,
  Edit as EditIcon,
  Visibility as ViewIcon,
  Delete as DeleteIcon,
  Search as SearchIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import pluginService from '../../services/pluginService';
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  DangerButton,
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

  const handleDelete = async (id, name) => {
    if (window.confirm(`Are you sure you want to delete the plugin "${name}"?`)) {
      try {
        await pluginService.deletePlugin(id);
        fetchPlugins(); // Refresh the list
      } catch (err) {
        setError(err.message);
      }
    }
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
    <Box>
      <TitleBox>
        <Box display="flex" justifyContent="space-between" alignItems="center">
          <Typography variant="h4" component="h1">
            Plugins
          </Typography>
          <PrimaryButton
            startIcon={<AddIcon />}
            onClick={handleCreate}
          >
            Create Plugin
          </PrimaryButton>
        </Box>
      </TitleBox>

      <ContentBox>
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
            <TableContainer component={Paper} variant="outlined">
              <Table>
                <TableHead>
                  <TableRow>
                    <TableCell>Name</TableCell>
                    <TableCell>Hook Type</TableCell>
                    <TableCell>Namespace</TableCell>
                    <TableCell>Status</TableCell>
                    <TableCell>Description</TableCell>
                    <TableCell align="right">Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {filteredPlugins.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} align="center">
                        <Typography variant="body2" color="textSecondary" py={4}>
                          {searchTerm ? 'No plugins match your search criteria' : 'No plugins found'}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredPlugins.map((plugin) => (
                      <TableRow key={plugin.id} hover>
                        <TableCell>
                          <Box>
                            <Typography variant="body2" fontWeight="medium">
                              {plugin.name}
                            </Typography>
                            <Typography variant="caption" color="textSecondary">
                              {plugin.slug}
                            </Typography>
                          </Box>
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={pluginService.getHookTypeLabel(plugin.hookType)}
                            size="small"
                            variant="outlined"
                            color="primary"
                          />
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={plugin.namespace}
                            size="small"
                            variant="outlined"
                            color={plugin.namespace === 'global' ? 'default' : 'secondary'}
                          />
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={plugin.isActive ? 'Active' : 'Inactive'}
                            size="small"
                            color={plugin.isActive ? 'success' : 'default'}
                            variant={plugin.isActive ? 'filled' : 'outlined'}
                          />
                        </TableCell>
                        <TableCell>
                          <Typography variant="body2" sx={{ maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                            {plugin.description || 'No description'}
                          </Typography>
                        </TableCell>
                        <TableCell align="right">
                          <Box display="flex" gap={1}>
                            <Tooltip title="View Details">
                              <IconButton
                                size="small"
                                onClick={() => handleView(plugin.id)}
                              >
                                <ViewIcon />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Edit">
                              <IconButton
                                size="small"
                                onClick={() => handleEdit(plugin.id)}
                              >
                                <EditIcon />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Delete">
                              <IconButton
                                size="small"
                                onClick={() => handleDelete(plugin.id, plugin.name)}
                                sx={{ color: 'error.main' }}
                              >
                                <DeleteIcon />
                              </IconButton>
                            </Tooltip>
                          </Box>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </TableContainer>

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
      </ContentBox>
    </Box>
  );
};

export default PluginList;